# project-Vortex

> KakaoT Driver 콜을 접근성 API로 자동 수락하는 Android 서비스(상태머신 + Hilt). | 도메인: 앱 자동화/리버싱 | 스택: Kotlin, Hilt, Shizuku, Coroutines/StateFlow, Supabase | 상태: 활성 (자산 밀도 최고)

## 핵심 자산 (다른 곳에서 주워올 것)
- `docs/INPUT_INJECTION_DETECTION.md` — 입력주입 vs 봇탐지 분석 (canonical). → [[Shizuku-입력주입]]
- `ShizukuConnectionManager.kt:246` — Shizuku 셸 탭. 3단계 클릭 폴백.
- `docs/VIEW_ID_REFERENCE.md`, `docs/ViewIdlist.md` — 디컴파일 view ID 레퍼런스. → [[디컴파일-ViewId-자동화]]
- `fasttree/` — 별도 프로세스 UI 트리 조회(UIAutomation + AIDL).
- `monitoring/AccessibilityDeathTracker.kt` — 접근성 서비스 death 추적.
- `.github/workflows/release.yml` — APK → Supabase Storage → GitHub Release 자동화.

## 의존 개념: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[State-Machine-접근성서비스]]
