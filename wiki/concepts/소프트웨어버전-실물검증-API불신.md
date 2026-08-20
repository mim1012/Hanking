# 소프트웨어버전-실물검증-API불신
> "최신 버전"을 버전 추적 API/블로그 공지만으로 판단하지 말고, 실제로 배포되는 설치파일 자체의 내장 메타데이터로 확정한다 — 단계적 롤아웃 중인 API 응답이 실제 배포 채널보다 앞설 수 있다.

## 무엇 / 언제 쓰나
어떤 소프트웨어가 "지금 최신 버전인지" 확인해야 할 때(플릿 전체를 하나의 베이스라인으로
통일하기 전 등). Google Chrome처럼 버전 추적 API(`versionhistory.googleapis.com`)와 실제
엔터프라이즈 배포 파일이 분리된 채널로 운영되는 소프트웨어에서, API 목록의 최상단 값을
그대로 "최신"이라 믿으면 틀릴 수 있다.

## 정본 구현 (canonical)
- `turafic_update`(round-penguin worktree) `scripts/verify-installer-msi-version.ps1` —
  MSI를 실제로 내려받아 `WindowsInstaller.Installer` COM으로 `Property` 테이블의
  `ProductVersion`을 직접 조회. 2026-08-20 이 세션에서 두 차례 실측 성공(신선한 다운로드
  직후 `151.0.7922.170` 확인, 이후 재실행에서도 동일 값 재현) — Google `versionhistory`
  API의 `channels/stable` 최상단 값(`152.0.7977.42`)과 달랐음을 원본 JSON 응답
  (`chrome/platforms/win64/channels/stable/versions/152.0.7977.42`, 경로 자체에 `stable`
  명시돼 beta 오분류 아님을 확인)과 대조해 확정.
- 판정 기준: API가 "stable"로 등록한 버전이라도, 실제 배포 URL
  (`dl.google.com/edgedl/chrome/install/GoogleChromeStandaloneEnterprise64.msi`)이 내려주는
  파일의 `ProductVersion`이 그보다 낮으면, **후자가 "지금 실제로 깔리는 버전"**이다 — 전자는
  단계적 롤아웃 중인 최신 등록치일 뿐.

## 다른 구현 / 변형
- 원본 페이지를 프롬프트 기반 웹 요약 도구(WebFetch류)로 읽으면 구조화된 버전 숫자가
  누락/왜곡될 수 있다 — 이번 세션에서 Chrome Releases 블로그를 WebFetch로 읽었을 때 버전
  번호가 계속 빠졌다. 정확한 숫자가 필요하면 API를 직접 파싱(`Invoke-RestMethod` 등)하거나
  파일 메타데이터를 읽어야 한다.

## 주의점 / 함정
- **"API가 최신이라고 등록했다" ≠ "지금 다운로드하면 그 버전이 온다"** — 이름 붙인 실패
  패턴: *등록-배포 채널 분리 오판*. 단계적 롤아웃을 쓰는 배포 시스템은 이 둘이 항상 일치하지
  않는다. 사용자가 두 번 "이거 베타 아니야?"라고 되물어서 재검증했고, 그 결과 API 기반
  판단이 실제와 달랐음이 드러남 — 사용자 반문이 검증 재개의 실제 계기였다.
- 이 방식으로 확인한 실물 버전(`151.0.7922.170`)이 이미 대상 PC(VM1)의 설치 버전과
  정확히 일치해, "업그레이드 필요"라는 오판을 막을 수 있었다.

## 배제된 접근 (dead-end)
- **WebFetch로 Chrome Releases 공식 블로그를 읽어 버전 번호를 얻으려는 시도** — 요약 모델이
  버전 숫자를 반복적으로 누락시켜(제목/날짜만 남고 정작 필요한 버전 문자열이 빠짐) 신뢰할 수
  있는 근거가 안 됨. 구조화된 숫자 데이터가 필요하면 요약 기반 fetch보다 원본 API 직접
  파싱이나 파일 메타데이터 직접 조회를 우선해야 한다는 근거.
- **`msiexec /a`(administrative install)로 MSI를 통째로 풀어 그 안의 실행파일
  `.VersionInfo`를 읽으려는 시도** — 이 Chrome 엔터프라이즈 MSI는 `/a`를 걸어도 일반적인
  다중 파일 트리를 풀지 않고 msi 자체를 대상 경로에 그대로 복사할 뿐이라(2026-08-20 실측,
  추출 폴더에 `installer.msi` 단일 파일만 생성됨), 이 경로로는 실행파일을 찾을 수 없었다.
  COM `Property` 테이블 직접 조회 쪽이 이 설치파일 형태에서는 더 안정적인 경로였다(단,
  COM 호출 자체가 막 받은 파일에서 간헐적으로 실패할 수 있다는 별도 함정은 남아있음 —
  `scripts/verify-installer-msi-version.ps1` 상단 주석 참고).

## 관련: [[project-turafic_update]]
