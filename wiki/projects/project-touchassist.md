# project-touchassist

> 카카오T 드라이버 예약콜을 자동 수락하는 경쟁 Android 앱(`com.touchassist.service`, 내부 패키지 `com.kakaotaxi.reserve.auto`). 우리 소유 프로젝트 아님 — [[project-Vortex]] 벤치마킹을 위해 디바이스에서 pull 후 jadx 디컴파일한 raw 아카이브. | 도메인: 앱 자동화/리버싱(경쟁분석) | 스택: Kotlin, rikka.shizuku, ML Kit(한국어 OCR), Compose, Supabase(구독) | 상태: 디컴파일 완료(2026-08-12), 소스 미보유(디컴파일 산출물만)

## 파일 위치
- 원본 APK: `D:\Project\android-decompile\touchassist\apk\com.touchassist.service.apk`
- jadx 디컴파일: `D:\Project\android-decompile\touchassist\jadx\sources\com\kakaotaxi\reserve\auto\` (99% 처리, 68클래스 에러 잔존 — 나머지는 정상 읽기 가능)

## 핵심 자산 (다른 곳에서 주워올 것 — [[project-Vortex]] 갭 분석 기준선)
- **시즈쿠 3용도 통합 ShellService** — `service/ShellService.java` + `util/ShizukuHelper.java` — 별도 프로세스(`:shell_service`)에서 탭 주입(`injectTap`/`input tap`) + UIAutomation 직접 트리 조회(`probeScreen`/`probeFirstReservationCard`) + `uiautomator dump` XML 셸 실행까지 시즈쿠 한 바인딩으로 처리. 버전 관리(`USER_SERVICE_VERSION`)·재바인딩 폭풍 방지(`duplicateConnectCount`) 포함. → [[Shizuku-입력주입]](변형)
- **행정동 기반 지역필터("맞춤콜")** — `data/AutoOriginPreferences.java`, `data/AutoDestinationPreferences.java`, `accessibility/ReservationWatchService.java:40361-40576`(`matchesOrigin`/`matchesDestination`) → [[행정동-기반-지역필터]](canonical)
- **맞춤콜 실시간 팝업 감지 → 자동 진입 → 자동거절 상태머신** — `ReservationWatchService.java:28644-28749`(팝업 탭+재시도), `:32352`(매칭실패시 거절 플로우). 카카오T가 띄우는 "맞춤콜"/"맞춤콜 보기" 시스템 다이얼로그를 매 사이클 감지. [[project-Vortex]]엔 이 흐름 자체가 없음(완전한 기능 갭).
- **4중 파싱 소스** — UIAutomation 직접조회 / `dump/KakaoUiDumpParser.java`(uiautomator dump XML, 접근성 트리와 별개 채널) / 접근성 트리 / ML Kit 한국어 OCR(`ReservationWatchService.java:53,56-57` `runOcrBenchmark`, 벤치마킹·디버그 폴백 용도로 추정, 상시 경로 아님).
- **NotificationListenerService로 카카오T 알림 직접 감청** — `notif/KakaoNotifProbeService.java`, `CALL_KEYWORD_REGEX`(원/km/분/예약/맞춤/콜) — 화면 꺼짐/백그라운드에서도 콜 발생 선감지.
- **구독/라이선스 speed tier** — `subscription/DeviceManager.java` — Supabase 기반, 30분 TTL 캐시, heartbeat, `NOT_REGISTERED_CONFIRM_THRESHOLD=3`(연속 3회 확인 후 처리, 오탐 방지), 요금제별 처리속도 차등(`currentSpeedTier`).

## 주의점 / 함정 (touchassist 자체 버그 — 이식시 반복 주의)
- **`selectedProvinces`가 저장만 되고 매칭엔 안 쓰이는 죽은 필드** — `AutoOriginPreferences`에 존재·영속화되지만 `matchesOrigin`/`matchesDestination`은 district/dong만 검사, province는 로그 출력에만 참조(`:39847,39854`). "설정 UI엔 있는데 필터엔 안 먹는" 유형의 실수.
- **`RadiusOption`(반경 필터)도 전체 코드베이스에서 실제 매칭 로직과 무연결** — `MockListSettings`라는 별개 UI 표시용 클래스에서만 소비. 좌표/거리계산(Haversine 등) 코드 자체가 프로젝트에 없음. 반경 옵션 UI가 있다고 해서 반경 기반 매칭이 동작한다고 가정하면 안 됨.

## 의존 개념: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[행정동-기반-지역필터]] · [[카카오T-예약콜-실시간감지]] · [[State-Machine-접근성서비스]]
