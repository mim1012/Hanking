# project-Vortex

> KakaoT Driver 콜을 접근성 API로 자동 수락하는 Android 서비스(상태머신 + Hilt). | 도메인: 앱 자동화/리버싱 | 스택: Kotlin, Hilt, Shizuku, Coroutines/StateFlow, Supabase | 상태: 활성 (자산 밀도 최고)

## 핵심 자산 (다른 곳에서 주워올 것)
- `docs/INPUT_INJECTION_DETECTION.md` — 입력주입 vs 봇탐지 분석 (canonical). → [[Shizuku-입력주입]]
- `ShizukuConnectionManager.kt:246` — Shizuku 셸 탭. 3단계 클릭 폴백.
- `docs/VIEW_ID_REFERENCE.md`, `docs/ViewIdlist.md` — 디컴파일 view ID 레퍼런스. → [[디컴파일-ViewId-자동화]]
- `fasttree/` — 별도 프로세스 UI 트리 조회(UIAutomation + AIDL).
- `monitoring/AccessibilityDeathTracker.kt` — 접근성 서비스 death 추적.
- `.github/workflows/release.yml` — APK → Supabase Storage → GitHub Release 자동화.

## 루팅 브랜치 — LSPosed/Frida 우회 서브시스템 (폴더 `D:\Project\vortex_rooting`, 브랜치 `feature/rooting-implementation`)
같은 Vortex 레포의 별도 브랜치 체크아웃. `app/`(접근성 서비스)는 Vortex main과 동일(위 자산이 커버)하나, **`lsposed/` Xposed 후킹 + `frida/` 디컴파일-매핑 워크플로우는 main에 없는 별도 서브시스템**이라 여기 노트로 기록(별도 페이지 아님). [[project-turbo]](카카오T LSPosed)와 같은 계열.
- **case-flip 난독화 클래스 탐색** — `vortex_rooting/lsposed/src/main/java/com/vortex/kakaobypass/MainHook.kt:75` `findClassFlex()` — jadx/ProGuard 대소문자 불일치(`A7.d→a7.d`) 대응해 첫 글자 case flip 재시도. [[project-turbo]]의 `findClassFlex`와 동일 패턴(형제 계보).
- **WebSocket 생명주기 후킹** — `vortex_rooting/lsposed/src/main/java/com/vortex/kakaobypass/MainHook.kt:426` `hookAllMethods(wsClass, "disconnect", …)` — 2045줄 단일 후킹 레지스트리(토큰 캡처, throttleFirst-5s 제거, Play Integrity 캡처, SSL 언핀, 예약 자동 리프레시 13개 훅).
- **LSPosed 모듈 스켈레톤** — `vortex_rooting/ROOTING_IMPLEMENTATION.md:52` + `lsposed/src/main/assets/xposed_init` — 엔트리 `com.vortex.kakaobypass.MainHook`, `handleLoadPackage` 패키지필터 + `param.result=true`로 KakaoT 5초 리프레시 리미터 우회. 재사용 Xposed 모듈 템플릿.
- **함정: OkHttp `compileOnly` → 런타임 `NoClassDefFoundError`** — `vortex_rooting/lsposed/src/main/java/com/vortex/kakaobypass/MainHook.kt:1859` — `java.net.HttpURLConnection` 폴백 필요(타입은 컴파일되나 번들 안 됨). [[project-turbo]]와 동일 함정.

## 의존 개념: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[State-Machine-접근성서비스]]
