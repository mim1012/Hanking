# project-dorami

> 실시간 방송으로 상품을 파는 한국어 라이브 커머스 플랫폼(모노레포). | 도메인: 라이브 커머스 | 스택: Next.js16/React19(client), NestJS11/Prisma6/PostgreSQL16/Redis7(backend), Socket.IO, SRS(RTMP→HLS), FFmpeg, Kakao OAuth→JWT | 상태: 활성 (자산 밀도 최상)

## 핵심 자산 (다른 곳에서 주워올 것)
### 동시성 (dorami 최고 자산)
- `backend/src/modules/orders/inventory.service.ts:104` — 재고 원자적 조건부 감소 (canonical). → [[원자적-조건부-감소]]
- `backend/src/modules/points/points.service.ts:192` `deductPointsTx` — 포인트 원자적 차감(동일 패턴). → [[원자적-조건부-감소]]
- `backend/src/modules/orders/orders.service.ts:210` — 다건 락 순서 정렬 데드락 방지 + 재시도 트랜잭션.
- `backend/src/modules/reservation/reservation.service.ts:207` — 예약 대기열 WAITING→PROMOTED 승격 + 만료 크론(`:256`).

### 실시간 스트리밍
- `backend/src/modules/streaming/streaming.service.ts:982` — SRS webhook 방송 전이. → [[실시간-스트리밍-소켓아키텍처]]
- `backend/src/modules/websocket/socket-io.provider.ts:21` — 서비스→소켓 브로드캐스트 싱글턴.
- `backend/src/modules/restream/restream.service.ts:260` — FFmpeg 멀티플랫폼 재송출(유튜브/인스타/틱톡).
- `backend/src/main.ts:491` `/chat` — 실시간 채팅(레이트리밋 20/10s, XSS 제거, Redis 히스토리 100개/24h, ADMIN 모더레이션).

### 인증 / 원장 / 인프라
- `backend/src/modules/auth/strategies/jwt.strategy.ts:36` — JWT 즉시폐기(블랙리스트 이중체크 + AuthSession 세션폐기, 가용성 우선 degrade). → [[웹앱-인증-세션-계층]]
- `PointBalance`:595 + `PointTransaction`:613(불변 원장, balanceAfter 스냅샷), `Settlement`:544, `settlement.service.ts:113`(ExcelJS 리포트). → [[정산-포인트-원장]]
- `backend/src/common/services/encryption.service.ts:22` — AES-256-GCM 봉투 암호화 + 키 로테이션(배송지).
- `backend/src/common/filters/business-exception.filter.ts:48` — 프로덕션 context 마스킹(재고수/내부ID 유출 방지). `common/interceptors/transform.interceptor.ts` — `{data,success,timestamp}` 표준.
- `common/throttler/throttler.config.ts` — 3티어 레이트리밋. `common/adapters/redis-io.adapter.ts` — 멀티인스턴스.

### 알림 (PG 결제는 없음 — 수동 입금 + 알림톡 방식)
- `backend/src/modules/notifications/push-notification.service.ts:16` — Web Push(VAPID). `notifications/clients/kakao-talk.client.ts` — Kakao 알림톡(Bizgo). `NotificationTemplate`:451 — DB 템플릿.
- 주문은 `BANK_TRANSFER`/`PENDING`으로 생성, 판매자 입금확인 후 CONFIRMED(`depositorName` 매칭). PG API 연동 없음.

## 미확인 (추가 조사 필요)
- Settlement 커미션 계산/레코드 생성 위치(현 서비스는 조회/엑셀만). ChatMessage DB 영속화 경로(소켓은 Redis만).

## 의존 개념: [[원자적-조건부-감소]] · [[실시간-스트리밍-소켓아키텍처]] · [[웹앱-인증-세션-계층]] · [[정산-포인트-원장]] · [[멀티에이전트-역할오케스트레이션]]
