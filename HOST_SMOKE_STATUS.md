# Host Smoke 실행 결과 요약 (Kali 호스트, v1.6.0)

**Date:** 2026-08-03 (lab)
**Runner:** `scripts/host_smoke.sh`
**최신 artifacts:** `data/host_smoke_1785733922/` (01:12 KST)

## 판정표 — FAIL 없음, 5/5 PASS

| # | 단계 | 실측 | 판정 |
|---|------|------|------|
| 1 | health | `server_ver 1.6.0-lab-stage`, crypto:false, transports [wss,http] | ✅ PASS |
| 2 | 에이전트 등록 (wss) | `agent-58e61c1cbc1bd4ef` online:true | ✅ PASS |
| 3 | shell (always-cmd) | status:done, exit:0, `HOST_SMOKE_OK_1785733922` | ✅ PASS |
| 4 | screenshot | HTTP 200 enqueue (gate 제거로 항상 허용) | ✅ PASS |
| 5 | priv | is_admin:false, elev_method:user, uid:1000 | ✅ PASS |

## 참고: 409 stream_gated 게이트 제거 (v1.6.0+)

- 이전(≤v1.4.4)에는 게임 미실행 시 screenshot이 HTTP 409 `stream_gated`로 차단됐음.
- **v1.6.0+에서 게이트 코드 제거 확정** — `game_active`/`stream_enable`은 heartbeat 보고·대시보드 LED 용도로만 사용하고 명령은 항상 허용.
- 관련 갱신: `docs/stream-enable-gate.md`(폐기 표기), `config.lab.env STREAM_GATE=always`, `scripts/host_smoke.sh` step[4], `scripts/smoke_windows.ps1`, `scripts/_hangame_smoke_snippet.ps1`.

## 참고: 이전 panic 검증

- 이전 `concurrent write to websocket connection` panic(02:13)은 과거 바이너리 이슈.
- 현재 소스(06:54)는 WS 쓰기를 `agent.mu`로 직렬화 → 재빌드 후 재검증하며 **panic 0건** 확인.

## 현재 상황

- `host_smoke.sh`는 검증 완료 후 말미의 `kill $AGT $SRV`로 **서버+에이전트를 자동 종료**하고 끝남.
- 따라서 실행 직후에는 서버가 꺼져 있어 `/health` 접속이 실패하는 것처럼 보임 — 다운이 아니라 정상 종료.

## 개선 옵션

1. **상시 기동**: 스크립트 말미의 종료(kill) 블록을 제거하거나, 서버만 별도 nohup 기동.
2. **screenshot 실성공 검증**: Linux agent는 `screenshot not implemented`를 반환 — Windows VM `smoke_windows.ps1`에서 실캡처 검증 필요.

## 재현 1줄

```bash
cd /home/kali/Projects/c2-project && bash scripts/host_smoke.sh
```
