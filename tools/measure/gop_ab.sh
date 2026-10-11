#!/bin/bash
# gop A/B 실험 (VPS root 전용) — GOP 값별 세그먼트 케이던스·ffmpeg CPU·비트레이트 비교
# 사용법: ./gop_ab.sh [agent_id] [gop] [seconds]
#   예: ./gop_ab.sh onpxe-PC-045 5 45   (실험), ./gop_ab.sh onpxe-PC-045 15 45 (기준)
# 주의: 실험 중 c2push 재시작 발생 (좌석 재등록 ~수초 블립). 종료 시 drop-in 제거·기본 GOP 복귀.
# 토큰은 /root/c2/env.systemd에서만 읽으며 절대 출력하지 않음.
set -u
AGENT="${1:-onpxe-PC-045}"
GOP="${2:-5}"
SECS="${3:-45}"
CPY=/root/c2/c2push.py
DROPIN_DIR=/etc/systemd/system/c2push.service.d
DROPIN=$DROPIN_DIR/gop.conf

[ "$(id -u)" -eq 0 ] || { echo "[abort] root 필요"; exit 1; }
[ -f "$CPY" ] || { echo "[abort] $CPY 없음"; exit 1; }
[ -f /root/c2/env.systemd ] || { echo "[abort] env.systemd 없음"; exit 1; }

# 1) gop가 상수면 C2_GOP 환경변수 오버라이드 패치 (백업 유지, git 미반영 실험 코드)
if grep -qE '^[[:space:]]*gop = H264_FPS' "$CPY"; then
  cp -a "$CPY" "$CPY.gopbak"
  sed -i 's/^[[:space:]]*gop = H264_FPS.*/        gop = int(os.environ.get("C2_GOP") or H264_FPS)  # gop_ab experiment hook/' "$CPY"
  echo "[patch] gop → C2_GOP 오버라이드 적용 (백업: $CPY.gopbak)"
elif ! grep -q 'C2_GOP' "$CPY"; then
  echo "[abort] gop = H264_FPS 마커 없음 — c2push.py 수동 확인 필요"; exit 1
else
  echo "[patch] C2_GOP 오버라이드 이미 존재"
fi

# 2) systemd drop-in으로 GOP 지정 후 재시작
mkdir -p "$DROPIN_DIR"
printf '[Service]\nEnvironment=C2_GOP=%s\n' "$GOP" > "$DROPIN"
systemctl daemon-reload
systemctl restart c2push
sleep 8

# 3) 실제 인코더 GOP 관측 확인
OBS=$(ps -eo args | grep 'ffmpeg -hide_banner' | grep -oE '\-g [0-9]+' | head -1)
echo "[gop] 요청=$GOP 관측=${OBS:-없음}"
if [ -z "$OBS" ]; then echo "[warn] ffmpeg 프로세스 없음 — 스트림이 안 켜졌는지 확인"; fi

# 4) CPU 기저 측정 (probe 전)
CPU1=$(ps -eo comm,%cpu --no-headers | awk '$1 ~ /ffmpeg/ {s+=$2} END {printf "%.0f", s+0}')
LOAD1=$(uptime | sed 's/.*load average: //')
SINCE_START=$(date '+%Y-%m-%d %H:%M:%S')

# 5) 세그먼트 케이던스 probe (서버 경계, since_seq 추적 롱폴)
export AGENT_ID="$AGENT" SECS="$SECS"
python3 - <<'PYEOF'
import json, os, ssl, time, urllib.request
tok = ""
for line in open("/root/c2/env.systemd"):
    if line.startswith("ADMIN_TOKEN="):
        tok = line.strip().split("=", 1)[1]
AGENT_ID = os.environ["AGENT_ID"]
SECS = int(os.environ["SECS"])
ctx = ssl.create_default_context(); ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
def req(path, data=None):
    r = urllib.request.Request("https://127.0.0.1:8443" + path, data=data,
        headers={"X-Auth-Token": tok, "Content-Type": "application/json"} if data else {"X-Auth-Token": tok})
    try:
        resp = urllib.request.urlopen(r, context=ctx, timeout=8)
        return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, b""
    except Exception:
        return 0, b""
body = json.dumps({"agent_id": AGENT_ID, "cmd_type": "stream_start",
                   "payload": json.dumps({"fps": 15, "quality": 70, "max_dim": 1280})}).encode()
print("[kick]", req("/admin/cmd", body)[0])
time.sleep(3)
since = -1
arrivals = []
n204 = n404 = 0
t_end = time.time() + SECS
while time.time() < t_end:
    status, data = req("/admin/stream?agent_id=%s&format=raw&since_seq=%d&wait_sec=1" % (AGENT_ID, since))
    if status == 204:
        n204 += 1; continue
    if status == 404 or (status == 200 and not data):
        n404 += 1; continue
    if status == 200 and data:
        off = 0; media = 0
        while off + 18 <= len(data):
            kind = data[off]
            seq = int.from_bytes(data[off+9:off+17], "little")
            cl = data[off+17]
            if off + 22 + cl > len(data): break
            plen = int.from_bytes(data[off+18+cl:off+22+cl], "little")
            poff = off + 22 + cl
            if poff + plen > len(data): break
            if kind == 2:
                media += 1
                if seq > since: since = seq
            off = poff + plen
        if media:
            arrivals.append((time.time(), len(data), media))
def pct(v, p):
    if not v: return None
    s = sorted(v)
    return s[min(int(len(s) * p / 100), len(s) - 1)]
gaps = [round(arrivals[i][0] - arrivals[i-1][0], 3) for i in range(1, len(arrivals))]
total = sum(b for _, b, _ in arrivals)
print("[cadence] arrivals=%d media=%d bytes=%d BPS=%d" % (
    len(arrivals), sum(m for _, _, m in arrivals), total, total // max(SECS, 1)))
print("[cadence] gap min=%s p50=%s p95=%s max=%s | 204=%d 404=%d" % (
    min(gaps) if gaps else None, pct(gaps, 50), pct(gaps, 95),
    max(gaps) if gaps else None, n204, n404))
PYEOF

# 6) CPU 측정 (probe 후) + journal 건강도
CPU2=$(ps -eo comm,%cpu --no-headers | awk '$1 ~ /ffmpeg/ {s+=$2} END {printf "%.0f", s+0}')
LOAD2=$(uptime | sed 's/.*load average: //')
EXC=$(journalctl -u c2push --since "$SINCE_START" --no-pager | grep -cE "EXC|ERR" || true)
RST=$(journalctl -u c2push --since "$SINCE_START" --no-pager | grep -c "encoder reset" || true)

echo "=== SUMMARY ==="
echo "GOP=$GOP | ffmpeg_cpu: $CPU1% → $CPU2% | load: $LOAD1 → $LOAD2 | EXC=$EXC resets=$RST | secs=$SECS seat=$AGENT"

# 7) 정리: drop-in 제거 → 기본 GOP으로 복귀 (백업은 유지)
rm -f "$DROPIN"
systemctl daemon-reload
systemctl restart c2push
echo "[cleanup] C2_GOP drop-in 제거, c2push 재시작(기본 GOP 복귀)"
if [ -f "$CPY.gopbak" ]; then
  echo "[revert] 상수 패치를 되돌릴 때: cp -a $CPY.gopbak $CPY && systemctl restart c2push"
fi
