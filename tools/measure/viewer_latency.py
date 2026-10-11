#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""viewer_latency.py — DocViewer 뷰어 세션 1회의 브라우저 측 end-to-end 실측.

헤드리스 Chromium(Python-Playwright)으로 로그인 → 좌석 클릭 → SECONDS 초 관측:
  - /admin/cmd fetch 시각(fetch 래퍼) → click→cmd 마일스톤
  - RTCPeerConnection 인스턴스 수집 + getStats(candidate-pair / inbound-rtp video)
  - video.srcObject 최초 SET 시각(HTMLVideoElement.prototype accessor 패치)
  - requestVideoFrameCallback 프레임 도착(pnow / mediaTime / presentedFrames)
  - videoWidth>0 최초 시각(firstVisual)

한계(필독):
  - getStats RTT/jitter는 세션 지표이고 캡처→표시 절대 지연은 측정 불가(송신측 타임스탬프 필요).
    click→firstVisual은 전송+인코딩+디코딩+표시 종합.
  - 시계: page performance.now()는 브라우저 로컬 시계이고 서버 로그는 ISO다.
    두 기록을 상관하려면 양쪽 시계가 NTP 동기화돼 있을 때만 한다.
    (meta.wall_ms_start = 클릭 시점 브라우저 Date.now() — perf.now 타임라인과의 정합용)

설정은 env/인자만 읽는다: ADMIN_TOKEN(또는 ADMIN_TOKEN_FILE), VIEWER_URL, SEAT, SECONDS, OUT.
토큰은 절대 출력하지 않는다(마스크: 앞 4자리 + "***").
"""
from __future__ import annotations

import argparse
import json
import math
import os
import sys
import time
from datetime import datetime
from typing import Any, Optional

try:
    from playwright.sync_api import sync_playwright
except ImportError:
    print("[viewer_latency] playwright 미설치 — "
          "pip install playwright && playwright install chromium", file=sys.stderr)
    sys.exit(2)

DEF_URL = "https://45.32.251.5:8443"
DEF_SEAT = "PC-045"
DEF_SECONDS = 45
DEF_OUT = "./viewer_latency_result.json"

# 페이지 로드 전 주입되는 계측 JS (add_init_script — 페이지 스크립트보다 먼저 실행됨).
INIT_JS = r"""
(() => {
  const M = { fetchLog: [], rvc: [], pcs: [], srcObjectAt: null, firstVisualAt: null };
  window.__m = M;
  window.__pcs = M.pcs;

  // (1) fetch 래퍼 — /admin/cmd 호출 기록 {url, method, t_start, t_end, status}
  const origFetch = window.fetch ? window.fetch.bind(window) : null;
  if (origFetch) {
    window.fetch = function (...args) {
      const target = args[0];
      const url = String((target && typeof target === "object" && target.url) || target || "");
      const method = String((args[1] && args[1].method) || (target && target.method) || "GET");
      const t_start = performance.now();
      const record = (status) => {
        if (url.indexOf("/admin/cmd") >= 0) {
          M.fetchLog.push({ url: url, method: method,
                            t_start: t_start, t_end: performance.now(), status: status });
        }
      };
      return origFetch(...args).then(
        (r) => { record(r.status); return r; },
        (e) => { record(null); throw e; }
      );
    };
  }

  // (2) RTCPeerConnection 래퍼 — 인스턴스를 window.__pcs 에 수집(인자는 그대로 패스스루)
  if (typeof RTCPeerConnection === "function") {
    const OrigPC = RTCPeerConnection;
    class WrappedPC extends OrigPC {
      constructor(...args) { super(...args); M.pcs.push(this); }
    }
    window.RTCPeerConnection = WrappedPC;
  }

  // (3) srcObject setter 패치 — 최초 SET 시각 기록(원본 descriptor 로 위임).
  //     Chromium 은 srcObject descriptor 가 HTMLMediaElement.prototype 에 있으므로
  //     HTMLVideoElement.prototype 우선, 없으면 HTMLMediaElement.prototype 로 폴백.
  try {
    for (const proto of [HTMLVideoElement.prototype, HTMLMediaElement.prototype]) {
      const d = Object.getOwnPropertyDescriptor(proto, "srcObject");
      if (d && d.get && d.set) {
        Object.defineProperty(proto, "srcObject", {
          configurable: true,
          enumerable: d.enumerable,
          get: function () { return d.get.call(this); },
          set: function (v) {
            if (M.srcObjectAt === null) M.srcObjectAt = performance.now();
            d.set.call(this, v);
          }
        });
        break;
      }
    }
  } catch (e) { /* 패치 실패 시에도 나머지 계측은 유효 */ }

  // (4) video.live-video 100ms 폴링 — rVFC 루프 부착 + firstVisualAt(videoWidth>0 최초 시각).
  //     video 요소는 app.js 가 경로 전환 시 새로 만들 수 있어 WeakSet 로 요소별 부착.
  const attached = new WeakSet();
  const check = () => {
    const vids = document.querySelectorAll("video.live-video");
    for (const v of vids) {
      if (v.videoWidth > 0 && M.firstVisualAt === null) M.firstVisualAt = performance.now();
      if (!attached.has(v) && typeof v.requestVideoFrameCallback === "function") {
        attached.add(v);
        const onFrame = (now, meta) => {
          if (M.rvc.length < 2000) {
            M.rvc.push({ pnow: performance.now(), mediaTime: meta.mediaTime,
                         presentedFrames: meta.presentedFrames });
          }
          if (v.videoWidth > 0 && M.firstVisualAt === null) M.firstVisualAt = performance.now();
          v.requestVideoFrameCallback(onFrame);
        };
        v.requestVideoFrameCallback(onFrame);
      }
    }
  };
  setInterval(check, 100);
})();
"""

# 2초 주기 샘플 — withStats=true 일 때만 pc.getStats() (start/mid/end 3회 스냅샷).
SAMPLE_JS = r"""
async (withStats) => {
  const out = { t_ms: performance.now(), video: null, quality: null, pc: null };
  const v = document.querySelector("video.live-video");
  if (v) {
    let path = "other";
    if (v.srcObject) path = "webrtc";
    else if (String(v.src || "").indexOf("blob:") === 0) path = "mse";
    out.video = { exists: true, videoWidth: v.videoWidth, videoHeight: v.videoHeight,
                  currentTime: v.currentTime, paused: v.paused,
                  readyState: v.readyState, path: path,
                  buffered_ahead: (v.buffered && v.buffered.length) ? +(v.buffered.end(v.buffered.length - 1) - v.currentTime).toFixed(2) : -1 };
    try {
      const q = v.getVideoPlaybackQuality();
      out.quality = { total: q.totalVideoFrames, dropped: q.droppedVideoFrames };
    } catch (e) { out.quality = null; }
  } else {
    out.video = { exists: false, videoWidth: 0, videoHeight: 0,
                  currentTime: 0, paused: null, readyState: 0, path: null };
  }
  if (withStats && window.__pcs && window.__pcs.length) {
    let pc = null;
    for (let i = window.__pcs.length - 1; i >= 0; i--) {
      const c = window.__pcs[i];
      try { if (c.connectionState !== "closed") { pc = c; break; } } catch (e) {}
    }
    if (pc) {
      try {
        const stats = await pc.getStats();
        let cp = null, inb = null;
        stats.forEach((r) => {
          if (r.type === "candidate-pair" && (r.state === "succeeded" || r.state === "in-progress")) {
            if (!cp || r.state === "succeeded") cp = r;
          }
          if (r.type === "inbound-rtp" && (r.kind === "video" || r.mediaType === "video")) inb = r;
        });
        out.pc = {
          connectionState: pc.connectionState,
          candidate_pair: cp ? {
            state: cp.state,
            currentRoundTripTime: (cp.currentRoundTripTime === undefined ? null : cp.currentRoundTripTime),
            availableOutgoingBitrate: (cp.availableOutgoingBitrate === undefined ? null : cp.availableOutgoingBitrate)
          } : null,
          inbound_video: inb ? {
            jitter: inb.jitter, framesDecoded: inb.framesDecoded, framesDropped: inb.framesDropped,
            framesPerSecond: inb.framesPerSecond, pliCount: inb.pliCount, nackCount: inb.nackCount,
            packetsReceived: inb.packetsReceived, bytesReceived: inb.bytesReceived
          } : null
        };
      } catch (e) { out.pc = { error: String((e && e.message) || e) }; }
    }
  }
  return out;
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


def pct(vals, p):
    """단순 보간 백분위(빈 리스트 → None)."""
    if not vals:
        return None
    s = sorted(vals)
    if len(s) == 1:
        return float(s[0])
    k = (len(s) - 1) * p / 100.0
    f = math.floor(k)
    c = math.ceil(k)
    if f == c:
        return float(s[f])
    return s[f] + (s[c] - s[f]) * (k - f)


def find_seat_row(page, seat: str, timeout_s: float = 15.0):
    """보이는(.dev-item) 행 중 .dev-name 텍스트에 seat 가 포함된 첫 행 Locator 반환."""
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        rows = page.locator("#dev-list .dev-item")
        try:
            n = rows.count()
        except Exception:
            n = 0
        for i in range(n):
            row = rows.nth(i)
            try:
                if not row.is_visible():
                    continue  # 숨은 행 스킵
                name = row.locator(".dev-name").text_content() or ""
            except Exception:
                continue
            if seat in name:
                return row
        page.wait_for_timeout(500)
    return None


def _default_channel() -> "str | None":
    """실제 Chrome 채널 우선 — 시스템 Chrome은 H.264를 포함하지만 Playwright 번들
    Chromium은 미포함이라 MSE/WebRTC H264 디코드가 실패할 수 있음. PW_CHANNEL 강제 가능."""
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


def main() -> int:
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass
    parser = argparse.ArgumentParser(description="DocViewer 뷰어 세션 브라우저 측 실측")
    parser.add_argument("--token", default="", help="ADMIN_TOKEN (env ADMIN_TOKEN 도 사용)")
    parser.add_argument("--token-file", default="", help="토큰 파일 경로 (env ADMIN_TOKEN_FILE)")
    parser.add_argument("--url", default=env_default("VIEWER_URL", DEF_URL))
    parser.add_argument("--seat", default=env_default("SEAT", DEF_SEAT))
    parser.add_argument("--seconds", type=int, default=int(env_default("SECONDS", str(DEF_SECONDS))))
    parser.add_argument("--out", default=env_default("OUT", DEF_OUT))
    parser.add_argument("--headed", action="store_true",
                        default=(os.environ.get("HEADED", "").strip() in ("1", "true", "TRUE")),
                        help="헤드리스 해제(디버깅). env HEADED=1 동일.")
    args = parser.parse_args()

    if args.seconds < 1:
        print("[viewer_latency] --seconds 는 1 이상이어야 함", file=sys.stderr)
        return 2

    token, tok_err = resolve_token(args)
    if not token:
        print("[viewer_latency] 토큰 없음 — %s" % (tok_err or "사유 미상"), file=sys.stderr)
        return 2

    started_at_iso = datetime.now().astimezone().isoformat(timespec="milliseconds")
    errors = []
    t_click = None
    wall_ms_start = None
    row_online_at_click = None
    online_wait_ms = None
    samples = []
    m = None
    codec_info = None

    try:
        with sync_playwright() as p:
            channel = _default_channel()
            print("[viewer_latency] browser channel=%s" % (channel or "bundled"))
            browser = p.chromium.launch(headless=not args.headed, channel=channel)
            context = browser.new_context(ignore_https_errors=True,
                                          viewport={"width": 1280, "height": 800})
            context.add_init_script(INIT_JS)  # 페이지 로드 전 계측 주입
            page = context.new_page()

            page.goto(args.url, wait_until="domcontentloaded", timeout=30000)
            page.fill("#login-token", token)
            page.click("#login-btn")
            page.wait_for_selector("#dev-list .dev-item", timeout=15000)
            try:
                codec_info = page.evaluate(
                    "() => ({ ua: navigator.userAgent,"
                    " h264_mse: !!(window.MediaSource && MediaSource.isTypeSupported"
                    " && MediaSource.isTypeSupported('video/mp4; codecs=\"avc1.42E01E\"')),"
                    " h264_el: document.createElement('video').canPlayType("
                    " 'video/mp4; codecs=\"avc1.42E01E\"') || '' })")
            except Exception as e:
                codec_info = {"error": "%s: %s" % (type(e).__name__, e)}

            row = find_seat_row(page, args.seat)
            if row is None:
                raise RuntimeError("좌석 행 미발견(보이는 행 중 .dev-name 에 '%s' 없음)" % args.seat)

            # 게이트 안전장치: 행의 초록 점(.dev-dot.online)을 기다린다 — roster가 winhips보다
            # 늦게 도착하면 online=false 행이라 클릭이 showIdle로 무시된다(관측된 널 측정 원인).
            t_online_wait = time.monotonic()
            row_online_at_click = False
            for _ in range(40):  # 최대 10초
                try:
                    dot_cls = row.locator(".dev-dot").get_attribute("class") or ""
                except Exception:
                    dot_cls = ""
                if "online" in dot_cls:
                    row_online_at_click = True
                    break
                page.wait_for_timeout(250)
            online_wait_ms = round((time.monotonic() - t_online_wait) * 1000, 1)
            if not row_online_at_click:
                errors.append("행 초록 점 대기 10초 초과 (online=false 상태로 클릭 진행)")

            t_click = page.evaluate("() => performance.now()")
            wall_ms_start = page.evaluate("() => Date.now()")
            row.click()

            # ---- SECONDS 초 관측: 2초 샘플 (getStats는 start/mid/end + 디코딩 정체 시 추가) ----
            t_loop = time.monotonic()
            mid_at = t_loop + args.seconds / 2.0
            stats_taken = 0
            last_decoded = None
            last_stats_at = 0.0
            while True:
                now = time.monotonic()
                is_end = (t_loop + args.seconds) - now <= 0
                is_start = (not is_end) and stats_taken == 0
                is_mid = (not is_end) and (not is_start) and stats_taken == 1 and now >= mid_at
                role = "end" if is_end else ("start" if is_start else ("mid" if is_mid else "periodic"))
                with_stats = is_start or is_mid or is_end
                # 디코딩 정체 감시: 마지막 getStats 이후 ≥6s 경과하면 framesDecoded 확인용 추가 getStats
                if (not with_stats) and last_decoded is not None and (now - last_stats_at) >= 6.0:
                    with_stats = True
                    role = role + "+stallcheck"
                try:
                    s = page.evaluate(SAMPLE_JS, with_stats)
                except Exception as e:
                    errors.append("샘플링 실패(%s): %s: %s" % (role, type(e).__name__, e))
                    s = {"t_ms": None, "video": None, "quality": None, "pc": None}
                s["role"] = role
                samples.append(s)
                if with_stats:
                    stats_taken += 1
                    last_stats_at = now
                    inb = ((s.get("pc") or {}).get("inbound_video") or {})
                    fd = inb.get("framesDecoded")
                    if isinstance(fd, (int, float)):
                        if last_decoded is not None and fd == last_decoded and s.get("video") \
                                and (s["video"].get("videoWidth") or 0) > 0:
                            errors.append("디코딩 정체 의심(프레임 미증가) t=%.1fs framesDecoded=%s" % (s.get("t_ms") or 0, fd))
                        last_decoded = fd
                if is_end:
                    break
                page.wait_for_timeout(2000)

            try:
                m = page.evaluate(
                    "() => ({ fetchLog: window.__m.fetchLog, rvc: window.__m.rvc,"
                    " srcObjectAt: window.__m.srcObjectAt, firstVisualAt: window.__m.firstVisualAt })")
            except Exception as e:
                errors.append("__m 수집 실패: %s: %s" % (type(e).__name__, e))
            try:
                context.close()
                browser.close()
            except Exception:
                pass
    except Exception as e:
        errors.append("측정 흐름 실패: %s: %s" % (type(e).__name__, e))

    # ---- 마일스톤/통계 계산 (부분 데이터 허용) ----
    milestones = {"click_to_cmd": None, "cmd_to_srcObject": None,
                  "cmd_to_firstVisual": None, "click_to_firstVisual": None}
    t_cmd_ok = None
    if m:
        for e in m.get("fetchLog") or []:
            if e.get("status") == 200:
                t_cmd_ok = e.get("t_end")
                break
        t_src = m.get("srcObjectAt")
        t_fv = m.get("firstVisualAt")

        def dur(a, b):
            if a is None or b is None:
                return None
            return round(b - a, 1)

        milestones = {
            "click_to_cmd": dur(t_click, t_cmd_ok),
            "cmd_to_srcObject": dur(t_cmd_ok, t_src),
            "cmd_to_firstVisual": dur(t_cmd_ok, t_fv),
            "click_to_firstVisual": dur(t_click, t_fv),
        }

    rvc_stats = {"count": 0, "p50_ms": None, "p95_ms": None, "max_ms": None,
                 "min_ms": None, "fps_estimate": None, "media_time_span_s": None}
    if m:
        rvc = m.get("rvc") or []
        pnows = [e.get("pnow") for e in rvc if isinstance(e.get("pnow"), (int, float))]
        rvc_stats["count"] = len(rvc)
        if len(pnows) >= 2:
            deltas = [b - a for a, b in zip(pnows, pnows[1:])]
            rvc_stats["p50_ms"] = round(pct(deltas, 50), 1)
            rvc_stats["p95_ms"] = round(pct(deltas, 95), 1)
            rvc_stats["max_ms"] = round(max(deltas), 1)
            rvc_stats["min_ms"] = round(min(deltas), 1)
            span_s = (pnows[-1] - pnows[0]) / 1000.0
            if span_s > 0:
                rvc_stats["fps_estimate"] = round((len(pnows) - 1) / span_s, 2)
            mts = [e.get("mediaTime") for e in rvc if isinstance(e.get("mediaTime"), (int, float))]
            if len(mts) >= 2:
                rvc_stats["media_time_span_s"] = round(mts[-1] - mts[0], 3)
        rvc_entries = [e for e in rvc if isinstance(e.get("pnow"), (int, float))]
        stalls = []
        for i in range(1, len(rvc_entries)):
            gap = rvc_entries[i]["pnow"] - rvc_entries[i - 1]["pnow"]
            if gap >= 500.0:
                stalls.append({"t_ms": round(rvc_entries[i]["pnow"], 1), "gap_ms": round(gap, 1),
                               "mediaTime": rvc_entries[i].get("mediaTime")})
        rvc_stats["stalls"] = stalls

    video_quality = None
    path = None
    for s in reversed(samples):
        vid = s.get("video") or {}
        if path is None and vid.get("exists"):
            path = vid.get("path")
        if video_quality is None and s.get("quality"):
            video_quality = {"total": s["quality"].get("total"),
                             "dropped": s["quality"].get("dropped")}
        if path is not None and video_quality is not None:
            break

    stats_samples = []
    prev_fd = None
    prev_t = None
    for s in samples:
        if s.get("role") not in ("start", "mid", "end"):
            continue
        entry = {"role": s.get("role"), "t_ms": s.get("t_ms"),
                 "video": s.get("video"), "quality": s.get("quality"), "pc": s.get("pc")}
        pc = s.get("pc") if isinstance(s.get("pc"), dict) else {}
        inv = (pc.get("inbound_video") or {}) if pc else {}
        fd = inv.get("framesDecoded")
        rate = None
        if (prev_fd is not None and fd is not None and prev_t is not None
                and isinstance(s.get("t_ms"), (int, float)) and s["t_ms"] > prev_t):
            rate = (fd - prev_fd) / ((s["t_ms"] - prev_t) / 1000.0)
        entry["framesDecoded_rate"] = round(rate, 2) if rate is not None else None
        if fd is not None:
            prev_fd, prev_t = fd, s["t_ms"]
        stats_samples.append(entry)

    video_samples = [{"role": s.get("role"), "t_ms": s.get("t_ms"),
                      "video": s.get("video"), "quality": s.get("quality")} for s in samples]
    fetch_log_count = len((m.get("fetchLog") if m else None) or [])
    ended_at_iso = datetime.now().astimezone().isoformat(timespec="milliseconds")

    result = {
        "meta": {"url": args.url, "seat": args.seat, "seconds": args.seconds,
                 "token_used": mask_token(token), "started_at_iso": started_at_iso,
                 "wall_ms_start": wall_ms_start, "ended_at_iso": ended_at_iso,
                 "click_perf_ms": t_click,
                 "row_online_at_click": row_online_at_click,
                 "online_wait_ms": online_wait_ms,
                 "codec_support": codec_info, "channel": _default_channel()},
        "milestones_ms": milestones,
        "rvc_stats": rvc_stats,
        "video_quality": video_quality,
        "path": path,
        "stats_samples": stats_samples,
        "video_samples": video_samples,
        "fetch_log_count": fetch_log_count,
        "errors": errors,
    }
    try:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
    except OSError as e:
        print("[viewer_latency] 결과 파일 쓰기 실패: %s" % e, file=sys.stderr)
        errors.append("결과 파일 쓰기 실패: %s" % e)

    # ---- 사람 요약 ----
    print("[viewer_latency] url=%s seat=%s seconds=%s headed=%s" %
          (args.url, args.seat, args.seconds, args.headed))
    print("  경로(path)=%s" % path)
    print("  마일스톤(ms): click→cmd=%s cmd→srcObject=%s cmd→firstVisual=%s click→firstVisual=%s" %
          (milestones["click_to_cmd"], milestones["cmd_to_srcObject"],
           milestones["cmd_to_firstVisual"], milestones["click_to_firstVisual"]))
    print("  rVFC: count=%s p50=%sms p95=%sms max=%sms min=%sms fps≈%s" %
          (rvc_stats["count"], rvc_stats["p50_ms"], rvc_stats["p95_ms"],
           rvc_stats["max_ms"], rvc_stats["min_ms"], rvc_stats["fps_estimate"]))
    stalls = rvc_stats.get("stalls") or []
    print("  스톨(≥500ms): %d건 %s" %
          (len(stalls), [{"t_ms": st.get("t_ms"), "gap_ms": st.get("gap_ms")} for st in stalls[:8]]))
    print("  video_quality: %s" % video_quality)
    print("  fetch_log_count=%s" % fetch_log_count)
    print("  started_at=%s" % started_at_iso)
    print("  ended_at=%s" % ended_at_iso)
    print("  JSON=%s" % args.out)
    for e in errors:
        print("  오류: %s" % e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
