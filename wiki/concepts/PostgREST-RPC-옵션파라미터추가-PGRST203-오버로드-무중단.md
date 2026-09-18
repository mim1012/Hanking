# PostgREST-RPC-옵션파라미터추가-PGRST203-오버로드-무중단
> 이미 배포된 Supabase RPC(함수)에 파라미터를 하나 더 붙이면서 "코드 배포 ↔ DB 마이그레이션" 두 파이프라인의 완료 시점이 어긋나도 무중단이게 하는 법. 잘못하면 `PGRST203 Could not choose the best candidate function between ...`(오버로드 모호성) 또는 배포 순서 창(window)에서 claim 실패가 난다.

## 무엇 / 언제 쓰나

라이브 supabase-js `rpc('fn', {...})` 를 쓰는 함수에 **선택 파라미터 1개를 추가**할 때(예: claim RPC 에 surface/전략 필터 추가). DB 함수 변경은 마이그레이션(즉시 라이브)이고, 그 함수를 호출하는 JS 는 Vercel 배포(나중 라이브)라 둘의 완료 시점이 다르다([[Supabase-마이그레이션-코드배포-분리]]). 그 사이 창에서 구/신 JS 와 구/신 함수 시그니처가 섞여도 깨지지 않아야 한다.

핵심 3원칙:
1. **새 파라미터는 `DEFAULT NULL`(또는 빈값 no-op)** — 안 넘기면 종전과 byte-identical 동작.
2. **옛 arity 함수는 DROP, 새 arity 만 남긴다** — 3-arg 와 4-arg 를 둘 다 남기면 3-key 호출이 두 후보에 매칭돼 PostgREST 가 PGRST203 로 거부할 수 있다. 새 4-arg 하나만 있으면 3-key 호출은 default 로 resolve, 4-key 호출은 exact match — 둘 다 모호성 없음.
3. **호출측 인자는 킬스위치가 되게** — env(예: `ANDROID_TRAFFIC_SURFACE`)를 `?? '기본값'`(빈문자 보존)으로 읽고 `값 || null` 로 넘겨, env 를 비우면 `null` → 필터 inert(DB 손 안 대고 롤백).

이러면 **DB 선적용/JS 후배포**(구 JS 3-key → default NULL no-op)든 **JS 선배포/DB 후적용**(신 JS 4-key 가 3-arg뿐인 DB에 걸려 RPC 에러 → 폴백 경로가 흡수)든 어느 순서든 안전하다.

## 정본 구현 (canonical)

- `adpang-production/supabase-migrations/add-android-mshopping-surface-filter.sql` (PR #83, merged `e852c74`)
  — 원자 claim RPC `claim_android_traffic_task` 에 `p_surface text DEFAULT NULL` 추가 + 후보 WHERE 에 `AND (p_surface IS NULL OR btrim(p_surface) = '' OR t.strategy = p_surface)` 한 줄. 맨 앞에서 `DROP FUNCTION IF EXISTS public.claim_android_traffic_task(text, boolean, boolean);` 로 3-arg 제거 후 4-arg 만 `CREATE OR REPLACE`.
  — 검증: `pg_get_function_identity_arguments` = `p_group_id text, p_slot_pinned_strict boolean, p_assigned_only boolean, p_surface text` (4-arg **단일**), `position('p_surface' in prosrc) > 0` = true.
- `adpang-production/src/lib/android-admin-server.ts` `leaseTask()` (PR #83)
  — 상수 `const ANDROID_TRAFFIC_SURFACE = process.env.ANDROID_TRAFFIC_SURFACE ?? 'm-shopping';` (`??` 로 빈문자 보존=킬스위치). RPC 호출에 `p_surface: ANDROID_TRAFFIC_SURFACE || null` 추가. 원본 함수 정의는 `adpang-production/supabase-migrations/add-android-traffic-claim-rpc.sql:55-118`.
- 배포 자동화: `adpang-production/.github/workflows/apply-migrations.yml`(main push + `supabase-migrations/**` 변경 트리거) → `scripts/apply-supabase-migrations.mjs:12`(적격 prefix `add-`/`alter-`/`fix-`/`grant-`만), `:110-123`(파일명 정렬), `:214-237`(파일당 단일 트랜잭션 + `_schema_migrations_applied` 파일명 키로 1회). 로그에서 `applied: <파일>.sql` 로 실제 적용 확인.

## 다른 구현 / 변형

- **시그니처를 안 바꾸고 런타임 플래그로 게이트** — `turafic_update/supabase-migrations/068-pc-tonggeom-surface-filter.sql:126-134,231-233,275-276`. PC claim `claim_strategy_slot_v3` 에 파라미터 대신 `strategy_runtime_settings.pc_surface_filter_enabled`(기본 false=inert) 컬럼을 읽어 `not (v_pc_surface and strategy='pc-tonggeom') or (...)` 조건으로 필터. 여러 워커가 공유하는 큰 함수라 시그니처를 못 건드릴 때(또는 A/B 로 켜고 끌 때) 이 방식이 낫다. android RPC 는 안드로이드 전용·호출자 1곳이라 파라미터 추가가 더 깔끔했다.

## 주의점 / 함정

- **「엉뚱한 경로 필터」= no-op** — 같은 기능에 claim 경로가 둘(원자 RPC vs REST 폴백)일 때, 라이브가 아닌 쪽에만 필터를 넣으면 코드는 그럴듯한데 프로덕션에서 전혀 안 먹는다. adpang 은 `claimRpcEligible = !exclusiveCustomerId && ...`(`android-admin-server.ts` ~2830)가 true(=`android_runtime_settings.mobile_exclusive_customer_id` 미설정=테이블 비어있음)면 **원자 RPC 가 라이브**, false 면 leaseOldest 폴백. 어느 게 라이브인지 **런타임 설정을 실제 조회해** 확정하고 그쪽을 고쳐야 한다. 양쪽 다 넣으면(정본=RPC, 폴백=REST) 스위치가 바뀌어도 안전.
- **오버로드 모호성(PGRST203)** — 3-arg 를 안 지우고 4-arg(default) 를 추가하면, 3-key POST 가 3-arg(exact) 와 4-arg(via default) 둘 다에 매칭돼 `Could not choose the best candidate function` 로 거부될 수 있다. **옛 arity DROP 필수.**
- **claim RPC 는 소모형** — `claim_android_traffic_task` 는 후보를 `FOR UPDATE SKIP LOCKED` 후 **DELETE** 로 반환한다. 스모크로 직접 호출하면 실제 큐행이 사라진다 → 검증은 호출이 아니라 `pg_get_function_identity_arguments`/`prosrc`/큐 분포/leases 로만.
- **잡 실패 ≠ 내 마이그 실패** — `apply-supabase-migrations.mjs` 는 파일마다 독립 트랜잭션이라 다른 레거시 마이그가 `policy already exists` 등으로 무더기 `errored` 나도 내 파일은 `applied` 될 수 있다. 워크플로 잡 색이 빨개도 로그에서 **내 파일명의 `applied:`** 를 직접 확인.

## 배제된 접근 (dead-end)

- **필터를 JS `leaseOldest` 폴백에만 추가** — 처음엔 REST 폴백 쿼리에 `.eq('strategy','m-shopping')` 를 넣었는데, 라이브는 원자 RPC 경로(`claimRpcEligible=true`)라 **프로덕션 무효과(no-op)**. 리뷰로 발각. 진짜 수정은 RPC SQL 함수 쪽이었다. (폴백에도 남겨두되 정본은 RPC.)
- **DB 함수를 Management API 로만 직접 바꾸고 레포엔 안 남김** — 다음 배포/CI 재적용 때 원복되거나 팀이 소스와 어긋난 상태를 못 본다. 반드시 `supabase-migrations/` 마이그레이션 파일 + PR 로 정본화(사용자 지시 "체리픽해서 PR 머지").

## 관련: [[Supabase-마이그레이션-코드배포-분리]] · [[PostgREST-RPC-40001충돌-30초무응답-PT409]] · [[PostgREST-1000행캡-회피]] · [[project-adpang-production]] · [[project-naver_turafic_samsung]]
