# project-dorami

> 실시간 방송으로 상품을 파는 한국어 라이브 커머스 플랫폼(모노레포). | 도메인: 라이브 커머스 | 스택: Next.js16/React19(client), NestJS11/Prisma6/PostgreSQL16/Redis7(backend), Socket.IO, SRS(RTMP→HLS), FFmpeg, Kakao OAuth→JWT | 상태: 활성 (자산 밀도 최상)

## 핵심 자산 (다른 곳에서 주워올 것)
### 동시성 (dorami 최고 자산)
- `backend/src/modules/orders/inventory.service.ts:34` `batchConfirmDecreaseStockTx` — 재고 감소 (canonical, 2026-07-07 재설계). **패턴이 바뀌었다**: 예전 `WHERE stock >= qty` 조건부 감소(주문 *생성* 시점, 부족하면 throw)를 삭제하고, **결제 *확정* 시점에 FOR UPDATE 비관적 락 + `Math.max(0, …)` clamp**(oversell 허용, throw 안 함)로 전환(`a0c7b0de feat(orders): decrement stock at payment confirmation, not order creation`). 미결제 PENDING_PAYMENT 주문은 재고를 잡지 않고, 물리 재고는 admin 입금확인 때만 감소. 락은 `sortedItems`(`:41`)로 정렬 후 `SELECT … FOR UPDATE`(product `:55`, variant `:64`)를 **선취득**해 다건 주문 동시 확정 데드락을 방지(IN 리스트는 `Prisma.join` 파라미터 바인딩 → injection-safe). 감소 자체는 clamp된 절대값 set(product `:116`/`:119`, variant `:83`/`:86`). 호출부는 결제확정 플로우 `backend/src/modules/admin/admin.service.ts:1963`(같은 Serializable 트랜잭션에서 `PENDING_PAYMENT→PAYMENT_CONFIRMED` 가드된 flip 직후 1회 감소), 되돌리기는 `batchRestoreStockTx`(`inventory.service.ts:143`). DB 레벨 belt-and-braces로 non-negative CHECK 제약(`backend/prisma/migrations/20260707120000_add_stock_non_negative_check/migration.sql:10`)까지 이중화. Golden Flow #8(Admin 주문관리/입금확인)에 직결. → [[원자적-조건부-감소]]
- `backend/src/modules/points/points.service.ts:192` `deductPointsTx` — 포인트 원자적 차감(동일 패턴). → [[원자적-조건부-감소]]
- `backend/src/modules/orders/orders.service.ts:210` — 다건 락 순서 정렬 데드락 방지(`withOrderTransactionRetry`로 감싸는 재시도 트랜잭션, `:102`).
- `backend/src/modules/reservation/reservation.service.ts:207` — 예약 대기열 WAITING→PROMOTED 승격 + 만료 크론(`:256`).
- `backend/src/modules/auth/auth.service.ts:397` `_doRefreshToken` — Redis NX 락을 먼저 획득한 뒤 세션 비교/토큰 로테이션(락-먼저-비교 패턴). 락 경합 시 `waitForCachedRefreshResult`로 결과를 폴링·재사용 — 동시 리프레시로 인한 대량 로그아웃 방지. → [[실시간-토큰리프레시-락-먼저-비교]] [[요청-코얼레싱-인플라이트]] [[웹앱-인증-세션-계층]]
- `client-app/src/lib/hooks/use-auth.ts:84` `fetchCurrentUser` — 세션 부트스트랩 401 처리: 저장된 로그인 정보가 있는 상태에서 `/users/me`가 401을 반환하면 즉시 anonymous로 떨어뜨리지 않고 리프레시 1회 + 재검증을 시도(`:67` `hasPersistedIdentity` 가드). 리프레시 자체가 실패해도 서버가 리프레시 쿨다운을 걸지 않은 경우(`getRefreshCooldownRemainingMs() === 0`)는 "진짜 만료"가 아닌 일시적 장애로 간주해 `BootstrapRecoverableError`(`:43`)를 던지고 `resolveBootstrapFailure`(`:50`)가 기존 로그인 상태를 유지(fail-open). 만료 접속-복구형 세션 부트스트랩의 재사용 가능한 형태. → [[웹앱-인증-세션-계층]]

### 실시간 스트리밍
- `backend/src/modules/streaming/streaming.service.ts:1044` `authenticateStream()` — SRS `on_publish` 웹훅 인증 + LIVE 전이(만료 48h 연장, Redis 캐시 갱신). 실제 HTTP 진입점은 `streaming.controller.ts:382`(`POST srs-auth`). → [[실시간-스트리밍-소켓아키텍처]]
- `backend/src/modules/websocket/socket-io.provider.ts:21` — 서비스→소켓 브로드캐스트 싱글턴.
- `backend/src/modules/restream/restream.service.ts:260` — FFmpeg 멀티플랫폼 재송출(유튜브/인스타/틱톡).
- `backend/src/main.ts:491` `/chat` — 실시간 채팅(전송 레이트리밋 20/10s at `:568`, XSS 제거, Redis 히스토리 100개/24h, ADMIN 모더레이션).
- `backend/src/modules/streaming/streaming.controller.ts:104` `armDevTestStream` (`GET /api/streaming/dev/arm-test-stream`) — 라이브 방송 테스트용 "안전한 dev 백도어" 패턴: 고정 스트림키를 PENDING으로 재무장해 인코더(OBS/PRISM) 재설정 없이 반복 테스트를 가능케 함. 이중 게이트(`:114` `ENABLE_DEV_AUTH==='true'` AND `:122` `APP_ENV==='staging' || localhost`)로 프로덕션은 컴파일 시점이 아니라 설정값 두 곳(compose 하드코드 `false` + `APP_ENV=production`)에서 각각 독립적으로 차단 — 보안 리뷰가 "compose 게이트가 리팩터링돼도 여전히 막힌다"를 별도 계층에서 검증한 사례. 다른 프로젝트의 스테이징 전용 테스트 보조 엔드포인트 설계에 재사용 가능.

### 클라이언트 상태 관리 (Next.js)
- `client-app/src/hooks/useLiveLayoutMachine.ts:73` `deriveSnapshot` + `:92` `computeLayout` — 4개의 독립 상태축(connection/stream/uiMode/overlay, 각각 별도 리듀서 `:142`~`:212`)을 하나의 `LiveSnapshot` enum(`LIVE_NORMAL`/`LIVE_TYPING`/`RETRYING`/`NO_STREAM`/`ENDED`)으로 합성한 뒤, 그 enum만으로 UI 레이아웃(`LiveLayout`)을 순수 함수로 도출하는 패턴. 리듀서는 비공개(`:142`)로 캡슐화하고 파생 함수만 `export`해 유닛테스트 대상을 좁힘. 실시간성이 있고 상태축이 여러 개인 UI(재생/연결/입력모드/오버레이가 서로 얽히는 화면) 일반에 재사용 가능한 "다축 리듀서 + 스냅샷 파생 + 레이아웃 프로젝션" 아키텍처.

### 인증 / 원장 / 인프라
- `backend/src/modules/auth/strategies/jwt.strategy.ts:36` — JWT 즉시폐기(블랙리스트 이중체크 + AuthSession 세션폐기, 가용성 우선 degrade). → [[웹앱-인증-세션-계층]]
- `PointBalance`:595 + `PointTransaction`:613(불변 원장, balanceAfter 스냅샷), `Settlement`:544, `settlement.service.ts:113`(ExcelJS 리포트). → [[정산-포인트-원장]]
- `backend/src/common/services/encryption.service.ts:22` — AES-256-GCM 봉투 암호화 + 키 로테이션(배송지).
- `backend/src/common/filters/business-exception.filter.ts:48` — 프로덕션 context 마스킹(재고수/내부ID 유출 방지). `common/interceptors/transform.interceptor.ts` — `{data,success,timestamp}` 표준.
- `common/throttler/throttler.config.ts` — 3티어 레이트리밋. `common/adapters/redis-io.adapter.ts` — 멀티인스턴스.
- `docker-compose.prod.yml:87` `NODE_OPTIONS: --max-old-space-size=768` + `backend/src/modules/streaming/live-metrics.service.ts:172` `logHeap()`(60초 간격, `process.memoryUsage()` 기반, warn 레벨) — 컨테이너에 `mem_limit`만 걸고 V8 힙 크기 플래그를 안 주면 V8이 호스트 RAM 기준으로 힙을 잡아 GC가 압박을 못 느끼고, 라이브 방송 피크에 커널이 스택트레이스 없이 프로세스를 SIGKILL한다. 텔레메트리도 `os.totalmem()/freemem()`(호스트 기준)으로 재던 시절엔 "메모리 정상"으로 보여 원인 특정이 수주간 불가능했다.
- `.github/workflows/deploy-production.yml:113` `golden-flow-gate` 잡 — user+admin 골든플로우 Playwright 스펙이 전부 통과해야만 프로덕션 프로모션이 진행(`:539` `needs: [...golden-flow-gate]`). 아래 교훈 절 "처방"이 실제 CI 하드 게이트로 구현된 사례.

### 알림 (PG 결제는 없음 — 수동 입금 + 알림톡 방식)
- `backend/src/modules/notifications/push-notification.service.ts:16` — Web Push(VAPID). `notifications/clients/kakao-talk.client.ts` — Kakao 알림톡(Bizgo). `NotificationTemplate`:451 — DB 템플릿.
- 주문은 `BANK_TRANSFER`/`PENDING`으로 생성, 판매자 입금확인 후 CONFIRMED(`depositorName` 매칭). PG API 연동 없음.

## 교훈 — 채팅 두더지잡기 포렌식 (2026-07-06, git 3,597커밋 + 코드 분석; 2026-07-07 갱신: 처방 이행 확인)

채팅/소켓 fix 커밋 154+건, 최악 구간은 이틀 18 fix(04-26~27). 이름 붙인 실패 패턴 5개:
1. **삼중 소켓 진입점** — `main.ts` 수동 /chat 핸들러 + websocket.gateway + chat.gateway 공존. ChatGateway 리팩토링→revert→제거(`969dee42`→`12649ce3`→`2c2cc97e`).
2. **인증-게스트 진자** — 채팅 전송 정책이 "인증 강제↔게스트 허용"을 이틀에 4회 왕복(`faf4556b`↔`04437ed3` 등). 정책 결정이 기록 안 돼 세션마다 재발명.
3. **토큰 만료 소켓사** — 1h+ 방송 중 access 만료→재연결 handshake 실패. 2월~7월 동일 증상 반복(`969dee42`, `3861cedc`, `718be805`, `ec9ee5a0`).
4. **재연결 잡음/유령 소켓** — rapid unmount로 중복 소켓·메모리 누수(`4ddc5f3b`, `1fbb2bf9`).
5. **환경별 전송 미스매치** — ws/wss·namespace·localhost fallback(`74cab857`, `9a3ee03a`, `c80af29e`).

**구조 원인**: 채팅 배관은 격리돼 있으나 (a) 단일 `authenticateSocket`(`backend/src/common/middleware/ws-jwt-auth.middleware.ts:145`)을 전 네임스페이스가 공유, (b) refresh 싱글톤(`client-app/src/lib/api/client.ts:56`)을 HTTP+전 소켓이 공유, (c) 채팅 마운트가 auth store에 종속(`client-app/src/app/live/[streamKey]/page.tsx:192` liveChatEnabled) — 그래서 채팅 fix가 auth를 건드리고 auth fix가 채팅을 터뜨린다.

**처방 이행 확인 (2026-07-06 커밋 `8bf2413f`, 포렌식 당일 같은 날 반영):**
- 골든플로우 #2(`client-app/e2e/chat-send-message.spec.ts:117-127`) — A-CHT-02(발신자 즉시표시)/A-CHT-05(수신자 실시간 전파)가 이제 하드 `expect().toBeVisible()`. 셀렉터도 데스크톱(`메시지 입력...`)+모바일(`메시지를 입력하세요...`) placeholder를 `:visible` union으로 매칭해 모바일 레이아웃에서 조용히 스킵되던 문제가 해결됨. **"스펙이 종이호랑이"였던 이전 상태는 더 이상 사실이 아니다.**
- 신규 스펙 `client-app/e2e/chat-token-expiry-reconnect.spec.ts` — 실패 패턴 3(토큰 만료 재연결) 회귀를 전담 가드. accessToken 쿠키만 만료시키고 소켓 재연결을 강제한 뒤 `POST /api/auth/refresh` 성공 + accessToken 재발급을 hard-assert.
- CI 배포 게이트(`.github/workflows/deploy-production.yml:113` `golden-flow-gate`)가 프로덕션 프로모션을 골든플로우 e2e 통과에 하드 종속(`:539`) — "실패할 수 없는 스펙"이 게이트 역할을 못 하던 문제가 인프라 레벨에서도 봉쇄됨.
- 프로젝트 자체 `AGENTS.md`에 "Chat Regression Contract"로 불변식(정책은 재논의 금지, 공유 레이어 4파일 수정 시 골든플로우 #1/#2/#6 동반 검증, 단일 소켓 진입점 유지)이 명문화됨.

**남은 리스크**: 백엔드 소켓 통합테스트는 여전히 0개(검증은 E2E 레이어에서만 이뤄짐).

## 교훈 — raw SQL 테이블명 드리프트 (2026-07-07, 재고 감소 재설계 중 2회 발생)

이름 붙인 실패 패턴: **Prisma 모델명(`"Product"`/`"ProductVariant"`)을 `$queryRaw`·원시 마이그레이션에 그대로 써서, 실제 `@@map` snake_case 테이블(`products`/`product_variants`)을 못 찾아 런타임 `42P01 relation does not exist`(500)로 터진다.** Prisma Client 고수준 API는 매핑을 해주지만 raw SQL은 안 해준다. 같은 결함이 두 곳에서 각각 발생:
- 마이그레이션(`41a0dfcf`) — CHECK 제약 대상 테이블명이 모델명이라 CI `prisma migrate deploy`가 42P01로 실패(배포 게이트가 정상 차단 → staging 미적용).
- 런타임 FOR UPDATE 락(`e5e685b1`) — `inventory.service.ts:55`/`:64`의 두 `$queryRaw`가 `FROM "Product"`/`FROM "ProductVariant"`였음. 실 DB에서 결제확정이 500으로 throw. **모킹된 유닛 테스트는 못 잡는다**(`tx.$queryRaw`가 stub이라 SQL 문자열 미검증) → staging 실DB API 검증에서만 발견.

처방: Prisma raw SQL/마이그레이션의 테이블명은 모델명이 아니라 `@@map` 실테이블명을 쓴다. 이 클래스는 모킹 유닛테스트 사각지대 → 실DB 통합/스테이징 검증 필수. (단일 프로젝트 관측이라 concept 미승격, 재고 감소 재설계 자산의 부속 교훈으로 기록. mt5/StockDante 등에서 재관측 시 concept 후보.)

## 미확인 (추가 조사 필요)
- Settlement 커미션 계산/레코드 생성 위치 — 여전히 미확인. `prisma.settlement.create()` 호출이 코드베이스 전체에 0건(grep 확인); `settlement.service.ts`는 조회·엑셀 리포트만 구현. `Settlement.commission` 컬럼(schema.prisma:551)은 존재하지만 채우는 코드가 없다.
- ChatMessage DB 영속화 경로 — 여전히 소켓은 Redis만 사용(`main.ts` `/chat` 핸들러가 `chat:{liveId}:history`에만 씀). 다만 `ChatMessage` 테이블 자체는 schema.prisma:135에 존재하고, `admin.service.ts:1285,1291`이 최근 7일 카운트 쿼리 대상으로 읽는다 — 즉 어드민 대시보드가 항상 0을 반환하는 죽은 지표를 노출 중일 가능성.

## 의존 개념: [[원자적-조건부-감소]] · [[실시간-스트리밍-소켓아키텍처]] · [[웹앱-인증-세션-계층]] · [[정산-포인트-원장]] · [[멀티에이전트-역할오케스트레이션]] · [[요청-코얼레싱-인플라이트]] · [[실시간-토큰리프레시-락-먼저-비교]]
