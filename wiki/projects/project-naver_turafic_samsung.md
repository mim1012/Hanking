# project-naver_turafic_samsung

> naver-traffic-android-samsung의 자매 클론 — 봇 서버를 adpang에서 분리(Phase 1)하는 파생 작업 레포. | 도메인: 네이버 트래픽(서버 분리) | 스택: Kotlin(앱) + Node 표준 라이브러리(서버, npm 의존 0), Supabase | 상태: 활성 (스캐폴드/서버 분리 + 전략 실험 모니터링)

## 핵심 자산 (다른 곳에서 주워올 것)
- `server/lib/crypto.js:15` `encryptSecret()` — 네이버 계정 자격증명 AES-256-GCM 암호화(`v1:iv:tag:cipher` base64url) + `safeEqual`(상수시간).
- `server/lib/tasks.js:109` `resolveGroupStrategyOverride()` — 디바이스 그룹 단위 전략 오버라이드(lease 시 그룹값이 product/slot 값보다 우선).
- `server/lib/landing.js:45,66` `buildNaverSuggestUrl()` / `landingRedirect()` — Toss 리워드 실측 트래픽 시그니처(mtp_sug, base62 ackey, 무관 시드 acq, acr 6-8) 서버측 위조 + `Referrer-Policy: no-referrer` 307 랜딩. Kotlin `SamsungBrowserStrategyB.buildSuggestUrl`과 파라미터 시그니처 동일 유지.
- `server/lib/admin.js:510` `getExperimentHistory()`(+`buildStints:463`, `rankStatsInWindow:494`) — 전략 실행 로그와 외부 순위 피드를 조인해 전략(A/B/G/H)별 순위개선 스코어카드 산출. `server/web/admin/history.html` 모니터링 UI가 소비.
- `server/dev-server.js` — npm 의존성 0, Node 표준 라이브러리만으로 계정 lease/heartbeat/rotation API. 무의존 계약 테스트 서버 패턴.
- `docs/server-schema.sql`, `docs/vercel-supabase-api.md` — 프로덕션 API 계약 문서.

## 관계
- **모체** = [[project-naver-traffic-android-samsung]] (0.1.x 릴리스 계보, 전략 A/B/C/G/H 전체). 이 레포는 그 봇 서버 분리 파생본. 버리는 커밋도 자매 클론 + reflog에 보존.

## 의존 개념: [[IP-로테이션]] · [[캡차-차단-대응]] · [[네이버-순위-계산]] · [[봇탐지-우회-스택]]
