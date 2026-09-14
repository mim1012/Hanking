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

## 변형: Android 증분빌드가 manifest는 신버전·dex는 구버전으로 갈라진다 (2026-09-08)

같은 원리의 안드로이드 판. Gradle android 증분빌드가 오염되면 **manifest/output-metadata/dumpsys는 신 versionCode(vc76/0.4.59)인데 dex에 컴파일된 `BuildConfig`는 구 versionCode(vc75/0.4.58)**로 남을 수 있다. `generateReleaseBuildConfig`가 새 파일을 만들어도 javac/dex가 캐시된 구 클래스를 dex에 넣어, "설치 vN" 표시가 도처에 뜨는데 **런타임은 구 코드를 실행**한다.

- **이름 붙인 실패 패턴**: *manifest-런타임 버전 스큐*. `adb install -r`, force-stop 재기동, 완전 uninstall+재설치를 각각 시도해도 안 고쳐졌다(3회 실측, 런타임 로그가 계속 "0.4.58/현재 75"). 포그라운드 서비스 앱은 `install -r` 시 구 프로세스가 살아 구 dex를 유지할 수 있다.
- **해결 = `gradlew clean assembleRelease`(clean 필수)**. `adb install -r`류 재설치가 아니라 빌드 캐시 자체를 비워야 dex가 새 BuildConfig로 다시 컴파일된다.
- **실물 검증법**: 설치 후 반드시 logcat의 **런타임 버전 로그**로 확정한다 — `sellermate_naver_place_all/rank-check/android-app/app/src/main/java/com/navertraffic/rankchecker/MainActivity.kt:66`가 `네이버 순위체크 <VERSION_NAME> 무인 서비스 준비`를, `Release.kt:105`가 `업데이트 없음: 현재 <VERSION_CODE>`를 찍는다. **dumpsys/output-metadata(=manifest)만 믿으면 안 된다.**
- 부수 함정: `gradlew clean` 중 Gradle 데몬 JVM 크래시(`hs_err_pid`)가 1회 발생 → 재시도로 통과.
- dead-end: 재설치 방식(`adb install -r` / force-stop / uninstall+reinstall)으로 구 dex를 밀어내려는 시도 — 빌드 산출물 자체가 구 클래스를 담고 있으므로 몇 번을 재설치해도 런타임 버전이 안 바뀐다(3회 실측이 이 dead-end의 증거).

## 관련: [[project-turafic_update]] · [[project-sellermate_naver_place_all]] · [[앱-자동업데이트-채널]]
