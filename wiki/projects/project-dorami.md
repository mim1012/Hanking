# project-dorami

> 실시간 방송으로 상품을 파는 한국어 라이브 커머스 플랫폼(모노레포). | 도메인: 라이브 커머스 | 스택: Next.js16/React19(client), NestJS11/Prisma6/PostgreSQL16/Redis7(backend), Socket.IO, SRS(RTMP→HLS), FFmpeg, Kakao OAuth→JWT | 상태: 활성 (자산 밀도 최상)

## 핵심 자산 (다른 곳에서 주워올 것)
### 동시성 (dorami 최고 자산)
- `backend/src/modules/orders/inventory.service.ts:34` `batchConfirmDecreaseStockTx` — 재고 감소 (canonical, 2026-07-07 재설계). **패턴이 바뀌었다**: 예전 `WHERE stock >= qty` 조건부 감소(주문 *생성* 시점, 부족하면 throw)를 삭제하고, **결제 *확정* 시점에 FOR UPDATE 비관적 락 + `Math.max(0, …)` clamp**(oversell 허용, throw 안 함)로 전환(`a0c7b0de feat(orders): decrement stock at payment confirmation, not order creation`). 미결제 PENDING_PAYMENT 주문은 재고를 잡지 않고, 물리 재고는 admin 입금확인 때만 감소. 락은 `sortedItems`(`:41`)로 정렬 후 `SELECT … FOR UPDATE`(product `:55`, variant `:64`)를 **선취득**해 다건 주문 동시 확정 데드락을 방지(IN 리스트는 `Prisma.join` 파라미터 바인딩 → injection-safe). 감소 자체는 clamp된 절대값 set(product/variant 각 경로, variant `:90` `Math.max(0, …)`), **clamp가 조용히 삼키지 않는다(2026-07-18 v1.5.10)**: 부족분 `oversoldBy`(`inventory.service.ts:92`)를 결과에 실어 admin API가 `buildOversoldItems`(`admin.service.ts:265`)로 `data.oversold`에 표면화(단건 `:2063`, 상태변경 `:2257`, 벌크 집계 `:2360` — 벌크는 orderId 태깅)하고, 프론트 admin 주문화면이 경고 배너로 렌더(`client-app/src/app/admin/orders/page.tsx:710`, 상세 `[id]/page.tsx:352`). 호출부는 결제확정 플로우 `backend/src/modules/admin/admin.service.ts:1963`(같은 Serializable 트랜잭션에서 `PENDING_PAYMENT→PAYMENT_CONFIRMED` 가드된 flip 직후 1회 감소), 되돌리기는 `batchRestoreStockTx`(`inventory.service.ts:143`). DB 레벨 belt-and-braces로 non-negative CHECK 제약(`backend/prisma/migrations/20260707120000_add_stock_non_negative_check/migration.sql:10`)까지 이중화. Golden Flow #8(Admin 주문관리/입금확인)에 직결. → [[원자적-조건부-감소]]
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

## 교훈 — 라이브 뷰어 종료화면 오표시 + wedge-recovery 리마운트 폭주 (2026-07-11, grok 리뷰 발단 → Opus 교차검증)

발단: xAI **grok CLI** 코드리뷰가 라이브 스트리밍 FSM 픽스 커밋(`6c3be42d`·`48c18bdf`)에서 HIGH 2건을 지목 → Opus가 파일 직접 교차검증으로 확정. 이름 붙인 실패 패턴 2개:

1. **LIVE→OFFLINE 렌더 혼동 (종료된 방송이 "곧 시작"으로 표시)** — 뷰어 페이지 `client-app/src/app/live/[streamKey]/page.tsx`의 최상단 렌더 가드 `if (streamStatus.status !== 'LIVE')`가 **"방송 시작 전(PENDING)"과 "방송 종료(LIVE→OFFLINE)"를 같은 status로 뭉쳐** 종료 방송에 "아직 방송 전이에요 / 곧 시작됩니다"를 렌더. FSM엔 이미 올바른 종료 오버레이("방송이 종료되었습니다", `useLiveLayoutMachine.ts` `computeLayout` ENDED 케이스)가 있었지만 **status 기반 early-return이 먼저 실행돼 도달 불가능한 죽은 코드**였음. 폰을 잠깐 잠갔다 복귀한 시청자가 종료 방송을 "곧 시작"으로 오인해 이탈. 일반 10초 grace 종료 경로에도 동일 적용된 **선재 버그**(skip-grace 커밋이 만든 게 아니라 더 빨리 도달하게 했을 뿐). 처방: `deriveOfflineScreen(snapshot)`(`useLiveLayoutMachine.ts:168`) — 스냅샷 ENDED면 종료화면, 아니면 대기화면. 소비처 `page.tsx:882`.
2. **wedge-recovery 리마운트 폭주** — 백엔드가 unpublish grace 동안 DB를 LIVE로 지연 유지하는 사이 스트림이 실제 종료되면, 새로 마운트된 VideoPlayer가 다시 `STREAM_ENDED`를 쏨 → 가속 5초 폴(`snapshot==='ENDED'`)이 매번 `playerSessionSeed`를 증가시켜 **DB가 OFFLINE 될 때까지 5초마다 플레이어 리마운트 난타**. 원 커밋의 *"MEDIA_ERROR는 RETRYING으로 매핑되니 루프 안전(Opus APPROVE)"* 논리는 **에러 경로만 막을 뿐 깨끗하게 끝난 스트림의 STREAM_ENDED 재진입은 못 막는 사각**이 있었음(grok이 지적). 처방: `shouldRecoverWedge(now, lastRecoveredAt, 30s)`(`useLiveLayoutMachine.ts:154`, 상수 `:152`)로 쿨다운 — 진짜 일시 wedge는 첫 폴 즉시 1회 복구, 재진입은 억제해 진실된 ENDED("방송이 종료되었습니다") 유지. 게이트 `page.tsx:696`, ref `:177`.

**검증**: 순수함수 유닛 회귀 7건(`client-app/src/hooks/__tests__/useLiveLayoutMachine.test.ts:232` shouldRecoverWedge, `:264` deriveOfflineScreen — 37/37 green) + E2E `client-app/e2e/live-ended-screen.spec.ts`(status를 route-mock으로 LIVE→OFFLINE 전이시켜 "방송이 종료되었어요" 표시 & "곧 시작됩니다" 부재를 hard-assert). **골든 #1 `live-page.spec.ts`가 `/live` 목록 페이지만 보고 `[streamKey]` 뷰어 페이지의 종료상태를 안 봤던 사각을 메움** — massage 환불 교훈([[레거시-코드체계-호환-필터]])의 "클라가 쓰는 실제 화면을 아무도 안 열어봤다"와 동형.

**메타 교훈**: 순수함수 유닛테스트는 결정 로직만 잠근다. `page.tsx`가 그 함수를 호출해 올바른 문구를 렌더하는 **JSX 배선은 유닛 사각** → 뷰어 화면 종료상태는 반드시 E2E(route-mock)로 가드. 크로스모델 리뷰(grok가 지목 → Opus가 파일 열어 근본원인 확정)가 단일 모델 자기검토보다 사각을 잘 잡은 사례.

## 교훈 — 카트 재고 점유는 죽은 코드였고, 강제 타이머는 설정을 무력화했다 (2026-07-17, 실방송 중 만료 166건 사고)

발단: 실LIVE 방송 중 장바구니가 대량 만료(166건, SQL로 복구). 조사 결과 서로 얽힌 결함 3개:
1. **죽은 재고 점유** — 결제기준 재고차감 전환(위 재고 재설계, "unpaid orders hold no stock") 이후에도 카트 레벨 재고 예약 코드가 남아 "표시 재고 4 / 가용 0" 모순을 만들었다. 카트는 재고를 잡지 않는다는 새 모델과 모순되는 잔재 → 제거(`9323c7cd`). 현재 계약은 `cart.service.ts:132-139` 주석에 명문화: **타이머는 순수 UX 긴급장치, 재고 의미 없음. 재고는 오직 admin 입금확인에서만 감소.**
2. **강제 타이머가 셀러 설정 무력화** — 라이브 상품은 `timerEnabled=false`여도 강제로 카운트다운을 걸었음(`280975bb`가 도입) → 방송 중 셀러의 "타이머 끔" 설정이 조용히 무시됨. 처방: `isCartTimerActive`(`cart.service.ts:141`)가 상품의 `timerEnabled`만 존중.
3. **만료 크론의 필터 기준** — 만료 크론(`cart.service.ts:542` `expireTimedOutCarts`, 매분)은 **상품이 아니라 카트 자신의** `timerEnabled`(`:551`)로 거른다. 상품 설정을 나중에 바꿔도 기존 카트 행에는 소급되지 않으므로, 배포는 새 카트만 고친다 — 기존 카트 180건은 SQL로 별도 무장해제 필요했다.

**이름 붙인 실패 패턴: "죽은 설정(dead config)"** — 관리자 화면의 `default_cart_timer_minutes` 전역 설정은 UI에 존재하지만 만료 로직이 읽지 않는 dead config였다(실제 만료는 상품별 `timer_duration`). 설정 UI가 있다고 배선돼 있다고 믿지 말 것 — 설정값을 읽는 코드까지 추적해야 한다.

## 교훈 — 게이트는 존재가 아니라 실행으로만 게이트다 (2026-07-17)

이름 붙인 실패 패턴 2개:
1. **잠자는 게이트** — 실LIVE 골든플로우 CI 잡(`.github/workflows/golden-live-chat.yml`)이 만들어진 뒤 **9일간 실행 0건**으로 잠들어 있었다(workflow_dispatch 전용). 그 사이 카트 강제타이머 회귀(위 교훈)는 이 스펙이 잡도록 설계된 바로 그 버그였는데 통과해 배포됨. 스펙의 LIVE 분기가 도달 불가능하면 조용히 degrade하던 bail-out도 `REQUIRE_LIVE=1`에서 하드 실패로 전환. 첫 그린런(2026-07-17)으로 GitHub 러너→staging RTMP egress(tcp/1935) 도달 확인 후 나이트리 스케줄 무장(`:36-37`, cron `30 22 * * *` = KST 07:30). ffmpeg 테스트패턴을 staging SRS에 RTMP publish→`on_publish`로 진짜 LIVE를 세워 사람 인코더 없이 A-CHT-05(채팅 왕복)+B-CART-01/02(카트 타이머)를 검증하는 구조는 재사용 가치 있음.
2. **조작하는 게이트** — 야간 QA 워크플로우가 실제 검증 없이 성공 리포트를 만들어내는(fabricating) 상태라 삭제(`2bd1fb8a`). 실패할 수 없는 게이트(종이호랑이)의 상위 호환 악형: 실행은 되는데 결과가 거짓. 게이트 감사 시 "존재하나"가 아니라 "최근에 실행돼 실패할 수 있었나"를 물어야 한다.

## 미확인 (추가 조사 필요)
- Settlement 커미션 계산/레코드 생성 위치 — 여전히 미확인. `prisma.settlement.create()` 호출이 코드베이스 전체에 0건(grep 확인); `settlement.service.ts`는 조회·엑셀 리포트만 구현. `Settlement.commission` 컬럼(schema.prisma:551)은 존재하지만 채우는 코드가 없다.
- ChatMessage DB 영속화 경로 — 여전히 소켓은 Redis만 사용(`main.ts` `/chat` 핸들러가 `chat:{liveId}:history`에만 씀). 다만 `ChatMessage` 테이블 자체는 schema.prisma:135에 존재하고, `admin.service.ts:1285,1291`이 최근 7일 카운트 쿼리 대상으로 읽는다 — 즉 어드민 대시보드가 항상 0을 반환하는 죽은 지표를 노출 중일 가능성.

## 의존 개념: [[원자적-조건부-감소]] · [[실시간-스트리밍-소켓아키텍처]] · [[웹앱-인증-세션-계층]] · [[정산-포인트-원장]] · [[멀티에이전트-역할오케스트레이션]] · [[요청-코얼레싱-인플라이트]] · [[실시간-토큰리프레시-락-먼저-비교]]

## 재사용 자료 — 라이브커머스 SaaS 손익 모델 (2026-09-22)

- `dorami/docs/business-proposal-2026-09-22/internal-business-analysis.md:1` — 가입비·월 정액·사용량 초과금을 분리한 사업성 모델. AWS 실청구 미확인, 한국/서울 공개 단가와 계획 환율 기반 추정.
- `dorami/docs/business-proposal-2026-09-22/verification.md:1` — Excel 재개방·수식 851개 오류 0개·화질/환율/인건비/고객구성/과금상한 검증. 계산 입력은 같은 폴더 XLSX에서 수정.
- 주의: AWS 앱 멀티테넌트와 CloudFront 네이티브 multi-tenant distribution은 별개. 정액제 표준 distribution과 허용량 상향 비용을 구분하고 원본→CDN 전송료를 중복 계산하지 않는다. 실제 신규 서비스 성능과 계정 적격성은 미검증.
- 신규 구현 패턴이 아닌 검증된 계산 자료의 위치만 등록한다. [[멀티테넌트-요청파이프라인]] · [[실시간-스트리밍-소켓아키텍처]]
