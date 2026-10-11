# tools/measure — 뷰어 지연 · GOP A/B · CPU 프로파일 측정

1 vCPU 비디오 릴레이(c2push → c2server → 브라우저)의 지연 원인 분해용 측정 스크립트.
서버 경계 세그먼트 배치는 이미 p50 1.035s(GOP 구동)로 측정돼 있어, 남은 물음을 아래 순서로 답는다.

## 측정 목적과 4단계 실행 순서

1. **브라우저 실측** — `viewer_latency.py`(로컬 실행): 헤드리스 Chromium으로 좌석 1개 시청 세션의
   click→firstVisual 종합 지연, rVFC 프레임 도착 간격, getStats 세션 품질을 잰다.
2. **GOP A/B** — `gop_ab.sh`(VPS root): 같은 좌석에서 GOP 15(현행) vs 5를 비교 — 갭 p50/p95, BPS, ffmpeg CPU.
3. **CPU 프로파일링** — `profile_c2push.sh`(VPS root): c2push ~26% CPU의 함수별 분포(py-spy) — Go 재작성 판단 전.
4. **프레임 단위 전송 판단** — 1~3 결과로 GOP 축소/프레임 단위 전송(또는 Go 재작성) 여부를 결정한다.
   (보조: `server_cadence.py`(VPS) — 브라우저 측정과 같은 윈도의 서버 경계 배치/CPU/waited_ms)

## 실행 커맨드 예시

토큰은 항상 환경변수로만 넣는다 — 스크립트 본문/커밋/로그에 절대 적지 않는다.

VPS(서버)에서:

```bash
export ADMIN_TOKEN=$(grep ^ADMIN_TOKEN= /root/c2/env.systemd | cut -d= -f2)
python3 tools/measure/server_cadence.py                # 서버 경계 배치 + CPU + waited_ms
./tools/measure/gop_ab.sh onpxe-PC-045 5               # GOP=5 팔
./tools/measure/gop_ab.sh onpxe-PC-045 15              # GOP=15 베이스라인 팔
./tools/measure/profile_c2push.sh                      # c2push CPU 프로파일(py-spy)
```

로컬(브라우저 실측)에서:

```bash
export ADMIN_TOKEN=...        # 로컬 셸에서 직접 export (파일 저장 금지)
python tools/measure/viewer_latency.py --seat PC-045 --seconds 45
```

## 산출물 위치

- `./viewer_latency_result.json` — milestones_ms, rvc_stats, stats_samples, video_samples (viewer_latency.py)
- `./server_cadence_result.json` — arrivals, gap_stats, bps, cpu_start/end, waited_ms_stats (server_cadence.py)
- `/tmp/c2push_profile.json`(speedscope), `/tmp/c2push_pyspy_top.txt` (profile_c2push.sh)
- `gop_ab.sh` 은 요약 표를 stdout에 출력한다(파일 미생성)

## 지표 해석 가이드

- `click→firstVisual` = 전송+인코딩+디코딩+표시의 **종합** 지연이다. 캡처→표시 절대 지연이 아니다.
- 갭 p50(서버 경계) = 세그먼트가 서버에 도착하는 배치 간격(GOP 구동 — GOP 15@15fps면 ≈1s).
- getStats RTT/jitter = 세션 품질 지표(절대 지연 아님).
- 캡처→표시 절대 지연이 필요하면 향후 옵션: 송신측 타임스탬프 주입(트랜스포트) 또는 화면 시계 대조 촬영.

## GOP 실험 시 주의

- 1 vCPU 서버: 실험 팔 동안 다른 시청 세션/스트림을 끊고 한 좌석만 돌린다(안정화) —
  ffmpeg CPU 합이 팔 간 비교를 오염시킨다.
- `gop_ab.sh` 는 c2push.py를 임시 패치한다(env-hook). 실험 후 원복
  (`mv /root/c2/c2push.py.gopbak /root/c2/c2push.py && systemctl restart c2push`) 또는 정식 버전 커밋 필수.
