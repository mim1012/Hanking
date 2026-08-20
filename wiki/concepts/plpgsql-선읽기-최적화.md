# plpgsql-선읽기-최적화
> plpgsql 함수의 `WHERE` 절 인라인 서브쿼리가 후보 행마다 반복 실행돼 슬롯 수 비례 렉을 내는 패턴을, `DECLARE` 블록에 변수를 선읽기해서 1회로 줄이는 기법.

## 무엇 / 언제 쓰나
`SELECT … FROM table WHERE … AND col < (SELECT val FROM settings WHERE key = 'x')` 형태로 인라인 스칼라 서브쿼리가 `WHERE` 절 안에 있으면, 외부 쿼리가 후보 행을 평가할 때마다 서브쿼리가 한 번씩 실행된다. 설정 테이블이 항상 같은 값을 반환할 때라도 플래너가 상관 서브쿼리로 처리하면 행 수만큼 반복된다. 슬롯 100개 → 서브쿼리 100회 이상. 설정값이 한 번 읽으면 충분한 상수라면 `DECLARE` 블록에서 미리 읽어 변수에 넣고, `WHERE` 절에선 변수만 참조한다.

같은 서브쿼리가 `SELECT` 목록과 `WHERE` 모두에 나오면(두 곳에서 인라인) 2× 중복이다.

## 정본 구현 (canonical)
```sql
-- 잘못된 패턴 (per-row subquery):
WHERE coalesce(dc.success_count, 0) < (SELECT daily_target FROM strategy_runtime_settings WHERE setting_key = 'strategy_a_daily_target')

-- 수정 패턴 (선읽기):
DECLARE
  v_daily_target integer;
BEGIN
  SELECT r.daily_target INTO v_daily_target
  FROM strategy_runtime_settings r WHERE r.setting_key = 'strategy_a_daily_target';
  -- ...
  WHERE coalesce(dc.success_count, 0) < v_daily_target
```
- `dev-cache/Temp/zen-nocdp-sm01/supabase-migrations/022-optimize-claim-strategy-slot.sql:71` — `v_daily_target integer;` declare
- `dev-cache/Temp/zen-nocdp-sm01/supabase-migrations/022-optimize-claim-strategy-slot.sql:83` — 선읽기 SELECT INTO
- `dev-cache/Temp/zen-nocdp-sm01/supabase-migrations/022-optimize-claim-strategy-slot.sql:125` — WHERE절 변수 참조

실제 적용: adpang-production DB `cwsdvgkjptuvbdtxcejt`, 022 마이그레이션, 2026-08-20. 적용 후 `pg_proc` `pg_get_functiondef` read-back으로 `v_daily_target integer` 존재 확인(검증된 성공).

## 다른 구현 / 변형
- 설정값이 자주 바뀌거나 캐시가 필요 없는 경우에는 이 기법이 불필요. 위 케이스에서 `strategy_runtime_settings`는 거의 바뀌지 않는 상수에 가까운 값이라 선읽기가 정확하다.

## 주의점 / 함정
- **named failure: "슬롯 수 비례 WHERE절 인라인 서브쿼리 반복 렉"** — `claim_strategy_slot`이 슬롯이 늘어날수록 호출 한 번에 수백ms씩 걸리는 현상이 이 버그에서 발생했다. 100슬롯 기준 `strategy_runtime_settings` 서브쿼리가 `WHERE`(1회), `SELECT 목록`(1회) 두 곳에서 각각 반복 → 슬롯당 2회, 전체 200회 이상.
- **named failure: "그룹/단건 이중 경로 포트 드리프트"** — 같은 기능의 그룹(group)·단건(single) RPC가 별도로 존재할 때, 한쪽에만 패치를 적용하면 다른 쪽이 소리 없이 뒤처진다. 이번 케이스에서 014(normalize 수정)·016(3일 재사용 방지)이 `claim_strategy_slot_group`에만 적용되고 `claim_strategy_slot`(단건)에는 미이식 상태로 여러 마이그레이션이 쌓였다. 패치 후 **항상 group과 single 양쪽을 대조 확인**한다.

## 배제된 접근 (dead-end)
- **CTE로 설정값 추출** — `WITH settings AS (SELECT daily_target FROM …) SELECT … FROM slots, settings WHERE …` 형태는 문법상 불가. `FOR UPDATE OF s SKIP LOCKED`가 있는 단일 `SELECT INTO v_slot …` 구문과 결합하면 CTE를 사용할 수 없고, `FOR UPDATE SKIP LOCKED`는 CTE 안에서 지원하지 않는다(PostgreSQL 제약). 원자적 잠금 의미론(atomic locking semantics)이 깨지므로 배제.

## 관련: [[Postgres-함수내-정규화비교-불일치]] · [[동시-클레임-랜덤화-떼몰림-방지]] · [[project-adpang-production]]
