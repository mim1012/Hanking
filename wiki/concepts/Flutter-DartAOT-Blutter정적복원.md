# Flutter-DartAOT-Blutter정적복원

> Flutter 앱의 `libapp.so`(Dart AOT 스냅샷)를 Blutter로 파일별 의사-Dart 코드로 복원해서, jadx로는 절대 안 보이는 Dart 로직(네트워크 클라이언트, 인증 흐름, 화면 구조)을 정적으로 읽는 절차.

## 무엇 / 언제 쓰나
Flutter 앱은 실제 앱 로직(Dart 코드)이 `classes.dex`가 아니라 `libapp.so`(AOT 컴파일된 네이티브 코드)에 들어있어서, jadx/apktool로는 껍데기(플러그인 브릿지, 네이티브 연동부)만 보이고 진짜 비즈니스 로직(API 클라이언트, 인증 요구사항, 화면 라우팅)은 전혀 안 보인다. Blutter는 APK가 쓰는 것과 동일한 Dart 버전의 VM을 소스에서 빌드해서 스냅샷을 파싱, 클래스/함수 단위로 파일을 쪼개 의사-Dart(주석 달린 ARM64 디스어셈블) 형태로 복원한다 — 완전한 원본 Dart는 아니지만 문자열 리터럴·호출 관계·기본 인자값(예: 함수 시그니처의 `{requiresAuth = true}`)까지 그대로 드러나 실질적으로 코드 리뷰가 가능하다.

## 정본 구현 (canonical)
- 도구 위치: `/home/kali/Projects/blutter`(Kali VM, `D:\Project\_wiki`의 [[kali-migration]] 참고) — `blutter.py`, `.venv`, `scripts/` 이미 세팅됨.
- 입력 준비: APK의 `lib/arm64-v8a/libapp.so` + 같은 폴더의 `libflutter.so`(엔진 버전 식별용, jadx 리소스 추출본에서 구할 수 있음: `jadx-out/resources/lib/arm64-v8a/libflutter.so`)를 한 폴더에 모아 scp로 Kali에 전송.
- 실행: `python3 blutter.py <lib_dir> <out_dir>` — Dart 버전을 자동 감지해서 맞는 blutter 실행파일이 없으면 Dart SDK 소스를 체크아웃 후 cmake+ninja로 자동 빌드(최초 1회, 의존성만 맞으면 수 분).
- 산출물: `<out_dir>/asm/<패키지명>/**/*.dart` — 앱의 실제 패키지 구조(clean architecture라면 core/features, data/domain/presentation 계층까지) 그대로 파일이 쪼개져 나옴. 각 함수는 ARM64 어셈블리에 Dart 심볼명 주석이 달린 형태(`// 0x43d888: r2 = "/api/v1/auth/login"` 식으로 문자열 리터럴이 바로 보임).
- 실사례: `guardian-decompile/ANALYSIS_REPORT.md:§10` — `core/network/api_client.dart`에서 `ApiClient.post(.... {requiresAuth = true})` 기본값과, 로그인 호출 한 곳만 `requiresAuth: false`로 오버라이드하는 걸 코드로 확정(어떤 엔드포인트가 토큰 필요한지 100% 정적으로 답함).

## 다른 구현 / 변형
- IDA/Ghidra용 스크립트(`ida_script/addNames.py`)와 Frida 자동생성 스크립트(`blutter_frida.js`)도 같이 산출됨 — 동적분석([[ARM64네이티브-동적후킹-Frida에뮬레이터]])과 조합 가능(Blutter가 함수 위치를 알려주면 Frida로 그 정확한 함수를 후킹).

## 주의점 / 함정
- **venv 셰뱅 경로 깨짐** — Kali 프로젝트 폴더 재구성(2026-08-02, [[kali-migration]]) 이후 `.venv/bin/pip`의 셰뱅이 옛 경로(`/home/kali/blutter`)를 가리켜 `bad interpreter`로 실패. `.venv/bin/python3`가 시스템 python3의 심볼릭 링크뿐이라 사실상 격리 안 됨 — 이런 경우 venv를 고치려 하지 말고 그냥 시스템 `python3 -m pip install --break-system-packages pyelftools requests`로 우회하는 게 빠름.
- **ICU/capstone 미설치로 cmake 실패** — README의 의존성(`libicu-dev`, `libcapstone-dev`, `cmake`, `ninja-build`)을 안 깔면 `Failed to find all ICU components`로 빌드 자체가 안 됨. 에러 메시지만 보고 Blutter 자체가 안 되는 도구라고 오판하지 말 것 — 그냥 apt 의존성 문제.
- **jadx가 `libapp.so`를 리소스로만 취급** — jadx-out에서 `libapp.so`는 `resources/lib/`에 바이너리 그대로만 있고 디컴파일 소스가 안 나온다는 걸 모르고 jadx 결과만 보고 "Dart 로직은 확인 불가"로 결론 내리기 쉬움 — Blutter 같은 전용 도구가 있다는 걸 알아야 함.

## 배제된 접근 (dead-end)
- **capstone으로 직접 손 디스어셈블** — libguardian_secrets.so 같은 작은 네이티브 라이브러리는 가능했지만(→ [[ARM64네이티브-동적후킹-Frida에뮬레이터]]), 6.8MB짜리 Dart AOT 스냅샷 전체를 그 방식으로 읽는 건 현실적으로 불가능(Dart 콜링컨벤션·오브젝트풀·타입정보를 사람이 손으로 복원하는 건 배보다 배꼽이 큼) — Blutter처럼 Dart VM 구조를 아는 전용 파서가 필수.

## 관련: [[ARM64네이티브-동적후킹-Frida에뮬레이터]] · [[디컴파일-ViewId-자동화]]
