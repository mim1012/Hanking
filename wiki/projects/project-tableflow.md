# project-tableflow (폴더: `D:\Project\-qr`)

> **TableFlow** — 식당 QR 테이블 주문 + POS 관리 B2B SaaS(멀티테넌트). 고객 QR 메뉴(`/m/:storeSlug/:tableId`), 점주 admin/KDS, 대기열 키오스크. | 도메인: 웹앱/SaaS(요식 주문·POS) | 스택: Next.js15 App Router, React18, Supabase(Postgres+Realtime+Auth+Edge Functions, 51 SQL 마이그레이션), Vitest+Playwright(6디바이스), Sentry, Tailwind v4 | 상태: 활성/프로덕션급. origin `github.com/mim1012/tableflow.git`(초기 Figma Make export `@figma/my-make-file`에서 실제 앱으로 확장됨). 폴더명이 `-qr`이라 파일 슬러그는 `tableflow`로 등록(경로는 `-qr/` 프리픽스).

> 실전 인시던트로 하드닝된 DB 레벨 동시성/보안 패턴이 강점(레이스, TOCTOU, 레이트 남용, 가격 위변조, 멀티테넌트 격리).

## 핵심 자산 (다른 곳에서 주워올 것)
- **원자적 주문+아이템 생성 RPC(서버가 DB에서 가격 계산)** — `-qr/supabase/migrations/20260317000001_create_order_atomic.sql:73` `SECURITY DEFINER` 함수 — 클라 payload를 무시하고 `mi.price * item.quantity`를 **DB menu_items 행에서** 계산해 가격 위변조 차단, 유효성 위반 시 `RAISE EXCEPTION 'invalid order items'`, 같은 트랜잭션에서 테이블 occupied 표시. → [[원자적-조건부-감소]]
- **TOCTOU-safe 대기열 번호(일일 리셋, Asia/Seoul)** — `-qr/supabase/migrations/20260505102000_waiting_atomic_registration.sql:20` — 단일 원자 UPSERT `ON CONFLICT (store_id) DO UPDATE`로 순차 큐번호 + 날짜 바뀌면 1로 리셋. `waitings_store_id_queue_number_key` 실 프로덕션 충돌 수정(초기 수정본 `20260329000001_fix_queue_number_race.sql`과 함께 레이스 포스트모템 교재).
- **DB 내 레이트리밋(테이블당 5주문/분)** — `-qr/supabase/migrations/20260324000003_rate_limit_orders.sql:72` — 같은 `SECURITY DEFINER` 함수 안에서 `COUNT(*) … created_at > now()-interval '1 minute'` 검사 → API 직접 호출로 우회 불가. → [[서버리스-공유-레이트리밋]]
- **Realtime 구독 + 30s 폴링 폴백 + JWT 자동갱신** — `-qr/src/hooks/useOrders.ts:102` — Supabase Realtime에 30초 폴링 폴백, 401/403·JWT 만료 시 `supabase.auth.refreshSession()`, `knownIdsRef` 토스트 dedup. Realtime만 믿지 않는 실전 패턴.
- **주문상태 상태기계(클라+DB 공유 불변식)** — `-qr/src/lib/utils/orderStatus.ts:3` `VALID_TRANSITIONS` 맵 + `canTransition`/`getNextStatuses`, 서버측은 마이그레이션 `20260322000001_order_status_transition_check.sql`이 미러. → [[리스크-관리-상태기계]](상태전이 검증 변형)
- **멀티테넌트 감사로그 라이터** — `-qr/src/lib/server/auditLog.ts:33` — actor 롤(super_admin vs `store_members.role`) 해석 후 service-role 클라이언트로 기록(실패 시 actor 클라이언트로 graceful fallback), before/after diff 캡처. → [[정산-포인트-원장]](감사 원장 인접)

## 주의점 / 함정
- Figma Make export(`@figma/my-make-file`)가 씨앗 — 현재 코드는 원본 비즈니스 로직. package name은 leftover.
- `src/app/components/ui/`(shadcn 48개)는 읽기전용 라이브러리, 수정 금지. RLS가 `store_id` 멀티테넌트 격리 강제.

## 의존 개념: [[원자적-조건부-감소]] · [[서버리스-공유-레이트리밋]] · [[멀티테넌트-요청파이프라인]] · [[리스크-관리-상태기계]] · [[PostgREST-1000행캡-회피]]
