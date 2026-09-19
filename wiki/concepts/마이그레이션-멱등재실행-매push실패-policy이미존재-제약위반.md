# 멱등 마이그레이션 재실행 (매-push 실패 파이프라인 초록화)
> CI가 폴더의 모든 마이그를 매 push 재실행하는데 비멱등 구문(`policy already exists`, `check constraint ... violated by some row`) 때문에 런이 상시 failure → 각 구문을 재실행-안전(idempotent)하게 바꿔 한 번 성공시키면 기록되어 그린이 된다.

## 무엇 / 언제 쓰나
- Supabase/Postgres 마이그레이션을 "성공분만 추적 테이블에 기록하고 실패분은 매 실행 재시도"하는 CI 파이프라인에서, 특정 파일이 매번 실패해 런 전체가 failure로 뜰 때.
- 증상 에러: `policy "..." for table "..." already exists`, `check constraint "..." of relation "..." is violated by some row`, `column "..." does not exist`, `syntax error at or near "https"`(SQL 본문에 URL 혼입).
- DB 상태는 이미 의도대로인 경우가 많음(정책·테이블 이미 존재) → **재실행이 안전한 no-op 성공이 되게** 만드는 게 핵심. 데이터를 건드리지 않는다.

## 정본 구현 (canonical)
- `adpang-production/scripts/apply-supabase-migrations.mjs:110-208` — eligible 접두어(`add-`/`alter-`/`fix-`/`grant-`)만 적용, 성공분만 `public._schema_migrations_applied`(filename PK)에 기록. **실패분은 미기록→매 push 재시도·재실패**하는 구조(그래서 비멱등이면 영구 failure).
- `adpang-production/supabase-migrations/add-sellermate-traffic-navershopping.sql:23-26` — policy 멱등 관용구: `ALTER TABLE ... ENABLE ROW LEVEL SECURITY;` → `DROP POLICY IF EXISTS "<정확한 이름>" ON <table>;` → `CREATE POLICY ...`. (CREATE TABLE/INDEX는 `IF NOT EXISTS`로.)
- `adpang-production/supabase-migrations/add-distributors-type-media-life.sql` / `add-sellermate-referral-rewards.sql:72-78` — 기존행이 위반하는 CHECK 제약: `ADD CONSTRAINT ... CHECK (...) NOT VALID` — 기존행 검증을 스킵하고 신규/UPDATE 행에만 적용(데이터 무변경). 기존행 강제는 데이터 정리 후 `VALIDATE CONSTRAINT` 별도 실행.
- `adpang-production/supabase-migrations/add-sellermate-place-tables.sql` — 이미 다른 마이그로 drop된 레거시 컬럼을 `CREATE TABLE IF NOT EXISTS`가 재도입 못 해(기존 테이블엔 no-op) 그 컬럼 인덱스 생성만 실패 → 컬럼·인덱스 정의를 **제거**해 현행 스키마와 일치(신규설치도 정합).

## 주의점 / 함정
- **「성공만 기록」 파이프라인 = 비멱등 마이그의 영구 재실패기.** 한 번 실패하면 추적 테이블에 안 남아 매 push 재시도→재실패→런 failure. 정상 마이그의 적용까지 failure 로그에 묻힌다.
- **CHECK를 즉시검증(VALIDATE)으로 추가하면 기존 위반행에 걸려 실패.** 새 값 집합이 기존 데이터를 다 포괄하지 못하면 `NOT VALID`가 안전판. 단 NOT VALID는 기존 위반행을 남기므로 "제약이 걸렸다"는 착각 금지.
- **policy는 `CREATE POLICY IF NOT EXISTS`가 없다**(Postgres 미지원) → `DROP POLICY IF EXISTS` 선행이 정답. 한 파일에 policy가 여러 개면 전부 처리(하나만 고치면 다음 것에서 재실패).
- SQL 본문에 주석 없는 URL(`https://...`)이 섞이면 `syntax error at or near "https"`.

## 배제된 접근 (dead-end)
- **`_schema_migrations_applied`에 실패 파일명을 수동 INSERT해 스킵** — 런은 초록이 되지만 마이그 자체는 여전히 비멱등이라, DB를 처음부터 재빌드하면 똑같이 실패한다(진짜 고침 아님). 각 구문을 멱등하게 고치는 게 정본.
- **위반행을 지우거나 컬럼을 drop해서 제약을 맞추기** — 데이터 손실 위험. NOT VALID/정의제거로 상태 무변경 no-op 성공화가 안전.

## 관련: [[Supabase-마이그레이션-코드배포-분리]] · [[PostgREST-RPC-옵션파라미터추가-PGRST203-오버로드-무중단]]
