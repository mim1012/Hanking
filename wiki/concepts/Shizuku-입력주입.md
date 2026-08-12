# Shizuku 입력 주입 (접근성 봇탐지 우회)

> Android에서 접근성 gesture가 탐지될 때 Shizuku 셸 권한으로 `input tap`을 실행해 진짜 입력처럼 위장.

## 무엇 / 언제 쓰나
KakaoT/Uber 등 앱이 접근성 자동클릭을 차단할 때. 자동 수락 봇의 핵심 우회.

## 정본 구현 (canonical)
- **입력주입 vs 봇탐지 분석** — `Vortex/docs/INPUT_INJECTION_DETECTION.md` — dispatchGesture의 `FLAG_IS_ACCESSIBILITY_EVENT 0x800` 탐지(`:44`), `MotionEvent.getDeviceId() == -1` 약점(`:63`), escalation: `sendevent`로 `/dev/input/eventX` 직접 주입 → 진짜 deviceId(`:77`).
- **Shizuku 셸 탭** — `Vortex/app/src/main/java/com/example/twinme/utils/ShizukuConnectionManager.kt:246` (`tryInputTapDetailed`, `input tap $x $y`, uid 2000).
- **3단계 클릭 폴백** — Shizuku `input tap` → `performAction` → `dispatchGesture`.

## 다른 구현 / 변형
- `UberAutoAccept/utils/ShizukuHelper.kt` — `input tap x y` 5탭×30ms, `FLAG_IS_GENERATED_BY_ACCESSIBILITY` 우회 (`AGENTS.md:82`).
- `touchassist/service/ShellService.java` + `util/ShizukuHelper.java` — 시즈쿠 한 바인딩(`:shell_service` 별도 프로세스)으로 **탭 주입 + UIAutomation 직접 트리조회 + uiautomator dump 셸 실행**까지 3용도 통합. 버전 관리(`USER_SERVICE_VERSION`)로 앱 업데이트마다 UserService 갱신, 재바인딩 스토밍 방지(`duplicateConnectCount`). Vortex의 탭 전용 사용보다 심화. → [[project-touchassist]]

## 관련: [[디컴파일-ViewId-자동화]] · [[State-Machine-접근성서비스]] · [[카카오T-예약콜-실시간감지]] · [[project-Vortex]] · [[project-UberAutoAccept]] · [[project-touchassist]]
