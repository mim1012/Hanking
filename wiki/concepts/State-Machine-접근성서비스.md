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
- **수락 완료 오판과 정지 누락 (Vortex, 2026-09-14)** — 탭 문구는 비선택 상태에서도 존재한다. 실제 수락/확인 입력 이후 선택된 목적 화면을 판정하고 실패 다이얼로그를 우선해야 한다. `Vortex/app/src/main/java/com/example/twinme/domain/state/handlers/WaitingForConfirmHandler.kt:153`. 성공 후 다음 root 조회를 기다리지 않고 즉시 정지해야 화면 전환으로 root가 사라져도 멈춘다: `Vortex/app/src/main/java/com/example/twinme/engine/CallAcceptEngineImpl.kt:357`. Android 기기 테스트에서 지연 완료 방식의 실패와 즉시 완료 방식의 통과를 확인했다.
- **"무시(ignored)"와 "거절(rejected)"이 같은 accepted=false로 보여도 다운스트림 액션이 다르다 (Vortex, 2026-09-15)** — 증상: 출발임박 배지 맞춤콜이 거절 클릭 없이 맞춤 목록에 남아 매 사이클 재감지. 원인: 적격성 판정기 `Vortex/app/src/main/java/com/example/twinme/domain/model/CallEligibilityDecision.kt:41-52`의 임박 분기가 맨 앞에서 `ignored`로 조기 리턴해, 뒤의 맞춤콜 경로(능동 거절)를 굶겼다. `Vortex/app/src/main/java/com/example/twinme/domain/state/MatchCallRejectionPolicy.kt:9-15`는 `!accepted && !ignored`일 때만 거절을 지시한다. 기법: 3상태(수락/거절/무시) 판정기에서 "무시"는 능동 액션이 필요한 항목(맞춤콜처럼 큐에 남는 것)에 절대 쓰지 말고, 분기 추가 시 정책 객체의 기대 액션까지 테스트에 박는다(`CallEligibilityDecisionTest.imminentMatchCall*`). 수락은 비가역 커밋이라 애매하면 거절이 fail-safe. 경쟁앱 오버런도 이 결합케이스는 거절 없이 새로고침만 한다(`Vortex/apk_extractor/overun-tuto/jadx-single/sources/com/overun/tuto/AdvancedMacroService.java:488-534`).
- **OTA 교체 후 미바인딩** — `pm install -r` 뒤 `enabled_accessibility_services`가 비워질 수 있다. 바인딩 확인 없이 태스크를 시작하면 입력 경로가 보장되지 않으므로, 복구·확인·작업 시작을 하나의 게이트로 묶어야 한다.
- **무루트 기기** — `su`가 실패하면 자동복구할 수 없다. 이 경우 성공률을 추정해 계속 돌리지 말고 `rootAvailable`/`bindingVerified`를 별도 상태로 보고해 운영자가 제외·조치할 수 있게 한다.

## 배제된 접근 (dead-end)
- **접근성 미바인딩 시 `View.dispatchTouchEvent` 합성 폴백** — 서비스가 죽은 상태에서도 작업을 계속해 바인딩 장애를 숨긴다. vc252에서 이 경로를 제거하고 `accessibility_required`로 명시 실패시켜, 잘못된 성공률과 입력 경로 혼합을 막았다.

## 관련: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[앱-자동업데이트-채널]] · [[project-Vortex]] · [[project-UberAutoAccept]] · [[project-naver_turafic_samsung]]

## 입력 성공과 화면 반영 분리 (Vortex, 2026-09-14)
- **실패 패턴: 감지bounds 무시 + pending 입력 중복 전송** — Android 기준선 재현에서 버튼660,2035 대신540,2080 탭, pending 프로세스에3회 전송을 확인했다. `Vortex/app/src/main/java/com/example/twinme/domain/state/handlers/DetectedCallHandler.kt:702`은 한 관찰 사이클에 transport를 한 번만 호출한다.
- **재사용 정책** — `Vortex/app/src/main/java/com/example/twinme/domain/state/AcceptButtonTiming.kt:3`: 동일버튼250ms 안정화, 이전단계300ms, 같은단계1200ms 간격 최대3회. 화면전환 검증과 최초입력 기준 deadline을 분리한다. `WaitingForConfirmHandler.kt`의 성공판정은 재시도 중에도 계속 우선한다.
- **배제한 접근** — 노드를 얻은 뒤 고정sleep하고 같은좌표 연타: 화면갱신을 관찰하지 못하고 아직 실행 중인 입력을 중복 보낸다. 빈callKey만 리셋키로 쓰면 다음 시도에 상태가 남으므로 기존step1ClickedAt도 비교한다: `Vortex/app/src/main/java/com/example/twinme/domain/state/handlers/WaitingForConfirmHandler.kt:86`.
- 검증은 로컬 Android fixture까지이며 실제 서비스 운영 성공은 별도 확인한다. 참고앱 분석: `Vortex/docs/ACCEPT_TIMING_STABILITY.md`.
- **실패 입력도 재시도 예산에 포함** — 맞춤콜 거절의 성공일 때만 증가하던 카운터 때문에 입력 실패시3.5초간18/47회 시도됐다. `Vortex/app/src/main/java/com/example/twinme/domain/state/handlers/DetectedCallHandler.kt:491`, `Vortex/app/src/main/java/com/example/twinme/domain/state/handlers/WaitingForConfirmHandler.kt:593`은 실패도 카운트하고 별도 단조시계로500ms 간격/최대4회를 제한한다. 전송성공 시각과 시도시각을 합치면 실패 후 버튼 소실을 성공으로 오인하므로 분리한다. Android fixture 재현은4/4회로 통과.
