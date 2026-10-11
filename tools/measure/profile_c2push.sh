#!/bin/bash
# c2push CPU 프로파일 (py-spy) — ~26% CPU가 어디서 오는지 판별 (AES-GCM/복사/스레드 vs 구조)
# Go 이전 가치 판단용. 사용법: ./profile_c2push.sh [--record]
#   기본: py-spy top 10초 (인간용 상위 함수 테이블)
#   --record: speedscope JSON 30초 녹화 → https://www.speedscope.app 에서 열기
set -u
[ "$(id -u)" -eq 0 ] || { echo "[abort] root 필요"; exit 1; }
PID=$(systemctl show -p MainPID --value c2push)
if [ -z "$PID" ] || [ "$PID" = "0" ]; then echo "[abort] c2push 프로세스 없음"; exit 1; fi

PYSPY=$(command -v py-spy || true)
if [ -z "$PYSPY" ]; then
  echo "[install] py-spy 설치 시도..."
  pip3 install --quiet py-spy 2>/dev/null || pip3 install --quiet --break-system-packages py-spy 2>/dev/null || true
  PYSPY=$(command -v py-spy || true)
fi
if [ -z "$PYSPY" ]; then
  echo "[abort] py-spy 설치 실패 — 수동 설치: pip3 install py-spy"
  exit 1
fi
echo "[info] pid=$PID py-spy=$PYSPY"

echo "=== py-spy top (10s, 상위 40행) ==="
$PYSPY top --pid "$PID" --duration 10 2>&1 | head -40

REC=""
if [ "${1:-}" = "--record" ]; then
  OUT=/tmp/c2push_profile_$(date +%Y%m%d_%H%M%S).json
  echo "=== py-spy record (30s, speedscope format) → $OUT ==="
  $PYSPY record --pid "$PID" --duration 30 --rate 100 --format speedscope -o "$OUT" 2>&1 | tail -3
  echo "[info] 열기: https://www.speedscope.app 에서 $OUT 파일 drag&drop"
  REC="$OUT"
fi

echo "=== 부하 스냅샷 ==="
ps -eo comm,%cpu,%mem --sort=-%cpu | head -8
echo "RSS(c2push): $(ps -o rss= -p "$PID") KB"
uptime | tail -1

echo "=== 요약 ==="
echo "record=${REC:-skip} | 해석: top 테이블에서 seal/openenv/json.dumps/seal_raw/프레임복사 라인이 상위면 암호화·직렬화 비용, 그 외엔 스레드/네트워크"
