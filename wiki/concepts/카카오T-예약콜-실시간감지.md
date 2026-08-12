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
- **UI 팝업 기반 준-실시간 감지 (비루팅 대안)** — `touchassist/ReservationWatchService.java:28644-28749` — 카카오T가 새 맞춤콜 발생시 띄우는 "맞춤콜"/"맞춤콜 보기" 시스템 다이얼로그를 접근성 트리로 매 사이클 감지해 자동 탭. 코드주입 없이(AccessibilityService만으로) 카카오T 앱 자체의 폴링 결과가 화면에 반영되는 순간을 캐치하는 방식 — root/LSPosed 불필요. → [[project-touchassist]]
- Vortex의 현재 방식 — 화면(접근성 트리) 기반 주기적 새로고침, ~2-5초 간격(`refresh_delay` 설정). e1 리더와 비교해 "우리 요청 0"은 아니지만(새로고침 제스처 자체 발생), 서버가 어차피 ~3초 캐시라 실질 fresh 주기는 큰 차이 없음. → [[project-Vortex]]

## 주의점 / 함정
- **"count API = 전체 개수"라는 착각** — `/v1/reservations/count`는 **"맞춤"(필터 매칭) 개수만** 반환하고 전체 예약콜 개수가 아님(`:33-41`, 실측: 화면 "맞춤 0건/전체 237건"일 때 API가 0 반환). count 폴링으로 전체 리스트 변화를 추적하려 하면 처음부터 잘못된 신호를 쫓게 된다.
- **모듈 직접 HTTP 폴링/재현은 root 여부와 무관하게 밴 위험** — `:49-50` 실측: <3초 간격 직접 fetch는 HTTP 400(rate limit) 유발. root/LSPosed로 인증 토큰을 확보할 수 있다 해도, 이 경로 자체가 서버측 리스크라 회피되지 않음. turbo 프로젝트도 이 방식 대신 "이미 뜬 화면/이미 파싱된 데이터"만 읽는 e1 리더를 채택.

## 배제된 접근 (dead-end)
- **비루팅(Shizuku/ADB shell 권한)에서 e1 리더 재현** — 불가능. e1 리더는 카카오T 앱 프로세스 내부에 Xposed/LSPosed로 코드를 주입해 그 프로세스의 메모리 내 파싱 결과를 가로채는 방식이며, 이는 Zygote 레벨 후킹이라 구조적으로 root(또는 Magisk+Zygisk)를 전제한다. Shizuku는 ADB shell(uid 2000) 권한만 주는 별도 격리 프로세스일 뿐 — 다른 앱의 프로세스 메모리에 개입하거나 그 안의 메서드를 후킹할 수 없다. 비루팅에서 같은 값을 얻으려면 카카오T 인증 토큰을 별도로 확보해야 하는데, 샌드박싱 때문에 root 없이는 다른 앱의 private storage/메모리에서 토큰을 꺼낼 수 없고, MITM(로컬 VPN 프록시)도 인증서 피닝에 막힐 가능성이 높다.
- **모듈이 카카오T 인증 토큰으로 직접 API 재현(HTTP fetch)** — 토큰 확보 자체가 비루팅에서 막히는 데다(위 항목), 확보하더라도 <3초 폴링은 rate-limit/밴 위험이라 turbo 프로젝트에서조차 최종 채택하지 않음.

## 관련: [[Shizuku-입력주입]] · [[project-touchassist]] · [[project-turbo]] · [[project-Vortex]]
