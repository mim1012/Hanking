# EDR 탐지 실측 계획 — 서명/무서명 A/B (2026-08-04)

> 목적: EDR 보안팀이 랩 호스트에서 **무서명 vs lab-CA 서명 vs 신뢰서명** 변종을 대상으로
> SSmartScreen/Defender/MS 커넥트 탐지 여부를 실측한다.
> 이 문서는 승인된 랩(자체 Windows VM) 한정 작업 절차이다.

## 변종 준비 (Kali)

| 변종 | 산출물 | 상태 |
|------|--------|------|
| A. 무서명 | `bin/agent.exe`(스테이지 페이로드), `bin/loader.exe`(서비스), `bin/loader.dll` | 기본 빌드 |
| B. lab-CA 서명 | `bin/agent_signed.exe`, `bin/loader_signed.exe`, `bin/loader_signed.dll` | `./build/sign.sh` |
| C. 신뢰 서명 (선택) | `SIGN_P12=<pfx> SIGN_P12_PASS=... ./build/sign.sh artifacts` | 있으면만 |

> v1.7.1: MSI 서비스 바이너리는 **loader.exe** (C2 `/stage/agent`에서 agent PE를 메모리 스톰핑). agent.exe는 디스크에 설치되지 않고 스테이지 서버만 제공. MSI 재빌드는 호스트 WiX 단계.

> 코드사인으로 SmartScreen 파란 화면 억제를 실측하는 절차는 `docs/msi-codesign-smartscreen.md` 참조.

재현:
```bash
./build/build.sh        # 무서명 A
./build/build.sh sign   # lab-CA 서명 B
osslsigncode verify -in bin/agent_signed.exe   # 서명 검증
```

## A/B 실측 (Windows 랩 호스트, Administrator)

스테이징:
```powershell
Copy-Item \\KALI\share\DocViewer_Setup.msi C:\lt\a_unsigned.msi
Copy-Item \\KALI\share\DocViewer_Setup_signed.msi C:\lt\b_signed.msi
```

절차 — 각 변종에 대해 동일:

1. (첫 변종만) `Set-MpPreference -DisableRealtimeMonitoring $false`
   → Defender 실시간 보호 **활성화**
2. `msiexec /i C:\lt\<variant>.msi /qn /l*v C:\lt\msi_<variant>.log`
3. 설치 후 즉시:
   ```powershell
   Get-MpThreat | fl ThreatName,Path,Signature
   Start-MpScan -ScanType QuickScan
   Start-Sleep -Seconds 90
   Get-MpThreatDetection | fl Resources,FileName
   ```
4. 서비스/파일 존재 확인:
   ```powershell
   Get-Service DocViewerHelper
   Get-ChildItem 'C:\Program Files\DocViewer'
   Get-AuthentiCodeSignature 'C:\Program Files\DocViewer\DocViewerHelper.exe' | fl
   ```
5. **B 변종 확인용**: `Get-AuthentiCodeSignature`가 `Status=Valid`(서명은 유효)로 표시되지만
   `IsOSBinary=False`/트러스트체인 `Unknown` — 신뢰루트가 아님을 실측에서 기록한다.

### 기재 항목 (테이블)

| 변종 | Defender 설치차단 | QuickScan 탐지 | 서명상태(AuthentiCode) | 서비스 Running | WMI 구독 알림 |
|------|------------------|----------------|--------------------------|----------------|----------------|
| A 무서명 | | | | | |
| B lab 서명 | | | | | |
| C 신뢰(있으면) | | | | | |

## 파생 데이터 / 후속

- lab-CA 서명이 Defender를 우회하지 **못하는**지 기록 → 신뢰 인증서 여부가 유일 변수인지 확인.
- 무서명 PE `Program Files` 신설 vs 서명 PE 간 탐지 차이를 정량화 (행위 기반 탐지 여부).

> 경고: 이 절차는 승인된 랩 자산에 한한다. 실운영 타깃에는 금지.