# State Machine + 접근성 서비스 아키텍처

> sealed class 상태 + 이벤트 구동 전이 + StateFlow로 Android 접근성 자동화 봇을 구성하는 패턴. Vortex 계보 공유.

## 무엇 / 언제 쓰나
자동 수락/자동 입력 봇의 라이프사이클(대기→온라인→감지→수락)을 견고하게 관리할 때.

## 정본 구현 (canonical)
- **Vortex State Machine** — `Vortex/` — Idle→Online→OfferDetected→…→Accepting 상태 전이, StateFlow 기반. Hilt DI.
- **접근성 서비스 death 추적** — `Vortex/monitoring/AccessibilityDeathTracker.kt` — Settings 변경 ContentObserver, 메모리압박 ComponentCallbacks2, Doze/Shizuku death 원인 분석.

## 다른 구현 / 변형
- `UberAutoAccept/engine/StateMachine.kt` (`AGENTS.md:56`) — 동일 패턴 이식, `com.example.twinme` 패키지 계보 공유.

## 관련: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[project-Vortex]] · [[project-UberAutoAccept]]
