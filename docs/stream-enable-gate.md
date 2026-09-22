# stream_enable 게이트 — 설계·구현 (v1.4.3) / **폐기됨 (v1.6.0+)**

> 작성: 2026-07-31  
> 버전: agent/server `1.4.3-lab-streamgate`  
> **2026-08-03 변경: 게이트 제거 확정. screenshot/stream 계열 명령은 `game_active`와 무관하게 항상 허용.**

## 상태 변경 이력

- v1.4.3: §3-1 게임 실행 중일 때만 화면 송출 허용 (이중 게이트: server 409 + agent deny).
- **v1.6.0+: 게이트 코드 제거** — `game_active`/`stream_enable`은 heartbeat에 계속 보고되지만
  `screenshot`/`stream_*` 명령을 차단하지 않음. `game_active`는 대시보드 LED 표시용으로만 사용.

## (폐기 전) 원래 설계

### 목표

§3-1 한게임 클라이언트(`game_active=true`)가 **실행 중일 때만** 화면 송출 계열 명령을 허용한다.  
대시보드 LED는 기존과 동일: `green` = active, `gray` = idle.

## 바인딩

| 필드 | 출처 | 의미 |
|------|------|------|
| `game_active` | agent process poll | allowlist 이미지 매칭 (debounce 후) |
| `stream_enable` | `= game_active` | heartbeat에 동시 보고 |
| `dashboard_led` | green/gray | UI 표시 |

프로필 `profiles/hangame_games.json`:

```json
"stream": { "mode": "on_game_active_only" }
```

## 이중 게이트 (폐기 전 동작)

### 1) Server (`POST /admin/cmd`)

- `cmd_type` ∈ `screenshot | stream | stream_start | stream_frame | stream_stop`
- agent 상태가 `stream_enable=false` **그리고** `game_active=false` 이면 **HTTP 409**  
  body: `{ "error":"stream_gated", "reason":"game_active=false", ... }`
- lab 우회: body에 `"force": true` (운영 기본 off)

### 2) Agent (`executeCommand`)

- 동일 cmd_type 수신 시 `scanGames()` 재확인
- inactive → `success=false`, `error=stream_gated: game_active=false...`
- active → lab stub 성공 JSON (`stream_enable:true`, game meta). **실제 캡처 파이프라인은 미구현**

shell/exec/shell_admin/exec_admin/priv/upload/download/ping 등 비-stream 명령은 게이트 대상 아님 (v1.4.4: elevated 경로 포함, 상시 허용).

## 폴링

| 설정 | 기본 | 비고 |
|------|------|------|
| `GAME_POLL_SEC` | **60** | v1.4.3 (이전 3s) |
| `GAME_DEBOUNCE_TICKS` | 2 | 연속 miss 후 idle |
| 프로필 `poll_sec` | 60 | hangame_games.json |

스모크에서는 `GAME_POLL_SEC=5`, `GAME_DEBOUNCE_TICKS=0` 권장.

## 운영 시퀀스

1. agent 등록 → heartbeat `game_active=false`, LED gray  
2. 사용자가 바둑이/포커 실행  
3. ≤ poll 주기 내 detect → `game_active=true`, LED green, `stream_enable=true`  
4. 대시보드/운영자가 screenshot·stream 요청 → 통과  
5. 게임 종료 → debounce 후 gate 닫힘

## 검증

- **현재(v1.6.0+)**: screenshot은 항상 허용. `scripts/host_smoke.sh` step [4]가 enqueue 200 확인.
- (폐기 전) Win VM: `.\scripts\smoke_windows.ps1` (Hangame presence + stream gate 섹션)
- 프로세스 실명: `.\scripts\hangame_proc_probe.ps1`

## 후속

- 실제 GDI/DXGI 캡처 + chunk upload
- stream 세션(start/frame/stop) 상태머신
- 3-2/3-3 게임 프로필 확장 시 allowlist만 추가
