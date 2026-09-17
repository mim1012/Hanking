# 카카오T 예약콜 실시간 감지의 한계

> 카카오T 기사앱의 예약콜(전체 리스트)은 서버가 ~3초 캐시 + push 없음 — <1초 fresh 감지는 물리적으로 불가능하며, "요청 0"으로 fresh하게 읽는 유일한 검증된 방법(e1 리더)은 LSPosed/root의 프로세스 코드주입을 전제한다. Shizuku(ADB shell 권한)로는 비루팅에서 재현 불가.

## 무엇 / 언제 쓰나
카카오T 기사앱의 예약콜(맞춤콜 포함) 신규 발생을 빠르게/저비용으로 감지하는 방법을 설계할 때. "웹소켓 후킹" "직접 API 폴링" 같은 지름길을 시도하기 전에 먼저 확인할 사전 조사.

## 정본 구현 (canonical) — 실측 검증됨, root/LSPosed 전제
- **최종 결론 + 실측표** — `turbo/docs/260622_realtime_reservation_detection_research.md:20-58` — 10가지 방식 실측 비교. cold restart(~3.2s, fresh) vs 워밍 재생성(~0.5s, but stale) vs count 폴링(무의미) 등.
- **e1 리더(유일하게 "우리 요청 0"으로 fresh 확보)** — `H7.a.e1(ReservationScheduleResponse) → ReservationSchedule` 매퍼를 LSPosed로 후킹해 앱이 이미 파싱한 리스트를 그대로 읽음(`:60-63`). fresh 진입(cold/navigate) 시에만 발화, 워밍 single-top 진입에선 캐시 재사용이라 미발화.
- **서버측 ~3초 캐시 확인** — tcpdump 실측(`:67-72`), 앱 코드 `c9.C3304G.Q()`가 `(현재-마지막폴링) >= reservationListRefreshTime`일 때만 새 GET, 아니면 캐시 `o1()` 사용. `reservationListRefreshTime` 기본 0 → 화면 열어둬도 자동폴링 안 함.
- **push/WebSocket 조사** — `:76-82` — PushType 중 `DISPATCH_POOL_CALL_REQUESTED(24)`는 **특정 콜이 나에게 배차될 때의 개별 알림**이지 예약콜 풀 전체 변경 신호가 아님. 전체 리스트 변경을 실시간 push하는 WebSocket 채널은 존재하지 않음(즉시콜만 push 실시간, 예약콜은 폴링/refresh만).

## 다른 구현 / 변형
- **UI 팝업 기반 준-실시간 감지 (비루팅 대안, 이벤트 구동)** — 카카오T가 새 맞춤콜 발생 시 띄우는 "맞춤콜"/"맞춤콜 보기" 시스템 다이얼로그를 **접근성 이벤트(`onAccessibilityEvent`, `TYPE_WINDOW_STATE_CHANGED`)에서 즉시** 감지·자동 탭한다(주기 폴링 아님, 지연 ~0ms). 신호=팝업 뷰ID+텍스트(`tv_title="맞춤콜"` + `btn_positive="맞춤콜 보기"` + `v_button_wrapper`); `tv_total_count`(맞춤 N건)나 알림리스너가 아님(`KakaoNotifProbeService`는 진단전용, 트리거 안 함). 견고성 장치: type32 이벤트에 "맞춤콜" 텍스트만 보이면 **1초 힌트 프리암**(`rememberCustomCallPopupHintIfPresent`) 후 접근성 트리를 못 읽으면(FLAG_SECURE/스테일) **UiAutomation 덤프 폴백**으로 bounds 재확보; 탭 중복억제 300ms·실패 700ms×5회 재시도·초과 시 3000ms giveup·성공 후 "리스트 진입 대기(5s)→`vg_item` 확인" 핸드셰이크; 리스트 진입 직후 25ms fast-poll. 다른 콜 처리 중 새 맞춤콜은 **거절확정 단계(CLICK_REJECT_CONFIRM)만 보류, 그 외엔 선점(automationState 리셋 후 재파싱)**. `touchassist/jadx/sources/com/kakaotaxi/reserve/auto/accessibility/ReservationWatchService.java:27483,27548,28463-28749,31754`, `dump/KakaoUiDumpParser.java:1686-1710`. root/LSPosed 불필요(AccessibilityService + Shizuku 탭). → [[project-touchassist]]
- **Vortex — 폴링(~6초 새로고침) + 이벤트 구동 즉시 재분석 병행 (2026-09-17 추가)** — 원래 리스트 콜 평가가 `LIST_DETECTED→REFRESHING(~5.8~6초 게이트)→ANALYZING` **폴링에만 묶여**, 카카오T가 push로 리스트에 바로 띄운 맞춤콜을 **다음 새로고침까지(~6초) 방치**(사용자 리포트 "리스트에서 멍때림"). 접근성 이벤트는 관찰만 하고 상태머신은 매크로 루프가 돌리던 구조. 수정(touchassist식 이식): `CallAcceptAccessibilityService.tryImmediateListAnalyze()`가 `WINDOW_CONTENT_CHANGED`/`STATE_CHANGED`에서 (state∈{WAITING_FOR_CALL,LIST_DETECTED} + 실제 콜카드 `tv_reserved_at` 존재 + 250ms 디바운스 + 최상단 카드 signature 변경)이면 `engine.onListContentChanged()`(기존 미사용 `executeImmediate` 재활용)로 상태머신을 즉시 1회 kick → `ListDetectedHandler`가 `analyzeNowRequested` 플래그로 새로고침 없이 ANALYZING 전환. 커밋 `971dbab3`+`c721bed9`(never-reset 카운터 공유 버그 수정)+`de3d8670`(signature 가드). 구현·독립 Kotlin 리뷰 완료(차단 0), **실기기 검증 대기**. 서버 ~3초 캐시라 여전히 폴링도 병행(이벤트는 "이미 뜬 콜 즉시 반응"용). → [[project-Vortex]]

## 주의점 / 함정
- **"count API = 전체 개수"라는 착각** — `/v1/reservations/count`는 **"맞춤"(필터 매칭) 개수만** 반환하고 전체 예약콜 개수가 아님(`:33-41`, 실측: 화면 "맞춤 0건/전체 237건"일 때 API가 0 반환). count 폴링으로 전체 리스트 변화를 추적하려 하면 처음부터 잘못된 신호를 쫓게 된다.
- **모듈 직접 HTTP 폴링/재현은 root 여부와 무관하게 밴 위험** — `:49-50` 실측: <3초 간격 직접 fetch는 HTTP 400(rate limit) 유발. root/LSPosed로 인증 토큰을 확보할 수 있다 해도, 이 경로 자체가 서버측 리스크라 회피되지 않음. turbo 프로젝트도 이 방식 대신 "이미 뜬 화면/이미 파싱된 데이터"만 읽는 e1 리더를 채택.

## 배제된 접근 (dead-end)
- **비루팅(Shizuku/ADB shell 권한)에서 e1 리더 재현** — 불가능. e1 리더는 카카오T 앱 프로세스 내부에 Xposed/LSPosed로 코드를 주입해 그 프로세스의 메모리 내 파싱 결과를 가로채는 방식이며, 이는 Zygote 레벨 후킹이라 구조적으로 root(또는 Magisk+Zygisk)를 전제한다. Shizuku는 ADB shell(uid 2000) 권한만 주는 별도 격리 프로세스일 뿐 — 다른 앱의 프로세스 메모리에 개입하거나 그 안의 메서드를 후킹할 수 없다. 비루팅에서 같은 값을 얻으려면 카카오T 인증 토큰을 별도로 확보해야 하는데, 샌드박싱 때문에 root 없이는 다른 앱의 private storage/메모리에서 토큰을 꺼낼 수 없고, MITM(로컬 VPN 프록시)도 인증서 피닝에 막힐 가능성이 높다.
- **모듈이 카카오T 인증 토큰으로 직접 API 재현(HTTP fetch)** — 토큰 확보 자체가 비루팅에서 막히는 데다(위 항목), 확보하더라도 <3초 폴링은 rate-limit/밴 위험이라 turbo 프로젝트에서조차 최종 채택하지 않음.
- **이벤트마다 무조건 상태머신 kick (toolbar-only root 오판)** — Vortex가 과거 `onAccessibilityEvent`에서 상태머신을 이벤트마다 돌렸는데, 툴바만 있는 root가 이벤트 경로로 주입되면 LIST_DETECTED/ANALYZING을 오판해 misfire가 반복돼 연결을 끊고 매크로 루프만 남겼다(`CallAcceptAccessibilityService.kt` 이벤트 핸들러 주석에 박제). 이벤트 구동을 재도입할 땐 **실제 콜카드(`tv_reserved_at`) 존재 + state 아이들-온-리스트({WAITING_FOR_CALL,LIST_DETECTED}) + 디바운스 + 최상단 signature 변경** 가드로 막아야 오판이 안 난다(2026-09-17 이 가드들로 재도입). 또 이벤트 트리거를 **런타임 리셋 없는 누적 카운터**(`StateContext.matchCallFastCycleCount`)에 묶지 말 것 — 잦은 `WINDOW_CONTENT_CHANGED`가 몇 초 만에 캡을 소진해 기능이 죽는다(첫 구현 버그, `c721bed9`로 디바운스+signature 스로틀로 교체). 스로틀은 시간 기반(디바운스)+실제변경 감지(signature)로.

## 관련: [[Shizuku-입력주입]] · [[보안화면-검정-scrcpy-FLAG_SECURE우회]] · [[project-touchassist]] · [[project-turbo]] · [[project-Vortex]]
