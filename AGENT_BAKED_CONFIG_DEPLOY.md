# C2 Agent — 환경설정 Bake & 더블클릭 배포 (v1.4.4 / server 1.5.0)

> **작성일**: 2026-08-01
> **요구사항**: 환경설정은 미리 셋팅해서 빌드에 포함. 호스트에서 `agent.exe` 더블클릭만으로 에이전트가 실행되어 서버에 연결.

## 결론 (요약)

**달성됨.** `agent.exe` 는 환경변수 없이도 동작하도록 설정값(서버 URL, 토큰)이 **빌드 시 바이너리에 내장(baked-in)** 되어 있다. 더블클릭만으로 실행되며, 같은 호스트에 `c2-server.exe` 가 이미 떠 있으면 즉시 등록된다.

내장 기본값이 **서버의 기본값과 정확히 일치**하므로(아래 대조표) 별도 환경변수 없이도 통신이 성립한다.

## 1. 내장(baked) 설정 대조표

`agent/main.go` 의 `loadConfig()` 는 환경변수가 없으면 내장 기본값(`getEnv(key, default)`)을 사용한다.

| 설정 | 에이전트 내장 기본값 | 서버 기본값 | 일치 |
|------|---------------------|-------------|:----:|
| 전송 방식 | `wss` | wss/http 둘 다 | ✅ |
| 서버 URL | `wss://127.0.0.1:8443/agent` | `0.0.0.0:8443` listen | ✅ |
| 에이전트 토큰 | `lab-only-change-me` | `lab-only-change-me` | ✅ |
| 관리자 토큰 | (해당없음) | `lab-only-change-me` | ✅ |
| 암호화 PSK | `""` (비활성) | `""` (비활성, crypto=false) | ✅ |
| 버전 | `1.4.4-lab-always-cmd` | `1.5.0-lab-standalone` | — |

> **중요**: 단일 호스트(물리 Windows)에 서버+에이전트를 함께 돌리는 구성이므로 `127.0.0.1` 내장값으로 충분하다.
> VM 등 **서버가 다른 머신**인 경우에만 `C2_SERVER` / `AGENT_TOKEN` / `C2_PSK` 환경변수로 덮어쓰면 된다.

## 2. 동작 원리

- `agent/main.go` `loadConfig()`: `getEnv(key, default)` 로 먼저 환경변수를 읽고, 없으면 **두 번째 인자(내장 기본값)** 를 사용.
- 빌드 스크립트 `build/build.sh` 는 `{{...}}` 플레이스홀더 치환을 수행하지만, 이 프로젝트는 플레이스홀더 없이 **Go 소스의 기본값 인자에 직접 설정값을 넣는 방식**(bake)을 채택.
- `-H=windowsgui` 플래그 → 콘솔 창 없이 백그라운드로 실행.

## 3. 빌드 방법

```bash
export PATH=/home/kali/warm-agent/data/workspace/.toolchain/go/bin:$PATH
export GOROOT=/home/kali/warm-agent/data/workspace/.toolchain/go

# Windows 에이전트 (더블클릭용)
cd agent
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o ../bin/agent.exe .

# Linux 클라이언트 (프로토콜 테스트용)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../bin/agent_linux .
```

> 참고: 이 Kali VM에 시스템 `go` 가 없어 `/home/kali/warm-agent/data/workspace/.toolchain/go` (go1.22.5) 를 내려받아 사용함.

## 4. 호스트 배포 (더블클릭 절차)

```
c2-server.exe 더블클릭  ← 서버 상시 기동 (1.5.0-lab-standalone, 자동 인증서/디렉토리 생성)
agent.exe 더블클릭      ← 에이전트 즉시 등록
```

배포 폴더: `deploy_winvm/bin/agent.exe` (재빌드본 반영 완료)

| 항목 | 필요 |
|------|------|
| 환경변수 | ❌ 없음 (전부 내장) |
| 추가 파일 | `certs/` 는 서버가 자동생성, `data/` 자동생성 |
| 서버-에이전트 동일 호스트 | ✅ 127.0.0.1 기본값 |

## 5. 테스트 결과 (2026-08-01, Kali에서 검증)

환경변수 **전혀 없이** (c2server도 env 없이 기동된 상태) 검증:

```
[agent 실행] → registered as agent-58e61c1cbc1bd4ef (wss)
[server /health] → "server_ver":"1.5.0-lab-standalone", "agents":1, "online":1, "crypto":false
[/admin/agents] → agent-58e61c1cbc1bd4ef, version 1.4.4-lab-always-cmd, online:true
[/admin/cmd shell] → cmd-d655d82616e66cf9 → done, output 'BAKED_OK\nkali\n'
```

- ✅ 환경변수 없이 에이전트가 서버에 등록됨
- ✅ 등록 후 관리자 API(내장 토큰 `lab-only-change-me`)로 shell 명령 `echo BAKED_OK && hostname` 실행 → `BAKED_OK\nkali\n` 정상
- ✅ 관리자 인증도 내장 기본값으로 동작

## 6. 산출물

| 파일 | 위치 | 비고 |
|------|------|------|
| Windows 에이전트 (더블클릭) | `bin/agent.exe` | 5.6MB, GUI, v1.4.4 baked |
| 호스트 배포 에이전트 | `deploy_winvm/bin/agent.exe` | 동일 재빌드본 |
| Linux 테스트 클라이언트 | `bin/agent_linux` | 프로토콜 테스트용 |
| 서버 (더블클릭) | `bin/c2-server.exe` | 1.5.0-lab-standalone |

## 7. 보안 주의

- 기본 토큰(`lab-only-change-me`)은 **lab 전용**. 실제 배포 시 `AGENT_TOKEN`/`ADMIN_TOKEN`/`C2_PSK` 를 교체해야 하며, 그 경우 서버 실행 시 환경변수로 설정하고 **에이전트는 동일 값을 내장값으로 재빌드**하거나 환경변수로 주입해야 한다.
