# Hangame §3-1 프로세스 감지 — 구현·검증 메모

> 작성: 2026-07-31  
> 범위: **3-1 only** (바둑이 + 포커 패밀리)  
> 버전: agent/server `1.4.2-lab-hangame31`

## 목표

에이전트가 **§3-1 allowlist 클라이언트 프로세스**가 떠 있을 때만:

- heartbeat에 `game_active=true` + `game_id` / `game_title` / `game_pid`
- `stream_enable=true` (향후 화면 송출 게이트)
- 대시보드 바인딩용 `dashboard_led=green`

프로세스 없으면 `game_active=false`, `dashboard_led=gray`.

## Allowlist (§3-1)

| id | title | images |
|----|-------|--------|
| baduki | 한게임 바둑이 | `baduki.exe` |
| poker7 | 한게임 7포커 | `poker7.exe` |
| laspoker | 한게임 라스베가스 포커 | `laspoker.exe` |
| duelpoker | 한게임 듀얼포커 | `duelpoker.exe`, `dualpocker.exe` |
| highlow2 | 한게임 하이로우 | `highlow2.exe` |

런처(`hanstarter*`, `hangamestartersetup.exe`)는 **무시**.  
프로파일: `profiles/hangame_games.json` (에이전트에 동일 allowlist 임베드 폴백).

## 구현 위치

| 구성 | 변경 |
|------|------|
| `agent/main.go` | `initGameProfile` / `listProcessImages` / `scanGames` / `buildHeartbeat` |
| `server/main.go` | `Agent` 필드 + `applyGamePresence` (WS·HTTP heartbeat) + `/admin/agents` LED 기본값 |
| `profiles/hangame_games.json` | §3-1 프로파일 |
| `protocol.md` | v1.4.2 heartbeat extras |
| `config.lab.env` | `HANGAME_PROFILE`, `GAME_POLL_SEC`, `GAME_DEBOUNCE_TICKS` |

### Agent 동작 요약

- Windows: `tasklist /FO CSV /NH` → 이미지명 CI basename 매칭
- Linux(lab): `/proc/*/exe|comm` + stem↔`.exe` 상호 인덱싱 (스모크용)
- poll 기본 3s (`GAME_POLL_SEC`), off-debounce 기본 2틱 (`GAME_DEBOUNCE_TICKS`)
- games[] **순서 우선** — 동시 실행 시 첫 매치

### Server 동작 요약

- heartbeat/register에서 `game_*` / `dashboard_led` / `stream_enable` 반영
- `game_active=false`면 id/title/pid/since 클리어, LED=`gray`
- `/admin/agents` JSON에 동일 필드 노출 (대시보드 초록불 바인딩)

## Lab 스모크 결과 (2026-07-31, Kali HTTP beacon)

환경: `bin/c2server` + `bin/agent_linux`, `AGENT_STABLE_ID=lab-hangame31-smoke`,  
`HANGAME_PROFILE=./profiles/hangame_games.json`, `GAME_POLL_SEC=1`, `GAME_DEBOUNCE_TICKS=0`.

| 단계 | 조작 | 관측 | 결과 |
|------|------|------|------|
| baseline | 게임 없음 | `game_active=false`, `dashboard_led=gray`, profile 로드됨 | PASS |
| active | `/tmp/baduki` (sleep 복사) 실행 | `game_active=true`, `game_id=baduki`, `game_title=한게임 바둑이`, `dashboard_led=green`, `stream_enable=true` | **PASS_ACTIVE** |
| inactive | fake kill | `game_active=false`, LED `gray` | **PASS_INACTIVE** |
| poker7 | `/tmp/poker7` 실행 | `game_id=poker7`, LED `green` | **PASS_POKER7** |

빌드 산출물:

- `bin/c2server` (linux) — `1.4.2-lab-hangame31`
- `bin/agent_linux` — lab 프로토콜 테스트
- `bin/agent.exe` / `bin/agent_win_amd64` — Windows 타깃

## 환경 변수

| Env | Default | 설명 |
|-----|---------|------|
| `HANGAME_PROFILE` / `GAME_PROFILE` | `profiles/hangame_games.json` | allowlist JSON 경로 |
| `GAME_POLL_SEC` | `3` | 프로세스 스캔 주기 |
| `GAME_DEBOUNCE_TICKS` | `2` | 종료 후 gray 전환까지 연속 미스 횟수 |

## 의도적 비범위 (후속)

- §3-2/3-3 (고스톱·보드 등) 이미지 확장
- 실제 화면 송출 파이프 (stream_enable 게이트만 준비)
- 대시보드 UI 컴포넌트 (API 필드만 제공)
- Win VM `tasklist` 실측 교차검증 (리서치 문서의 미검증 exe 이름 확정)

## 리스크·주의

- allowlist 이미지명은 리서치 단계 값 — Win 실측 전 **오탐/미탐 가능**
- Linux stem 매칭은 lab 전용 편의; 운영 타깃은 Windows `tasklist`
- 프로세스 스캔은 heartbeat 경로에서 rate-limit됨 (기본 3s)


---

## v1.4.3 변경 (2026-07-31)

| 항목 | 이전 | 이후 |
|------|------|------|
| `GAME_POLL_SEC` 기본 | 3 | **60** |
| 프로필 `poll_sec` | 3 | **60** |
| stream gate | heartbeat 필드만 | **server 409 + agent execute 거부** |
| 스모크 | 기본 transport | + fake `baduki` image + gate 검증 |
| probe | (없음) | `scripts/hangame_proc_probe.ps1` |

상세: [stream-enable-gate.md](./stream-enable-gate.md) · [wmi-persistence.md](./wmi-persistence.md)
