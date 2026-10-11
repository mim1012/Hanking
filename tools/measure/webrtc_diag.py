#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""webrtc_diag.py — WebRTC 연결 체인 계층별 진단(offer/answer → ICE → UDP STUN).

viewer_latency.py 의 패턴 계승(토큰 처리/채널 선택/utf-8 stdout/ignore_https_errors).
1회 실행으로 3개 계층을 순서대로 점검한다:
  [offer/answer] 페이지 내 RTCPeerConnection(recvonly, STUN google) →
                 POST /admin/webrtc/offer → answer SDP(후보/공개IP/미디어 라인) 수집
  [ICE]          setRemoteDescription 후 최대 15s 상태 타임라인 관측
  [UDP]          Python STUN Binding 프로브(50000-50100) — 브라우저 단계와 병렬로
                 스레드가 돌며, answer 후보 포트가 파싱되면 그 포트를 정밀 프로브

getStats RTT 등은 성공 시에만 측정. 실패 시 후보/상태 타임라인과 STUN 응답이
판정 근거. 토큰은 절대 출력하지 않는다(마스크: 앞 4자리 + "***").
종료 코드: 0 = 실행 완료(체인 실패도 데이터), 1 = 하드 오류, 2 = 토큰/환경 문제.
"""
from __future__ import annotations

import argparse
import json
import os
import socket
import ssl
import struct
import sys
import threading
import time
import urllib.request
from datetime import datetime
from typing import Any, Optional
from urllib.parse import urlparse

try:
    from playwright.sync_api import sync_playwright
except ImportError:
    print("[webrtc_diag] playwright 미설치 — "
          "pip install playwright && playwright install chromium", file=sys.stderr)
    sys.exit(2)

DEF_URL = "https://45.32.251.5:8443"
DEF_AGENT = "onpxe-PC-012"
DEF_OUT = "./webrtc_diag_result.json"

UDP_PORT_LO = 50000
UDP_PORT_HI = 50100
STUN_MAGIC = 0x2112A442
STUN_BINDING_REQUEST = 0x0001
STUN_BINDING_SUCCESS = 0x0101

# 브라우저 1단계: PC 생성 → offer → 서버 answer 수신 → setRemoteDescription 까지.
# 상태는 window.__diag 에 남겨 2단계(WAIT_JS)가 이어받는다.
# 토큰은 fetch 헤더용으로만 페이지에 전달되며 어떤 기록에도 남지 않는다.
SETUP_JS = r"""
async (opts) => {
  const log = []; const t0 = performance.now();
  const pc = new RTCPeerConnection({iceServers:[{urls:'stun:stun.l.google.com:19302'}]});
  const d = {pc, log, t0, localCands: [], trackAt: null, answerApplied: false};
  window.__diag = d;
  for (const ev of ['signalingstatechange','iceconnectionstatechange','connectionstatechange','icegatheringstatechange'])
    pc.addEventListener(ev, () => log.push({t: Math.round(performance.now()-t0), ev,
      ice: pc.iceConnectionState, conn: pc.connectionState, sig: pc.signalingState, gather: pc.iceGatheringState}));
  pc.onicecandidate = e => { if (e.candidate) d.localCands.push(String(e.candidate.candidate).slice(0,120)); };
  pc.ontrack = () => { if (d.trackAt === null) d.trackAt = Math.round(performance.now()-t0); };
  pc.addTransceiver('video', {direction:'recvonly'});
  const offer = await pc.createOffer(); await pc.setLocalDescription(offer);
  const g0 = Date.now(); while (pc.iceGatheringState !== 'complete' && Date.now()-g0 < 3000) await new Promise(r=>setTimeout(r,100));
  let httpStatus = 0, answerSdp = '', answerBody = '';
  try {
    const ac = new AbortController();
    const tm = setTimeout(() => ac.abort(), 20000);
    const resp = await fetch(opts.url + '/admin/webrtc/offer', {method:'POST',
      headers:{'Content-Type':'application/json','X-Auth-Token':opts.token},
      body: JSON.stringify({agent_id: opts.agent, sdp: pc.localDescription.sdp}), signal: ac.signal});
    clearTimeout(tm);
    httpStatus = resp.status;
    const raw = await resp.text().catch(()=>'');
    answerBody = raw.slice(0, 300);
    try { const j = JSON.parse(raw); answerSdp = j.sdp || ''; } catch(e) {}
  } catch (e) { return {error: 'fetch: '+e.message, log, localCands: d.localCands,
    answerInfo: {status: 0, sdp_len: 0, cand_count: 0, cand_lines: [], has_public_ip: false, media_line: '', body: ''}}; }
  const cands = (answerSdp.match(/^a=candidate:.*$/gm) || []);
  const answerInfo = {status: httpStatus, sdp_len: answerSdp.length, cand_count: cands.length,
    cand_lines: cands.slice(0,8), has_public_ip: answerSdp.includes('45.32.251.5'),
    media_line: (answerSdp.match(/^m=.*$/m)||[''])[0], body: answerBody};
  if (httpStatus !== 200 || !answerSdp) { try { pc.close(); } catch(e) {} return {log, localCands: d.localCands, answerInfo}; }
  await pc.setRemoteDescription({type:'answer', sdp: answerSdp});
  d.answerApplied = true;
  return {log, localCands: d.localCands, answerInfo};
}
"""

# 브라우저 2단계: ICE 최대 15s 대기 + 최종 상태 + 성공 시에만 getStats.
WAIT_JS = r"""
async () => {
  const d = window.__diag; if (!d) return {error: 'no __diag'};
  const pc = d.pc, log = d.log, t0 = d.t0;
  let final = {};
  if (d.answerApplied) {
    const w0 = Date.now();
    while (Date.now()-w0 < 15000) {
      const ice = pc.iceConnectionState, conn = pc.connectionState;
      final = {ice, conn, t: Math.round(performance.now()-t0)};
      if (['connected','completed'].includes(ice) || ['failed','closed'].includes(ice)) break;
      await new Promise(r=>setTimeout(r,400));
    }
  } else {
    final = {ice: pc.iceConnectionState, conn: pc.connectionState, t: Math.round(performance.now()-t0)};
  }
  let stats = null;
  if (['connected','completed'].includes(pc.iceConnectionState)) {
    const pairs = [], inbs = [];
    const s = await pc.getStats();
    s.forEach(r => {
      if (r.type==='candidate-pair' && (r.state==='succeeded'||r.nominated))
        pairs.push({state:r.state, rtt:r.currentRoundTripTime, bytesRecv:r.bytesReceived, bytesSent:r.bytesSent});
      if (r.type==='inbound-rtp' && r.kind==='video')
        inbs.push({packets:r.packetsReceived, frames:r.framesDecoded, jitter:r.jitter});
    });
    stats = {pairs, inbs};
  }
  const result = {log, final, trackAt: d.trackAt, stats};
  try { pc.close(); } catch(e) {}
  return result;
}
"""


def env_default(name: str, default: str) -> str:
    v = os.environ.get(name)
    return v if v else default


def mask_token(token: str) -> str:
    return (token[:4] + "***") if len(token) > 4 else "***"


def resolve_token(args: argparse.Namespace) -> "tuple[Optional[str], Optional[str]]":
    """--token 인자 > ADMIN_TOKEN env > --token-file/ADMIN_TOKEN_FILE 경로 순."""
    if args.token:
        return args.token.strip(), None
    env_tok = os.environ.get("ADMIN_TOKEN", "").strip()
    if env_tok:
        return env_tok, None
    tf = (args.token_file or os.environ.get("ADMIN_TOKEN_FILE", "") or "").strip()
    if tf:
        if not os.path.isfile(tf):
            return None, "ADMIN_TOKEN_FILE 경로에 파일 없음: %s" % tf
        try:
            with open(tf, "r", encoding="utf-8") as f:
                for line in f:
                    if line.startswith("ADMIN_TOKEN="):
                        v = line.split("=", 1)[1].strip()
                        if v:
                            return v, None
                return None, "토큰 파일에 ADMIN_TOKEN= 항목 없음: %s" % tf
        except OSError as e:
            return None, "토큰 파일 읽기 실패: %s" % e
    return None, ("ADMIN_TOKEN 없음 — --token 인자 / ADMIN_TOKEN env / "
                  "--token-file(ADMIN_TOKEN_FILE) 중 하나로 제공")


def _default_channel() -> "str | None":
    """실제 Chrome 채널 우선 — 시스템 Chrome은 H.264를 포함하지만 Playwright 번들
    Chromium은 미포함이라 WebRTC H264 디코드가 실패할 수 있음. PW_CHANNEL 강제 가능."""
    ch = os.environ.get("PW_CHANNEL", "").strip()
    if ch == "none":
        return None
    if ch:
        return ch
    for pth in (r"C:\Program Files\Google\Chrome\Application\chrome.exe",
                os.path.expandvars(r"%LOCALAPPDATA%\Google\Chrome\Application\chrome.exe"),
                "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"):
        if os.path.exists(pth):
            return "chrome"
    return None


def stun_probe(host: str, port: int, timeout: float = 1.0) -> dict:
    """STUN Binding Request 1건 → {port, ok, rtt_ms}.
    검증: 응답 길이>=20, 타입 0x0101, txid echo(resp[8:20])."""
    txid = os.urandom(12)
    req = struct.pack(">HHI", STUN_BINDING_REQUEST, 0, STUN_MAGIC) + txid
    rtt_ms = None
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.settimeout(timeout)
            t0 = time.perf_counter()
            s.sendto(req, (host, port))
            resp, _ = s.recvfrom(2048)
            rtt_ms = int((time.perf_counter() - t0) * 1000)
        ok = (len(resp) >= 20
              and struct.unpack(">H", resp[:2])[0] == STUN_BINDING_SUCCESS
              and resp[8:20] == txid)
        return {"port": port, "ok": ok, "rtt_ms": rtt_ms}
    except OSError:
        return {"port": port, "ok": False, "rtt_ms": None}


def pick_agent(url: str, token: str) -> "tuple[str, Optional[str], list]":
    """GET /admin/agents → online + 'onpxe-' 접두 첫 id. 실패/미발견 → 폴백 DEF_AGENT."""
    endpoint = url.rstrip("/") + "/admin/agents"
    req = urllib.request.Request(endpoint, headers={"X-Auth-Token": token})
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    agents: list = []
    try:
        with urllib.request.urlopen(req, timeout=10, context=ctx) as resp:
            data = json.loads(resp.read().decode("utf-8", "replace"))
        agents = data.get("agents") or []
    except Exception as e:
        return DEF_AGENT, "agents 조회 실패(%s: %s) → 폴백 %s" % (type(e).__name__, e, DEF_AGENT), agents
    for a in agents:
        if a.get("online") and str(a.get("id", "")).startswith("onpxe-"):
            return str(a["id"]), None, agents
    return DEF_AGENT, "online onpxe-* 미발견 → 폴백 %s" % DEF_AGENT, agents


def parse_candidate_ports(cand_lines: list) -> list:
    """a=candidate 라인에서 UDP 포트 추출(중복 제거 후 정렬).
    라인 형식: a=candidate:<foundation> <component> <transport> <priority> <ip> <port> typ ..."""
    ports: list = []
    for line in cand_lines or []:
        parts = line.split()
        if len(parts) >= 7 and parts[2].lower() == "udp":
            try:
                p = int(parts[5])
            except ValueError:
                continue
            if p not in ports:
                ports.append(p)
    return sorted(ports)


def _run_prober(host: str, target_ports: list, lock: threading.Lock, successes: list,
                stop_event: threading.Event, deadline: float, t_start: float) -> None:
    """병렬 STUN 프로버: target_ports 비었으면 스윕(4포트/500ms), 차면 정밀(400ms).
    deadline(스레드 시작 후 20s) 또는 stop_event 까지. 성공은 {port, rtt_ms, at_elapsed_s}."""
    sweep = [UDP_PORT_LO, UDP_PORT_LO + 1, (UDP_PORT_LO + UDP_PORT_HI) // 2, UDP_PORT_HI]
    while not stop_event.is_set() and time.monotonic() < deadline:
        with lock:
            targeted = bool(target_ports)
            ports = list(target_ports) if targeted else list(sweep)
        interval = 0.4 if targeted else 0.5
        t_sweep = time.monotonic()
        for port in ports:
            if stop_event.is_set() or time.monotonic() >= deadline:
                break
            r = stun_probe(host, port, timeout=0.5)
            if r.get("ok"):
                with lock:
                    successes.append({"port": port, "rtt_ms": r.get("rtt_ms"),
                                      "at_elapsed_s": round(time.monotonic() - t_start, 2)})
        remain = interval - (time.monotonic() - t_sweep)
        if remain > 0:
            time.sleep(remain)


def main() -> int:
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass
    parser = argparse.ArgumentParser(
        description="WebRTC 연결 체인 계층별 진단(offer/answer → ICE → UDP STUN)")
    parser.add_argument("--token", default="", help="ADMIN_TOKEN (env ADMIN_TOKEN 도 사용)")
    parser.add_argument("--token-file", default="", help="토큰 파일 경로 (env ADMIN_TOKEN_FILE)")
    parser.add_argument("--url", default=env_default("VIEWER_URL", DEF_URL))
    parser.add_argument("--agent", default="",
                        help="에이전트 id (기본: /admin/agents 에서 online onpxe-* 첫 id)")
    parser.add_argument("--out", default=env_default("OUT", DEF_OUT))
    parser.add_argument("--headed", action="store_true",
                        default=(os.environ.get("HEADED", "").strip() in ("1", "true", "TRUE")),
                        help="헤드리스 해제(디버깅). env HEADED=1 동일.")
    args = parser.parse_args()

    token, tok_err = resolve_token(args)
    if not token:
        print("[webrtc_diag] 토큰 없음 — %s" % (tok_err or "사유 미상"), file=sys.stderr)
        return 2

    host = urlparse(args.url).hostname or ""
    if not host:
        print("[webrtc_diag] URL 에서 host 추출 실패: %s" % args.url, file=sys.stderr)
        return 2

    if args.agent.strip():
        agent, agent_note, agents_seen = args.agent.strip(), None, []
    else:
        agent, agent_note, agents_seen = pick_agent(args.url, token)

    started_at_iso = datetime.now().astimezone().isoformat(timespec="milliseconds")
    errors: list = []
    baseline = stun_probe(host, UDP_PORT_LO, timeout=1.0)

    target_ports: list = []
    lock = threading.Lock()
    successes: list = []
    stop_event = threading.Event()
    t_thread = time.monotonic()
    prober = threading.Thread(target=_run_prober,
                              args=(host, target_ports, lock, successes, stop_event,
                                    t_thread + 20.0, t_thread), daemon=True)
    prober_started = False

    r1: dict = {}
    r2: dict = {}
    try:
        with sync_playwright() as p:
            channel = _default_channel()
            print("[webrtc_diag] browser channel=%s" % (channel or "bundled"))
            browser = p.chromium.launch(headless=not args.headed, channel=channel)
            context = browser.new_context(ignore_https_errors=True,
                                          viewport={"width": 1280, "height": 800})
            page = context.new_page()
            page.goto(args.url, wait_until="domcontentloaded", timeout=30000)

            prober.start()  # ICE 대기(최대 15s) 전 기동 — 후보 파싱 전엔 스윕 프로브
            prober_started = True

            r1 = page.evaluate(SETUP_JS, {"url": args.url, "token": token, "agent": agent}) or {}
            with lock:  # answer SDP 후보 포트 → 프로버 타깃 전환
                target_ports[:] = parse_candidate_ports(
                    (r1.get("answerInfo") or {}).get("cand_lines"))
            r2 = page.evaluate(WAIT_JS) or {}
            try:
                context.close()
                browser.close()
            except Exception:
                pass
    except Exception as e:
        errors.append("브라우저 단계 실패: %s: %s" % (type(e).__name__, e))
    finally:
        if prober_started:
            time.sleep(2.0)  # STUN 스레드 꼬리 수집
            stop_event.set()
            prober.join(timeout=5.0)

    log = r2.get("log") or r1.get("log") or []
    local_cands = r1.get("localCands") or []
    answer_info = r1.get("answerInfo") or {}
    final = r2.get("final") or {}
    track_at = r2.get("trackAt")
    stats = r2.get("stats")
    setup_error = r1.get("error")  # 체인 데이터(판정 근거) — 하드 오류 아님
    wait_error = r2.get("error")

    status = answer_info.get("status")
    if status == 200 and (answer_info.get("sdp_len") or 0) > 0:
        offer_verdict = "OK — http=200 sdp_len=%s cand_count=%s public_ip=%s" % (
            answer_info.get("sdp_len"), answer_info.get("cand_count"),
            answer_info.get("has_public_ip"))
    else:
        offer_verdict = "FAIL — http=%s sdp_len=%s cand_count=%s%s" % (
            status, answer_info.get("sdp_len"), answer_info.get("cand_count"),
            (" (%s)" % setup_error) if setup_error else "")
    ice_state = final.get("ice") or "unknown"
    ice_verdict = "state=%s (conn=%s, t=%sms)" % (ice_state, final.get("conn"), final.get("t"))
    if successes:
        first = successes[0]
        udp_verdict = "reachable — %d건 (첫 응답 port=%s rtt=%sms @%ss)" % (
            len(successes), first.get("port"), first.get("rtt_ms"), first.get("at_elapsed_s"))
    else:
        udp_verdict = "not reachable — STUN 응답 0건 (baseline ok=%s)" % baseline.get("ok")

    ended_at_iso = datetime.now().astimezone().isoformat(timespec="milliseconds")
    result = {
        "meta": {"url": args.url, "host": host, "agent": agent, "agent_note": agent_note,
                 "agents_seen": [{"id": a.get("id"), "online": a.get("online")}
                                 for a in agents_seen[:20]],
                 "token_used": mask_token(token), "headed": args.headed,
                 "channel": _default_channel(),
                 "started_at_iso": started_at_iso, "ended_at_iso": ended_at_iso},
        "baseline_stun": baseline,
        "stun": {"udp_port_range": [UDP_PORT_LO, UDP_PORT_HI],
                 "target_ports": list(target_ports), "successes": successes,
                 "success_count": len(successes)},
        "browser": {"log": log, "local_cands": local_cands, "answer_info": answer_info,
                    "setup_error": setup_error, "wait_error": wait_error,
                    "final": final, "track_at_ms": track_at, "stats": stats},
        "verdicts": {"offer_answer": offer_verdict, "ice": ice_verdict, "udp": udp_verdict},
        "errors": errors,
    }
    try:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
    except OSError as e:
        print("[webrtc_diag] 결과 파일 쓰기 실패: %s" % e, file=sys.stderr)
        errors.append("결과 파일 쓰기 실패: %s" % e)

    # ---- 사람 요약 ----
    print("[webrtc_diag] url=%s agent=%s headed=%s token=%s" %
          (args.url, agent, args.headed, mask_token(token)))
    if agent_note:
        print("  agent_note: %s" % agent_note)
    print("  baseline STUN(udp %s:%s): ok=%s rtt_ms=%s" %
          (host, UDP_PORT_LO, baseline.get("ok"), baseline.get("rtt_ms")))
    print("  answerInfo: %s" % json.dumps(answer_info, ensure_ascii=False))
    if setup_error:
        print("  setup_error: %s" % setup_error)
    print("  local_cands=%d건 target_ports=%s" % (len(local_cands), list(target_ports)))
    print("  ICE 타임라인:")
    for e in log:
        print("    t=%sms %s ice=%s conn=%s sig=%s gather=%s" %
              (e.get("t"), e.get("ev"), e.get("ice"), e.get("conn"),
               e.get("sig"), e.get("gather")))
    print("  final: ice=%s conn=%s t=%sms" % (final.get("ice"), final.get("conn"), final.get("t")))
    print("  trackAt=%sms" % track_at)
    if stats:
        print("  stats: %s" % json.dumps(stats, ensure_ascii=False))
    else:
        print("  stats: 없음(연결 실패 시 미측정)")
    if successes:
        print("  STUN 응답 %d건:" % len(successes))
        for s_ in successes:
            print("    port=%s rtt_ms=%s at=%ss" %
                  (s_.get("port"), s_.get("rtt_ms"), s_.get("at_elapsed_s")))
    else:
        print("  STUN 응답 0건")
    print("  판정:")
    print("    [offer/answer] %s" % offer_verdict)
    print("    [ICE] %s" % ice_verdict)
    print("    [UDP] %s" % udp_verdict)
    print("  JSON=%s" % args.out)
    for e in errors:
        print("  오류: %s" % e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
