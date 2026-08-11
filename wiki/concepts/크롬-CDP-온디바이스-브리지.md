# 크롬-CDP-온디바이스-브리지

> 안드로이드 앱이 같은 기기의 외부 크롬을 Chrome DevTools Protocol(CDP)로 제어하기 위해, shell/root 권한 브리지 프로세스를 경유하는 패턴.

## 무엇 / 언제 쓰나

앱이 외부 크롬을 인텐트로 띄우면 JS를 주입하거나 URL을 읽을 방법이 없다. WebView는 `evaluateJavascript`로 이걸 할 수 있지만 외부 크롬은 별도 프로세스라 접근이 안 된다. CDP 소켓(`@chrome_devtools_remote`)은 열려 있으나 크롬이 접속자의 UID를 검사해 일반 앱을 거절한다(shell/root만 허용).

브리지는 shell 권한으로 도는 작은 dex 프로세스(app_process)로, `127.0.0.1:9223 ⇄ @chrome_devtools_remote`를 TCP↔abstract 소켓으로 중계한다. 앱은 평범한 loopback TCP로 브리지에 붙으므로 SELinux 벽도 없다.

## 정본 구현 (canonical)

- `naver_turafic_samsung/tools/cdp-bridge/src/com/navertraffic/cdpbridge/Bridge.java:1` — Java 브리지 (ServerSocket → LocalSocket 릴레이)
- `naver_turafic_samsung/.release-0.3.3-worktree/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/ChromeDevToolsBridge.kt:1` — 앱이 dex를 꺼내 su로 기동 + 60초 워치독
- `naver_turafic_samsung/.release-0.3.3-worktree/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/ChromeDevToolsClient.kt:1` — HTTP(탭 목록/URL) + WebSocket(Runtime.evaluate, 앵커 클릭) 클라이언트
- `naver_turafic_samsung/.release-0.3.3-worktree/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/ChromeDevToolsSocket.kt:1` — 런타임 의존성 없는 RFC6455 구현

## 다른 구현 / 변형

- PC에서 `adb forward tcp:9222 localabstract:chrome_devtools_remote`로 붙는 것은 테스트 하네스 전용. 프로덕션 경로는 기기 내부에서 브리지가 돌아야 한다.

## 주의점 / 함정

- **"접근성을 안 쓰면 ANR이 없어진다"는 참이지만, 접근성을 끄면 봇이 멎는 함정이 있었다.** 앱의 자가복구(AccessibilitySelfHeal)가 su로 접근성을 되켰고, CDP가 멀쩡한데도 clickMidLink가 접근성 바인딩만 보고 즉시 EXHAUSTED를 반환했다. CDP||접근성 게이트 + disableSelf()로 해결.
- **탭 누적이 정확도를 망친다.** 크롬은 인텐트마다 새 탭을 열고 닫지 않아 652개까지 쌓였다. firstPageWebSocketPath()가 옛 탭을 골라 현재 화면과 다른 DOM을 보게 된다. loadAndWait 앞에서 closeStaleTabs()를 불러 1탭을 유지해야 한다.
- **awaitUrlSettled가 옛 URL을 "안정"으로 오판한다.** CLEAR_TOP|SINGLE_TOP 인텐트는 기존 탭의 URL을 교체하는데, CDP가 볼 때 아직 교체 전이면 옛 URL이 2회 연속 같아 즉시 "안정"으로 통과한다. 인텐트 직전 URL의 호스트와 달라질 때까지 기다려야 한다.
- **SERP 쇼핑 블록은 지연 로딩이다.** URL이 정착해도 상품 앵커(`nv_mid=`)가 DOM에 아직 없을 수 있다. 스크롤+대기를 반복하는 탐색 루프가 필요하다.

## 배제된 접근 (dead-end)

- **앱이 직접 abstract 소켓에 붙기** — SELinux `avc: denied { connectto }` (permissive에선 통과하지만 enforcing에선 차단) + 크롬 `devtools_auth.cc`가 UID로 거절(`Connection reset by peer`). 우회 대상이 아니라 설계된 보안 경계다.
- **Shizuku 권한 브로커** — shell 권한을 얻을 수 있지만 기기마다 수동 활성화가 필요해 24대 플릿 자동화와 안 맞는다.
- **접근성으로 크롬 DOM을 읽기** — 시스템 서버를 경유하는 Binder 순회가 ANR을 일으킨다. 실기기 3대에서 크롬이 MotionEvent 하나를 3,086ms 처리. 순회 시간 상한을 어떤 값으로 잡아도 ANR과 기능 사이를 만족할 수 없었다(250ms→1500ms→400ms 시행착오).

## 관련: [[봇탐지-우회-스택]] · [[State-Machine-접근성서비스]] · [[앱-자동업데이트-채널]]
