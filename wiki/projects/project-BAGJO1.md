# project-BAGJO1 (백조오브제 / baekjo-temp)

> 프리미엄 반려생활 큐레이션 + 펫슈어런스(펫보험) + B2B 케어키트 에코시스템 커머스 플랫폼("백조오브제", Quiet Luxury 톤). | 도메인: 반려동물 커머스 SaaS(수주) | 스택: Next.js 16.2.10(App Router)/React 19.2.4/TS5, Tailwind v4, framer-motion 12, Supabase(service-role 단일 클라), next-auth v5, Toss Payments SDK 2, Playwright 1.61 | 상태: **백엔드 실전환 진행 중(2026-07-17 기준)** — Supabase가 화면의 진실 소스, `src/lib/*/repo.ts` 16개 + 마이그레이션 0001–0041, 결제 상태기계·배송(스마트택배)·admin CRUD 실연동. `src/data/{products,brands}.ts` 정적 정본은 **삭제**됨(be/kill-static-seed-canon). 폴더명 `BAGJO1`, package `baekjo-temp`.

## 핵심 자산 (다른 곳에서 주워올 것)
- **"콘센트/outlet" 데이터 접근 파사드 (drift-proof mock↔real-API seam)** — `src/lib/storage.ts`(1,527줄, 브라우저 측) — 모든 화면은 named 함수로만 읽고 쓰고, 컴포넌트에서 `fetch`/`localStorage` 직접 호출 금지. 함수 시그니처를 고정한 채 내부만 mock→API로 교체하면 프론트는 한 줄도 안 바뀐다(AGENTS.md §4 제1원칙). 서버는 storage가 아니라 `src/lib/<domain>/repo.ts`를 직접 호출(자기 API HTTP 왕복 금지, §10-2). 기계 강제: `eslint.config.mjs:22` `no-restricted-imports`가 `@/data/products`·`@/data/brands`의 컴포넌트 import를 에러로 차단.
- **결제 관찰↔판단 분리 상태기계 (Toss)** — `src/lib/payments/decide.ts` 순수 결정 함수: confirm/webhook/reconcile 3개 라우트가 각자 복제하던 판단을 `PaymentObservation`(none/authoritative/declined/unknown, `:42`) × 주문상태 매트릭스 하나로 통합. **브랜디드 타입 불변식**(`:58` `PAYMENT_ACTION_BRAND` 비공개 심볼) — `decidePaymentAction`을 거치지 않고는 `restoreConfirming` 액션을 구성할 수 없어 "불명이면 취소 금지"가 타입 수준에서 성립(codex HIGH 방어). I/O는 전부 `execute.ts`·라우트 책임. 재고 증감은 Postgres RPC(`supabase/migrations/0021_decrement_stock_for_order.sql`·`0023_restore_stock_for_order.sql`)로 원자화 — [[원자적-조건부-감소]] 변형.
- **싱글턴 JSONB config repo (~28줄 × 7도메인)** — `src/lib/kits/repo.ts:12` `getKitsConfig`/`:23` `saveKitsConfig` — `id='default'` 한 행의 `value jsonb`에 설정 통째 upsert, 행 없으면 null→`defaultXConfig` 폴백(옆 `config.ts`). kits·partners·qna·survey·settings·categorySettings·insuranceContent 7개 도메인이 같은 모양. 테이블 설계 없이 admin 편집 가능한 설정 화면을 최소 비용으로 DB화할 때 복제.
- **택배 조회 어댑터 (스마트택배/Sweet Tracker)** — `src/lib/tracking/sweettracker.ts` — 스키마 무지(schema-agnostic) 순수 함수, 실측 기반 방어 파싱. 함정 실측 2건 내장: t_code는 선행0 포함 **문자열**(우체국 '01', `:10`) · 미등록 송장도 **HTTP 200 + `result:'N'`** 응답(`:13`, 상태코드만 보면 오탐). Swagger 문서는 축약 모델이라 실측 응답을 진실로 둔다.
- **인증 이중 방어 (next-auth v5 + Next 16 proxy)** — `src/proxy.ts:35`(⚠️ Next 16은 `middleware.ts`가 아니라 `proxy.ts`) matcher `/admin/*`·`/api/admin/*` + 매 요청 `requireAdmin` DB 재조회(JWT role은 로그인 시점 스냅샷이라 강등이 세션 만료까지 안 먹는 문제 방어). 타이밍 오라클 방지: `src/lib/auth.ts:10` `DUMMY_PASSWORD_HASH`를 유저 부재 시에도 항상 검증(`:33`). 소셜 로그인 role은 'user' 하드 고정. 상세 [[카카오-네이버-소셜로그인-AuthJS]].
- **규칙기반 가중치 추천 엔진 (설문→상품/보험/키트)** — `src/data/survey.ts` `getSurveyResult` — 다중선택 답을 선언적 `SurveyResultRule[]`에 매칭해 가중 점수 합산 + default fallback. Golden Flow #1(1분 맞춤 진단) 구동.
- **hydration 가드 `useMounted`** — `src/lib/useMounted.ts:7` — `useSyncExternalStore`로 서버=false·마운트후=true. localStorage 의존 화면 게이트.
- **크로스탭 이벤트 동기 장바구니** — `src/lib/cart.ts:25` — 변경 후 `window.dispatchEvent(new Event('cart-updated'))`로 헤더 배지 라이브 갱신.
- **수주 골든플로우 배포게이트 (실가동)** — AGENTS.md §7 골든플로우 7종 + CI `verify`(typecheck+build+lint) required check + **시각 회귀 게이트**(`tests/golden/visual.spec.ts`, 7경로×2뷰포트=14장, Vercel Preview 대상 픽셀 비교, 베이스라인은 CI Linux 전용) — 상세 [[시각-회귀-베이스라인-게이트]]. contract-project-gates 규칙의 실동작 구현체.
- **카카오·네이버 소셜 로그인 (Auth.js v5)** — canonical은 [[카카오-네이버-소셜로그인-AuthJS]] (이 프로젝트). 서버 jwt 콜백 upsert로 멤버 합류, admin 승격 차단.

## 주의점 / 함정
- **정적 seed 정본 사고 (2026-07-17 — [[목데이터-수명-단일정본]]의 발원지)** — UI mock였던 `src/data/{products,brands}.ts`가 "재시드 정본"으로 승격돼 같은 값이 파일·시드 SQL·라이브 DB 세 곳에 손으로 적혔고, 불일치 수습 마이그레이션 0014~0018 + 프로덕션→파일 **역기입**(0035 정리 국면)까지 발생. 파일 삭제 + `/admin` 단일 입력구 + `supabase/migrations/`를 복구 정본으로 확정하며 종결.
- **미커밋 SQL prod 직접 적용 유실 사고 (2026-07-17)** — 미커밋 `0021_official_brand_catalog_sync.sql`을 Management API로 prod에 직접 적용 → 어느 브랜치에도 안 남아 SQL 유실(`_migrations`는 이름·시각만 저장), 결과가 상품 이미지 404. 이후 규칙: 스키마/데이터 변경 SQL의 prod 도달 경로는 "마이그레이션 커밋→PR 머지→CI migrate 잡" 하나뿐(AGENTS.md §10-8).
- **마이그레이션 번호 함정** — `0005`·`0032` 결번, `0004b` letter suffix, **`0034` 2개**(catcode·shipments 번호 충돌), `0041`도 2개(concerns·shipments_confirm). 새 번호 딸 때 파일명 나열 확인 필수.
- **InsuranceApplication 이중 필드(canonical `name`/`breed` + 레거시 `ownerName?`/`petBreed?`, `src/types/index.ts:188`·`:196`)** — 과거 "잠재 drift 버그"로 기록했으나 현재는 폼(`src/app/insurance/apply/page.tsx:65`·`:78`)이 레거시 필드를 채워 쓰고 repo(`src/lib/insurance/repo.ts:81`·`:89`)가 왕복 매핑해 **봉합 상태**. 단 이중 필드 자체는 존치라 새 쓰기 경로가 레거시 필드를 빼먹으면 admin 화면(ownerName 읽음)이 다시 깨진다 — [[레거시-코드체계-호환-필터]] 패턴.
- **Supabase RLS 무력** — 클라이언트가 `src/lib/supabase/server.ts` 하나뿐이고 service-role 키 사용. API 라우트 가드(`requireAdmin`)가 유일한 방어선 — 새 라우트에 가드 누락 = 금고 개방(AGENTS.md §10-3).
- **Playwright 기본 타깃이 로컬이 아니라 라이브 Vercel 프리뷰**(`playwright.config.ts` baseURL 폴백) — 무심코 돌리면 로컬 코드가 아니라 배포본을 검증. `E2E_BASE_URL=http://localhost:3000` 명시 시에만 dev 서버 자동 기동.

## 의존 개념: [[웹앱-인증-세션-계층]] · [[레거시-코드체계-호환-필터]] · [[카카오-네이버-소셜로그인-AuthJS]] · [[서버리스-응답후작업-after]] · [[시각-회귀-베이스라인-게이트]] · [[원자적-조건부-감소]] · [[목데이터-수명-단일정본]] · [[반응형-오프캔버스-모바일]]
