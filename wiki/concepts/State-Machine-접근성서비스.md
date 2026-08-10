# State Machine + 접근성 서비스 아키텍처

> sealed class 상태 + 이벤트 구동 전이 + StateFlow로 Android 접근성 자동화 봇을 구성하는 패턴. Vortex 계보 공유.

## 무엇 / 언제 쓰나
자동 수락/자동 입력 봇의 라이프사이클(대기→온라인→감지→수락)을 견고하게 관리할 때.

## 정본 구현 (canonical)
- **Vortex State Machine** — `Vortex/` — Idle→Online→OfferDetected→…→Accepting 상태 전이, StateFlow 기반. Hilt DI.
- **접근성 서비스 death 추적** — `Vortex/monitoring/AccessibilityDeathTracker.kt` — Settings 변경 ContentObserver, 메모리압박 ComponentCallbacks2, Doze/Shizuku death 원인 분석.

## 다른 구현 / 변형
- `UberAutoAccept/engine/StateMachine.kt` (`AGENTS.md:56`) — 동일 패턴 이식, `com.example.twinme` 패키지 계보 공유.
- **Android 접근성 바인딩 자가복구** — `naver_turafic_samsung/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/AccessibilitySelfHeal.kt:60-78` — root로 secure settings를 복구한 뒤 최대 3회 재시도하고, `pollUntilBound()`로 실제 바인딩을 확인한 경우에만 성공을 반환한다.
- **작업 시작 전 바인딩 게이트** — `naver_turafic_samsung/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/ui/MainActivity.kt:921-929` — OTA 교체 후 서비스가 풀려 있으면 30초 간격으로 재확인하며, 확인 전에는 작업 시작을 허용하지 않는다.
- **제스처 입력 하드 게이트** — `naver_turafic_samsung/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/WebViewGestureEngine.kt:245-273,299-341` — 탭·스크롤을 `dispatchGesture` 경로로만 보내고 서비스가 없거나 dispatch가 실패하면 `accessibility_required:*`로 중단한다.

## 주의점 / 함정
- **OTA 교체 후 미바인딩** — `pm install -r` 뒤 `enabled_accessibility_services`가 비워질 수 있다. 바인딩 확인 없이 태스크를 시작하면 입력 경로가 보장되지 않으므로, 복구·확인·작업 시작을 하나의 게이트로 묶어야 한다.
- **무루트 기기** — `su`가 실패하면 자동복구할 수 없다. 이 경우 성공률을 추정해 계속 돌리지 말고 `rootAvailable`/`bindingVerified`를 별도 상태로 보고해 운영자가 제외·조치할 수 있게 한다.

## 배제된 접근 (dead-end)
- **접근성 미바인딩 시 `View.dispatchTouchEvent` 합성 폴백** — 서비스가 죽은 상태에서도 작업을 계속해 바인딩 장애를 숨긴다. vc252에서 이 경로를 제거하고 `accessibility_required`로 명시 실패시켜, 잘못된 성공률과 입력 경로 혼합을 막았다.

## 관련: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[앱-자동업데이트-채널]] · [[project-Vortex]] · [[project-UberAutoAccept]] · [[project-naver_turafic_samsung]]
