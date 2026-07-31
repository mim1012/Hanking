# project-naver_turafic_samsung

> naver-traffic-android-samsung의 자매 클론 — 봇 서버를 adpang에서 분리(Phase 1)하는 파생 작업 레포. | 도메인: 네이버 트래픽(서버 분리) | 스택: Kotlin(앱) + Node 표준 라이브러리(서버, npm 의존 0), Supabase | 상태: 활성 (스캐폴드/서버 분리 + 전략 실험 모니터링)

## 핵심 자산 (다른 곳에서 주워올 것)
- `server/lib/crypto.js:15` `encryptSecret()` — 네이버 계정 자격증명 AES-256-GCM 암호화(`v1:iv:tag:cipher` base64url) + `safeEqual`(상수시간).
- `server/lib/tasks.js:109` `resolveGroupStrategyOverride()` — 디바이스 그룹 단위 전략 오버라이드(lease 시 그룹값이 product/slot 값보다 우선).
- `server/lib/landing.js:45,66` `buildNaverSuggestUrl()` / `landingRedirect()` — Toss 리워드 실측 트래픽 시그니처(mtp_sug, base62 ackey, 무관 시드 acq, acr 6-8) 서버측 위조 + `Referrer-Policy: no-referrer` 307 랜딩. Kotlin `SamsungBrowserStrategyB.buildSuggestUrl`과 파라미터 시그니처 동일 유지.
- `server/lib/admin.js:510` `getExperimentHistory()`(+`buildStints:463`, `rankStatsInWindow:494`) — 전략 실행 로그와 외부 순위 피드를 조인해 전략(A/B/G/H)별 순위개선 스코어카드 산출. `server/web/admin/history.html` 모니터링 UI가 소비.
- `server/dev-server.js` — npm 의존성 0, Node 표준 라이브러리만으로 계정 lease/heartbeat/rotation API. 무의존 계약 테스트 서버 패턴.
- `docs/server-schema.sql`, `docs/vercel-supabase-api.md` — 프로덕션 API 계약 문서.
- `android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/SamsungWebViewManager.kt:466` `clickMidLink()` — 캐러셀 상품 진입을 **실제 탭 우선**으로. 프로그램 이동(`loadUrl`)은 유입수 집계에서 누락된다(실측). → [[유입수-집계되는-진입]]
- `android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/data/TaskLease.kt:43` `firstKeyword = source.keyword` — 서버의 `keyword`(짧은 순위 타깃)와 `keyword_name`(긴 상품명)은 **다른 값**이다(활성 슬롯 568건 중 562건 불일치). 로컬 검증 스텁이 이 둘을 같게 시드하면 "쉬운 리그"가 되어 회귀를 못 잡는다. → [[검증리그-데이터형태-재현]]
- `tools/packet-a/packet_a.py` `--mode landing` — 랜딩 307 → 단일 SERP → 브릿지 링크로 앱과 같은 홉 구조를 브라우저 없이 재현(한 사이클 0.35~0.58초). 랜딩이 발급한 ackey/acq/acr/query 를 JSON 으로 돌려줘 앱 로그와 시그니처 대조가 된다.
- `tools/touch-record/parse-getevent.mjs` — `getevent -lt` 원시 터치 로그를 사람 리듬 통계(스와이프 거리·지속·속도, 탭 홀드, 동작 간격)로 환산. 실기기 녹화를 봇 상수로 옮기는 입구. → [[사람행동-시뮬레이션]]
- `tools/adb-fleet/fleet.mjs` — 로컬 ADB 플릿 감시/자동복구(핫스팟 확인 대화상자 탭, 프록시 잔재 제거, adb 데몬 자가복구, 이름 중복 경고).

## 관계
- **모체** = [[project-naver-traffic-android-samsung]] (0.1.x 릴리스 계보, 전략 A/B/C/G/H 전체). 이 레포는 그 봇 서버 분리 파생본. 버리는 커밋도 자매 클론 + reflog에 보존.

## 의존 개념: [[IP-로테이션]] · [[캡차-차단-대응]] · [[네이버-순위-계산]] · [[봇탐지-우회-스택]]
