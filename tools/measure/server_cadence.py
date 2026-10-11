#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""server_cadence.py — VPS 실행: 서버 경계 세그먼트 도착 배치 + CPU 스냅샷 + 롱폴 waited_ms 분포.

viewer_latency.py(브라우저 실측)와 같은 윈도를 서버 관점에서 잡는다.
  1. POST /admin/cmd stream_start — app.js sendCmd 와 동일한 바디 형태
     {agent_id, cmd_type:"stream_start", payload:"<json 문자열>"}
  2. GET /admin/stream?format=raw 롱폴 — app.js pollMSE/handleH264Raw 와 동일 프로토콜.
     바이너리 레코드: kind(1)|stream(8,LE)|seq(8,LE)|codecLen(1)|codec|payloadLen(4,LE)|payload
     kind==2(media)는 seq>since 일 때만 since 전진. 200+본문만 도착으로 기록(204/404는 별도 카운트).
  3. 도착 간격 gap min/p50/p95/max + 총 바이트 + BPS + 미디어 세그/초
  4. CPU 스냅샷(시작/종료): ps 상위 10줄 + uptime
  5. journalctl -u c2server 의 waited_ms=(\\d+) 분포(n/p50/p95/max) — journalctl 없으면 스킵 노트

토큰: ADMIN_TOKEN env 또는 /root/c2/env.systemd(ADMIN_TOKEN= 무인용 값). 절대 출력하지 않는다.
"""
from __future__ import annotations

import argparse
import json
import math
import os
import re
import ssl
import struct
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime
from typing import Optional

DEF_URL = "https://127.0.0.1:8443"
DEF_SEAT = "onpxe-PC-045"
DEF_SECONDS = 45
DEF_OUT = "./server_cadence_result.json"
DEF_ENV_FILE = "/root/c2/env.systemd"


def env_default(name: str, default: str) -> str:
    v = os.environ.get(name)
    return v if v else default


def now_iso_ms() -> str:
    return datetime.now().astimezone().isoformat(timespec="milliseconds")


def resolve_token(args: argparse.Namespace) -> "tuple[Optional[str], Optional[str]]":
    """--token 인자 > ADMIN_TOKEN env > --token-file/ADMIN_TOKEN_FILE > /root/c2/env.systemd 순."""
    if args.token:
        return args.token.strip(), None
    env_tok = os.environ.get("ADMIN_TOKEN", "").strip()
    if env_tok:
        return env_tok, None
    tf = (args.token_file or os.environ.get("ADMIN_TOKEN_FILE", "") or "").strip()
    if not tf:
        tf = DEF_ENV_FILE
    if not os.path.isfile(tf):
        return None, ("토큰 파일 없음: %s — ADMIN_TOKEN env 또는 --token 으로 제공 "
                      "(VPS 가 아니면 env 로 실행)" % tf)
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


def parse_records(buf: bytes):
    """kind(1)|stream(8,LE)|seq(8,LE)|codecLen(1)|codec|payloadLen(4,LE)|payload 파싱.

    반환: (init 수, media 수, 관측된 최대 media seq — 없으면 None)
    """
    n = len(buf)
    off = 0
    inits = 0
    medias = 0
    max_seq = None
    while off + 18 <= n:
        kind = buf[off]
        seq = struct.unpack_from("<Q", buf, off + 9)[0]
        cl = buf[off + 17]
        if off + 22 + cl > n:
            break
        plen = struct.unpack_from("<I", buf, off + 18 + cl)[0]
        poff = off + 22 + cl
        if poff + plen > n:
            break
        if kind == 1:
            inits += 1
        elif kind == 2:
            medias += 1
            if max_seq is None or seq > max_seq:
                max_seq = seq
        off = poff + plen
    return inits, medias, max_seq


def cpu_snapshot() -> dict:
    """ps 상위 10줄(헤더 포함) + uptime."""
    snap = {"ps": [], "uptime": ""}
    try:
        r = subprocess.run(["ps", "-eo", "comm,%cpu,%mem", "--sort=-%cpu"],
                           capture_output=True, text=True, timeout=10)
        snap["ps"] = r.stdout.splitlines()[:10]
    except Exception as e:
        snap["ps_error"] = "%s: %s" % (type(e).__name__, e)
    try:
        r = subprocess.run(["uptime"], capture_output=True, text=True, timeout=10)
        snap["uptime"] = r.stdout.strip()
    except Exception as e:
        snap["uptime_error"] = "%s: %s" % (type(e).__name__, e)
    return snap


def waited_ms_stats(since_iso: str) -> dict:
    """journalctl -u c2server --since <start> 에서 waited_ms=(\\d+) 분포."""
    try:
        r = subprocess.run(["journalctl", "-u", "c2server", "--since", since_iso, "--no-pager"],
                           capture_output=True, text=True, timeout=60)
    except FileNotFoundError:
        return {"note": "journalctl 없음 — 스킵"}
    except Exception as e:
        return {"note": "journalctl 실행 실패: %s: %s" % (type(e).__name__, e)}
    if r.returncode != 0:
        return {"note": "journalctl rc=%d" % r.returncode}
    vals = [float(x) for x in re.findall(r"waited_ms=(\d+(?:\.\d+)?)", r.stdout)]
    if not vals:
        return {"n": 0, "note": "구간 내 waited_ms 로그 없음"}
    return {"n": len(vals), "min_ms": min(vals), "p50_ms": pct(vals, 50),
            "p95_ms": pct(vals, 95), "max_ms": max(vals)}


def post_stream_start(base: str, seat: str, token: str, ctx) -> int:
    """app.js sendCmd 와 동일한 바디로 stream_start 전송 — HTTP status 반환."""
    body = json.dumps({
        "agent_id": seat,
        "cmd_type": "stream_start",
        "payload": json.dumps({"fps": 15, "quality": 70, "max_dim": 1280}),
    }).encode()
    req = urllib.request.Request(
        base + "/admin/cmd", data=body, method="POST",
        headers={"Content-Type": "application/json", "X-Auth-Token": token})
    try:
        with urllib.request.urlopen(req, timeout=10, context=ctx) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception:
        return -1


def main() -> int:
    parser = argparse.ArgumentParser(description="서버 경계 세그먼트 배치/CPU/waited_ms 측정 (VPS)")
    parser.add_argument("--token", default="", help="ADMIN_TOKEN (env ADMIN_TOKEN 도 사용)")
    parser.add_argument("--token-file", default="",
                        help="토큰 파일 경로 (기본: env ADMIN_TOKEN_FILE → /root/c2/env.systemd)")
    parser.add_argument("--url", default=env_default("VIEWER_URL", DEF_URL))
    parser.add_argument("--seat", default=env_default("SEAT", DEF_SEAT))
    parser.add_argument("--seconds", type=int, default=int(env_default("SECONDS", str(DEF_SECONDS))))
    parser.add_argument("--out", default=env_default("OUT", DEF_OUT))
    args = parser.parse_args()

    token, tok_err = resolve_token(args)
    if not token:
        print("[server_cadence] 토큰 없음 — %s" % (tok_err or "사유 미상"), file=sys.stderr)
        return 2

    ctx = ssl._create_unverified_context()  # 자체 서명 인증서(8443) 통과
    errors = []
    iso_start = now_iso_ms()
    cpu_start = cpu_snapshot()
    arrivals = []
    mono = []
    counts = {"http_204": 0, "http_404": 0, "http_other": 0,
              "empty_200": 0, "timeout_or_exc": 0}

    status = post_stream_start(args.url, args.seat, token, ctx)
    print("[server_cadence] stream_start HTTP %s (seat=%s)" % (status, args.seat))
    if status != 200:
        errors.append("stream_start HTTP %s — 측정 불가" % status)

    t0 = time.monotonic()
    if not errors:
        since = -1
        while time.monotonic() - t0 < args.seconds:
            url = args.url + "/admin/stream?" + urllib.parse.urlencode(
                {"agent_id": args.seat, "format": "raw", "since_seq": since, "wait_sec": 1})
            req = urllib.request.Request(url, headers={"X-Auth-Token": token})
            try:
                with urllib.request.urlopen(req, timeout=6, context=ctx) as r:
                    if r.status == 200:
                        body = r.read()
                        if body:
                            inits, medias, max_seq = parse_records(body)
                            if max_seq is not None and max_seq > since:
                                since = max_seq
                            arrivals.append({"iso": now_iso_ms(), "bytes": len(body),
                                             "media": medias, "init": inits})
                            mono.append(time.monotonic())
                        else:
                            counts["empty_200"] += 1
                    elif r.status == 204:
                        counts["http_204"] += 1
                    else:
                        counts["http_other"] += 1
            except urllib.error.HTTPError as e:
                if e.code == 404:
                    counts["http_404"] += 1
                else:
                    counts["http_other"] += 1
            except Exception:
                counts["timeout_or_exc"] += 1

    elapsed = time.monotonic() - t0
    gaps_ms = [(b - a) * 1000.0 for a, b in zip(mono, mono[1:])]
    gap_stats = {"count": len(gaps_ms),
                 "min_ms": round(min(gaps_ms), 1) if gaps_ms else None,
                 "p50_ms": round(pct(gaps_ms, 50), 1) if gaps_ms else None,
                 "p95_ms": round(pct(gaps_ms, 95), 1) if gaps_ms else None,
                 "max_ms": round(max(gaps_ms), 1) if gaps_ms else None}
    total_bytes = sum(a["bytes"] for a in arrivals)
    total_media = sum(a["media"] for a in arrivals)
    bps = round(total_bytes / elapsed, 1) if elapsed > 0 else None
    media_per_sec = round(total_media / elapsed, 3) if elapsed > 0 else None

    cpu_end = cpu_snapshot()
    iso_end = now_iso_ms()
    waited = waited_ms_stats(iso_start)

    result = {
        "iso_start": iso_start, "iso_end": iso_end, "seat": args.seat,
        "url": args.url, "seconds": args.seconds, "elapsed_s": round(elapsed, 3),
        "arrivals": arrivals, "gap_stats": gap_stats,
        "bps": bps, "media_total": total_media, "media_per_sec": media_per_sec,
        "counts": counts,
        "cpu_start": cpu_start, "cpu_end": cpu_end,
        "waited_ms_stats": waited,
        "errors": errors,
    }
    try:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
    except OSError as e:
        errors.append("결과 파일 쓰기 실패: %s" % e)
        print("[server_cadence] 결과 파일 쓰기 실패: %s" % e, file=sys.stderr)

    print("[server_cadence] seat=%s elapsed=%.1fs" % (args.seat, elapsed))
    print("  stream_start: HTTP %s" % status)
    print("  arrivals=%d (media=%d, 204=%d, 404=%d, other=%d, empty200=%d, exc=%d)" %
          (len(arrivals), total_media, counts["http_204"], counts["http_404"],
           counts["http_other"], counts["empty_200"], counts["timeout_or_exc"]))
    print("  gap(ms): min=%s p50=%s p95=%s max=%s (n=%d)" %
          (gap_stats["min_ms"], gap_stats["p50_ms"], gap_stats["p95_ms"],
           gap_stats["max_ms"], gap_stats["count"]))
    print("  BPS=%s media/s=%s" % (bps, media_per_sec))
    print("  waited_ms: n=%s p50=%s p95=%s max=%s" %
          (waited.get("n"), waited.get("p50_ms"), waited.get("p95_ms"), waited.get("max_ms")))
    print("  cpu_start top: %s" % ((cpu_start.get("ps") or ["?"])[1:4]))
    print("  cpu_end   top: %s" % ((cpu_end.get("ps") or ["?"])[1:4]))
    print("  uptime(end): %s" % cpu_end.get("uptime"))
    print("  JSON=%s" % args.out)
    for e in errors:
        print("  오류: %s" % e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
