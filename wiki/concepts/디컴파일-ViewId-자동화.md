# 디컴파일 → ViewId 매핑 → 자동화

> APK를 jadx로 디컴파일해 view ID를 추출하고, 그걸 자동화 봇의 노드 파싱 상수로 전환하는 워크플로우.

## 무엇 / 언제 쓰나
새 대상 앱을 자동화(자동수락/자동입력)하기 전, UI 요소를 안정적으로 식별할 상수를 뽑을 때.

## 정본 구현 (canonical)
- **추출·디컴파일 스크립트** — `android-decompile/extract-and-decompile.ps1:29` — `adb shell pm path` → `adb pull` → `jadx --show-bad-code --deobf`, extraction-info.txt 생성(`:42`).
- **분석 체크리스트 템플릿** — `android-decompile/architecture-analysis-template.md:26`.

## 산출물 활용 (canonical)
- **ViewId 레퍼런스** — `Vortex/docs/VIEW_ID_REFERENCE.md`, `docs/ViewIdlist.md` (디컴파일 APK 전체 view ID → 파싱 상수).
- **Uber view ID 테이블** — `UberAutoAccept/AGENTS.md:96` (`upfront_offer_configurable_details_accept_button` 등).

## 다른 구현 / 변형
- **Flutter 앱 리버싱** — `guardian-decompile/ANALYSIS_REPORT.md:12` — 로직은 `libapp.so` AOT 스냅샷, base URL은 XOR 난독화(`:133` 안티탬퍼 so 해시 검증).
- **Frida 런타임 후킹** — `turafic/src/frida/hook_signature_functions.js:24` (서명함수 후킹 + 스택트레이스).
- **smali 패칭 트리** — `TwinMe_Original_Smali_Source/smali/org/twinlife/device/`.

## 관련: [[Shizuku-입력주입]] · [[project-Vortex]] · [[project-guardian-decompile]]
