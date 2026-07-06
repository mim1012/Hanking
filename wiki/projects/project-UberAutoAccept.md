# project-UberAutoAccept

> Uber Driver 라이드 오퍼를 접근성으로 자동 수락하는 Android 앱(Vortex 동일 계열). | 도메인: 앱 자동화 | 스택: Kotlin, Shizuku, Coroutines/StateFlow, Supabase | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `engine/StateMachine.kt` (`AGENTS.md:56`) — Vortex 상태머신 이식. → [[State-Machine-접근성서비스]]
- `AGENTS.md:96` — Uber APK view ID 매핑 테이블. → [[디컴파일-ViewId-자동화]]
- `utils/ShizukuHelper.kt` (`AGENTS.md:82`) — Shizuku `input tap` 우회. → [[Shizuku-입력주입]]
- `AGENTS.md:78` — 크로스헤어 좌표 클릭(오버레이). `AGENTS.md:32` — 프로젝트 로컬 빌드 캐시 규약.
- `logging/RemoteLogger.kt` — 배치 원격 로깅(200 버퍼, 10s flush).

## 의존 개념: [[State-Machine-접근성서비스]] · [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]]
