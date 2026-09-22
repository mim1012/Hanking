# MSI 코드서명 — SmartScreen 파란 화면 억제 실측 (2026-08-04)

> 목적: 신뢰/랩 코드사인 서명으로 인터넷 다운로드 MSI 설치 시 나타나는
> SmartScreen 파란 화면("Windows에서 PC를 보호했습니다")을 억제하는지를 실측한다.
> 승인된 랩(자체 Windows VM/호스트) 한정 작업. 실운영 타깃 대상 금지.

## 결론

- **유효한 코드사인 서명이 있으면 인터넷 다운로드 MSI도 파란 화면이 안 뜬다.**
- 현재 lab-CA 서명(변종 B)은 신뢰루트가 아니라 기본으론 파란 화면이 뜬다.
  신뢰 억제 확인은 경로 1(실인증서) 또는 경로 2(lab CA를 호스트 트러스트에 설치)로만 가능.
- **전제: 파란 화면은 MotW(Mark of the Web, 인터넷 다운로드 표시)가 있는 파일만 대상.**
  SMB 공유로 옮긴 MSI는 MotW가 없어 애초에 경고가 안 뜰 수 있음 → 실측은 반드시 브라우저 다운로드 경유.

---

## 경로 1 — 실인증서 (정석)

공인 CA(OV/EV)에서 코드사인 인증서를 발급받아 `pfx` + 비번으로 서명:

```bash
SIGN_P12=<pfx> SIGN_P12_PASS=<pass> ./build/sign.sh artifacts
# → bin/agent_signed.exe · bin/loader_signed.exe · bin/loader_signed.dll
#   deploy/DocViewer_Setup_signed.msi  자동 생성
```

- OV도 보통 통과. EV는 신뢰 누적이 빨라 더 확실.
- 발급 직후 첫 실행엔 가끔 여전히 경고가 뜨는 경우도 있음(리퓨테이션 축적 전).

## 경로 2 — 랩 한정 (lab CA 트러스트 설치)

별도 실인증서 없이 현재 lab-CA 서명이 자 Windows 한정으로 "신뢰" 취급되게 하려면
호스트 관리자 콘솔에서 lab CA를 `Root` + `TrustedPublisher` 스토어에 설치:

```powershell
Import-Certificate -FilePath \\kali\share\certs\signing\lab-test-ca.crt `
  -CertStoreLocation Cert:\LocalMachine\Root
Import-Certificate -FilePath \\kali\share\certs\signing\lab-test-ca.crt `
  -CertStoreLocation Cert:\LocalMachine\TrustedPublisher
```

이후 서명 MSI 설치 → 파란 화면 억제 여부 확인. 무서명(변종 A)과 A/B 비교.

> 이 방법은 **해당 호스트 한정**이다. 실운영 기기엔 lab CA가 없으므로 적용되지 않음.

---

## A/B 실측 절차 (호스트)

1. 변종 준비 (Kali)
   ```bash
   ./build/build.sh        # A 무서명 → DocViewer_Setup.msi
   ./build/sign.sh artifacts  # B lab 서명 → DocViewer_Setup_signed.msi
   ```
2. 브라우저(Chrome/Edge)로 각 MSI를 다운로드 → MotW 부여 확인:
   ```powershell
   Get-Item .\DocViewer_Setup.msi | Select-Object FullName,@{n='Zone';e={$_.Zone}}
   # Zone=3 (인터넷)이어야 실측 의미 있음. 아니면 브라우저 경유.
   ```
3. 각 변종을 `msiexec /i ... /qn /l*v` 설치하며 파란 화면/알 수 없는 게시자 대화상자 발생 여부 기록.
4. 서명 검증:
   ```powershell
   Get-AuthentiCodeSignature .\DocViewer_Setup_signed.msi | fl Status,StatusMessage
   # Status=Valid → 서명 유효
   # 실인증서면 SignerCertificate가 DigiCert/Sectigo 등 공인 CA로 표시
   ```
   Kali에서: `osslsigncode verify -in deploy/DocViewer_Setup_signed.msi`

## 기록 항목

| 변종 | 브라우저 MotW | 파란 화면 | 알 수 없는 게시자 | AuthentiCode Status |
|------|--------------|-----------|------------|----------------------|
| A 무서명 | | | | |
| B lab 서명(+트러스트) | | | | |
| C 실인증서(있으면) | | | | |

## 참고

- 서명 억제 목표는 **"1차 방역"**에만. 행위 기반 탐지는 별도(`docs/edr-test-plan.md`).
- lab-CA 서명 파이프라인: `build/sign.sh`, `docs/edr-detection-surface.md`.