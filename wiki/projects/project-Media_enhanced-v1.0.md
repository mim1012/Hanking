# project-Media_enhanced-v1.0

> "Media Enhanced"는 위장명 — 실제로는 **카카오택시 기사용 앱 콜잡기(접근성 서비스) 안드로이드 앱 + Node.js 전화번호 라이선스/텔레메트리 백엔드**. | 도메인: 앱 자동화(콜잡기) + SaaS 라이선싱 | 스택: Android(Kotlin/Java, Compose, AccessibilityService, SQLite) + Node/Express(Supabase or MongoDB 토글), Railway 배포, Python ETL | 상태: 활성/프로덕션(형제앱 twinme/uber/rooting/turbo와 백엔드 공유). 폴더 `D:\Project\Media_enhanced-v1.0`(정본 체크아웃; `MediaEnhanced`는 얕은 복제). origin `github.com/mim1012/Media_enhanced-v1.0`.

## 핵심 자산 (다른 곳에서 주워올 것)
- **전화번호 라이선스 검증 엔드포인트** — `routes/authSupabase.js:9` `POST /api/auth/verify` — 앱 최초 실행 시 phone_number로 active/blocked/expired 게이트, `remaining_days` 반환. 미등록(`:24`)·차단(`:32`)·만료(`:40`) 3단 거부 후 성공 시 `last_auth`/`total_auths`만 갱신(`:48`). "앱 실행→서버 조회→허용/차단" 라이선싱의 canonical. → [[웹앱-인증-세션-계층]]
- **클라 라이선스 캐시(1h TTL, 오프라인 관용)** — `app/src/main/java/com/media/player/service/auth/AuthManager.java:33` — SharedPreferences 1시간 캐시, 만료 시 서버 재인증, 오프라인이면 캐시 유지. → [[브라우저-세션쿠키-영속화]]
- **디바이스 바인딩 신원 추출 폴백체인** — `app/src/main/java/com/media/player/service/auth/AuthManager.java:167` — SIM `getLine1Number()`(+82→0 정규화) → 실패 시 Android ID 앞 12자를 의사 전화번호로. 라이선스 키 파생/기기 지문에 재사용.
- **ActiveRecord-over-Supabase 파사드** — `models/UserSupabase.js:287` — Supabase 행에 Mongoose류 `.save()` 클로저 + 계산속성 `remaining_days`를 얹어, Mongoose용으로 쓴 라우트가 Supabase에서 무수정 동작. Mongo→Supabase 마이그레이션 어댑터.
- **JWT 어드민 게이트** — `routes/adminSupabase.js:8` `verifyToken` 미들웨어(`TokenExpiredError`/`JsonWebTokenError` 세분 처리) + 24h 로그인(`:50`). → [[웹앱-인증-세션-계층]]
- **원격 config 해시-diff 폴링(304형)** — `routes/turboConfigSupabase.js:89` — config JSONB의 SHA-256를 클라 `config_hash`와 비교해 변화 없으면 `{changed:false}`만 반환(대역폭 절약).
- **텔레메트리 수집(배치 insert + 자가청소 7일 TTL)** — `routes/turboConfigSupabase.js:36` — 요청당 50건 캡 배치 insert + `setInterval` 7일 초과 삭제(cron 의존 없는 서버리스 retention).
- **Telegram fire-and-forget 알림(키별 5분 쿨다운 dedup)** — `utils/telegram.js:95` — `deviceId:errorType` 키로 5분 내 중복 억제 + 확률적 맵 청소. 인메모리 알림 레이트리밋. → [[서버리스-공유-레이트리밋]](단일 인스턴스 변형)

## 주의점 / 함정
- 백엔드가 **형제 콜잡기 앱들(twinme/uber/rooting/turbo)의 멀티테넌트**(`serverSupabase.js:55`에 각 LSPosed 모듈 라우트 마운트) — 위 라이선싱/텔레메트리 패턴은 이미 4개 프런트에서 검증됨. [[project-turbo]]가 같은 백엔드(server-only 브랜치)를 공유.
- 레포 루트가 디컴파일 dex 덤프·일회성 스크립트로 지저분함. 실소스는 `app/`·`routes/`·`models/`.

## 의존 개념: [[웹앱-인증-세션-계층]] · [[브라우저-세션쿠키-영속화]] · [[서버리스-공유-레이트리밋]] · [[State-Machine-접근성서비스]]
