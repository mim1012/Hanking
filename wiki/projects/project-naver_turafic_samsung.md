# project-naver_turafic_samsung

> naver-traffic-android-samsung의 자매 클론 — 봇 서버를 adpang에서 분리(Phase 1)하는 파생 작업 레포. | 도메인: 네이버 트래픽(서버 분리) | 스택: Kotlin(앱) + Node 표준 라이브러리(서버, npm 의존 0), Supabase | 상태: 활성 (스캐폴드/서버 분리 + 전략 실험 모니터링)

## 핵심 자산 (다른 곳에서 주워올 것)
- `server/lib/crypto.js:15` `encryptSecret()` — 네이버 계정 자격증명 AES-256-GCM 암호화(`v1:iv:tag:cipher` base64url) + `safeEqual`(상수시간).
- `server/lib/tasks.js:109` `resolveGroupStrategyOverride()` — 디바이스 그룹 단위 전략 오버라이드(lease 시 그룹값이 product/slot 값보다 우선).
- `server/lib/landing.js:45,66` `buildNaverSuggestUrl()` / `landingRedirect()` — Toss 리워드 실측 트래픽 시그니처(mtp_sug, base62 ackey, 무관 시드 acq, acr 6-8) 서버측 위조 + `Referrer-Policy: no-referrer` 307 랜딩. Kotlin `SamsungBrowserStrategyB.buildSuggestUrl`과 파라미터 시그니처 동일 유지.
- `server/lib/admin.js:510` `getExperimentHistory()`(+`buildStints:463`, `rankStatsInWindow:494`) — 전략 실행 로그와 외부 순위 피드를 조인해 전략(A/B/G/H)별 순위개선 스코어카드 산출. `server/web/admin/history.html` 모니터링 UI가 소비.
- `server/dev-server.js` — npm 의존성 0, Node 표준 라이브러리만으로 계정 lease/heartbeat/rotation API. 무의존 계약 테스트 서버 패턴.
- `docs/server-schema.sql`, `docs/vercel-supabase-api.md` — 프로덕션 API 계약 문서.
- `android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/SamsungWebViewManager.kt:494` `clickMidLink()` — 캐러셀 상품 진입을 **실제 탭 전용**으로(`loadUrl` 폴백 삭제). 프로그램 이동은 유입수 집계에서 누락된다(실측). → [[유입수-집계되는-진입]]

## 메모 — MID 탭 정확도·진입 시퀀스 (2026-08-01, 실기기 실측 검증됨)
프로덕션 A슬롯 10건 × 5회 스윕으로 검증. 결과: 진입 10/10, 스크롤로 캐러셀 도달 10/10,
로드 오탐 재시도 0, 슬롯 중앙값 37초→33초. (커밋 `38d4fb0`, versionCode 미변경)
- **탭 좌표는 스캔 시점이 아니라 탭 시점 기준이어야 한다** — `SamsungWebViewManager.kt:520` 은 좌표를
  얻은 직후 스캔을 한 번 더 돌려 신선한 좌표로 탭한다. JS→네이티브 왕복과 지연 로딩 이미지 부착
  사이에 레이아웃이 밀리면 첫 좌표가 낡는다. 재스캔 실패 시 낡은 좌표로 누르지 않고 재시도로 돌린다.
- **좌표 위 요소를 `elementFromPoint` 로 재확인** — `NaverPageScripts.kt:405`. rect 가 멀쩡해도
  스티키 헤더·오버레이가 덮으면 탭은 엉뚱한 요소로 간다. 가려지면 rect 안 다른 지점을 프로브.
- **착지 판정은 2값이 아니라 3값이어야 한다** — `NaverPageScripts.kt:441` `midLandingMatchJs`. 카탈로그
  MID(`nv_mid`)와 스마트스토어 `productNo` 는 **다른 번호**라 착지 URL 에 MID 가 없는 것이 정상이다.
  2값(일치/불일치)으로 짜면 거짓 실패 기계가 된다. MISMATCH 는 양쪽 모두 확정 상품번호를 노출하고
  값이 다를 때만.
- **동작 변경과 계측을 같은 배포에 섞지 않는다** — MISMATCH 여도 `success=true` 를 유지하고
  `detailStatus` 문자열만 바꿨다. 오탑률 미측정 상태에서 실패로 돌리면 이후 성공률 변동이
  "오탑 발견"인지 "새 회귀"인지 귀속 불가. → [[검증리그-데이터형태-재현]]
  **이 판단이 실측으로 정당화됐다**: MISMATCH 로 잡힌 케이스가 오탑이 아니라 **슬롯 `linkUrl` 이 낡은 것**
  이었다(판매자 재등록으로 productNo 변경, 같은 스토어·같은 상품). 실패로 처리했다면 정상 동작하는
  슬롯이 100% 실패로 집계됐다.
- **판정 기준선은 추적 URL 로는 부족하다** — 캐러셀 앵커는 `cr3.shopping.naver.com/.../searchGate?nv_mid=`
  형태라 `/products/<번호>` 가 없다. 앵커 href 만 쓰면 10건 중 9건이 "판정 불가"였다. task 의 `linkUrl`
  (정본 상품 URL)을 기준선에 **추가**해야 작동한다
  (`naver_turafic_samsung/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/data/TaskLease.kt:55`).
- **캐러셀을 지나쳤으면 아래로 더 내려도 못 만난다** — 사람 흉내 SERP 훑기가 캐러셀을 지나치는 일이 잦다
  (실측 첫 상태 8회 중 7회 `ABOVE`). 아래 방향만 있는 스크롤 루프는 **10슬롯 0건 성공**이었고,
  위로 되돌리는 경로를 넣자 **10/10** 이 됐다(게다가 17초 빨라짐 — 헛스와이프 소진이 사라져서).
  → [[사람행동-시뮬레이션]]
- **서브리소스 오류를 페이지 실패로 취급하지 마라** — `onReceivedError` 에서 `isForMainFrame` 을 안 보면
  광고 이미지·CDN 실패가 정상 페이지를 "실패"로 만들어 **같은 URL 을 재요청**한다. 속도 손실이자
  트래픽 시그니처 오염이다(랜딩을 2초 간격으로 두 번 두드림). 직전 페이지의 늦은 `onPageFinished` 가
  새 대기를 완료시키는 것도 함께 막아야 한다(내비게이션 세대 카운터).
- `android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/data/TaskLease.kt:43` `firstKeyword = source.keyword` — 서버의 `keyword`(짧은 순위 타깃)와 `keyword_name`(긴 상품명)은 **다른 값**이다(활성 슬롯 568건 중 562건 불일치). 로컬 검증 스텁이 이 둘을 같게 시드하면 "쉬운 리그"가 되어 회귀를 못 잡는다. → [[검증리그-데이터형태-재현]]
- `tools/packet-a/packet_a.py` `--mode landing` — 랜딩 307 → 단일 SERP → 브릿지 링크로 앱과 같은 홉 구조를 브라우저 없이 재현(한 사이클 0.35~0.58초). 랜딩이 발급한 ackey/acq/acr/query 를 JSON 으로 돌려줘 앱 로그와 시그니처 대조가 된다.
- `tools/touch-record/parse-getevent.mjs` — `getevent -lt` 원시 터치 로그를 사람 리듬 통계(스와이프 거리·지속·속도, 탭 홀드, 동작 간격)로 환산. 실기기 녹화를 봇 상수로 옮기는 입구. → [[사람행동-시뮬레이션]]
- `tools/adb-fleet/fleet.mjs` — 로컬 ADB 플릿 감시/자동복구(핫스팟 확인 대화상자 탭, 프록시 잔재 제거, adb 데몬 자가복구, 이름 중복 경고).

## 메모 — 브라우저 정체성 헤더 와이어 실측 (2026-08-03)
mitmproxy 전수 캡처(네이버/pstatic 114건) + 헤더 에코 페이지 두 경로로 교차 확인.

- **UA ↔ sec-ch-ua 불일치는 해결돼 있다(114/114 일치).** 2026-06-01 캡처에서는 UA 가 `Chrome/137` 인데
  sec-ch-ua 가 `"Android WebView";v="138"` 로 **브라우저 종류도 버전도** 어긋났다. `applyPersona()` 단일 경로 +
  `BrowserIdentity` 로 어긋난 조합을 타입상 표현 불가능하게 만든 것이 와이어에서 검증됐다.
  → 프로덕션 라이브 `vc161 / 0.1.83` 에 이미 반영된 상태.
- **고엔트로피 클라이언트 힌트가 빈 값으로 새던 문제도 해소됐다.** 6월엔 `sec-ch-ua-model: ""`,
  `sec-ch-ua-arch: ""` 등이 붙어 나갔으나 지금은 아예 안 나간다(Accept-CH 미요청 시 정상 동작).
- **`X-Requested-With: com.navertraffic.samsung` 은 100% 노출된다(113/113).** 앱 레벨 해결책이 없다 —
  상세는 [[봇탐지-우회-스택]] 의 dead-end 항목. 과거 커밋 `90c27e8` "X-Requested-With 로 앱 패키지명 노출 차단"은
  **실제로는 차단된 적이 없다.**
- **`sec-fetch-site: none`** — `loadUrl` 진입이라 "주소창에 직접 입력"으로 보인다. 반면 상품 진입은 실제 탭이라
  `same-site` + `referer` 가 정상으로 붙는다. → [[유입수-집계되는-진입]] 의 구분선과 같은 자리.
  `zero-android-optionc/app/src/main/java/com/zero/traffic/engine/ActionExecutor.java:1288` `computeSecFetchSite()`
  는 이전 URL 로 이 값을 계산해 덮어쓴다(우리는 미적용).

## 메모 — S7 커스텀 ROM 준비 (2026-08-03, 미실행)
`SM-G930K`(heroltektt, Android 8.0/SDK26)에서 **크롬 143 은 커스텀 ROM 으로만 가능**하다
(Chrome 139+ 가 Android 10/API 29 를 요구). herolte 용 LineageOS 20(Android 13) zip 의
`META-INF/com/android/metadata` `pre-device` 에 **`heroltektt` 가 명시**돼 있어 한국 통신사 변종 호환이 선언돼 있다.
하드 브릭은 **다른 모델 스톡 펌웨어(BL/CP 포함)** 를 올릴 때만 나고, 커스텀 ROM/TWRP 는 최악이 부트루프다.
**"크롬 143 설치" != "봇 엔진 143"** — 봇은 WebView 로 도므로 판정은 `dumpsys webviewupdate` 의 제공자 줄.
펌웨어는 `samloader-rs`(topjohnwu) 로 삼성 공식 서버에서 받는다 — 파이썬 samloader 계열은 FUS 인증 키가
바뀌어 nonce 복호화에서 전부 실패하고, 서드파티 사이트는 403/reCAPTCHA 로 막힌다.

## 관계
- **모체** = [[project-naver-traffic-android-samsung]] (0.1.x 릴리스 계보, 전략 A/B/C/G/H 전체). 이 레포는 그 봇 서버 분리 파생본. 버리는 커밋도 자매 클론 + reflog에 보존.

## 의존 개념: [[IP-로테이션]] · [[캡차-차단-대응]] · [[네이버-순위-계산]] · [[봇탐지-우회-스택]]
