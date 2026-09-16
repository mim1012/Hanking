# Remove-Item-blocked-훅오인-SQL별칭-del-ManagementAPI적용

> PowerShell 도구 안에서 Supabase Management API로 SQL을 보낼 때 SQL 별칭 `as del`이 훅에 `Remove-Item on system path '*' is blocked`로 오인되어 차단된다 — `del`/`rm`/`ri` 토큰과 `count(*)`를 피하고(`count(1)`), 적용은 ref 일치 검사 → 적용 직전 함수 정의 스냅샷 → 파일 전체 적용 → 파일 내 READ-BACK 쿼리 + 실호출 스모크 순으로 한다.

## 무엇 / 언제 쓰나

PowerShell 스크립트/도구가 SQL 문자열을 Supabase Management API로 보낼 때, 그 문자열 안의 토큰이 PowerShell 명령/별칭처럼 보여 안전 훅(GateGuard류)이 파일시스템 삭제로 오인해 차단하는 상황, 그리고 마이그레이션을 실제 프로덕션에 적용하기 직전에 무엇을 확인·백업해야 안전한지 필요한 경우에 쓴다.

## 정본 구현 (canonical)

- `D:\Project\sellermate-contracts\migrations\058-purge-expired-slot-data.sql:1-24` — 마이그레이션 파일 헤더 형식: `Owner` → `Depends on`(선행 마이그레이션·FK 관계) → 사용자 명시 결정(archive가 아니라 delete 등, 날짜 포함) → `리뷰 반영 (N건)` 절.
- `D:\Project\sellermate-contracts\migrations\058-purge-expired-slot-data.sql:368-369` — 파일 하단 `READ-BACK` 절. 적용 전 필수로 `pg_constraint`를 조회해 `landing_keyword_pool`을 참조하는 FK가 이 마이그레이션이 처리하는 것(pc_dispatches/snapshot_051) 외에 더 있는지 확인하라는 안내가 파일 자체에 남아 있다.
- `D:\Project\sellermate-contracts\migrations\058-purge-expired-slot-data.sql:198-202` — 삭제 전 스냅샷 패턴의 실물: `insert into public.pool_purge_snapshot_058 select p.*, now(), 'orphan' from ... where s.id is null;` — 원샷 삭제 대상을 별도 스냅샷 테이블에 먼저 행 단위로 복사한 뒤에만 delete를 실행한다.
- `D:\Project\zero-landing\supabase-migrations\rollback\pick_landing_query.pre-v2.2026-09-16.sql` — 함수(`CREATE OR REPLACE FUNCTION`) 교체형 변경의 스냅샷 패턴(파일 전체, 줄번호 없음): 적용 직전 `pg_get_functiondef`로 뽑은 현재 프로덕션 정의를 파일로 그대로 저장해 롤백 정본으로 삼는다.
- `D:\Project\zero-landing\supabase-migrations\pick-landing-query-v2-surface-strategy.sql:20-28` — 적용 기록(적용 시각·방법·사용자 승인)과 read-back 쿼리 목록(`pg_get_functiondef(...) ilike '%...%'`, 스모크 호출)을 파일 헤더에 그대로 남기는 형식.

검증된 성공: 2026-09-16 058(고아 pool 17,354행 등 정리) · `pick_landing_query_v2` 두 건을 이 절차(ref 확인 → 스냅샷 → 적용 → 파일 내 read-back + 스모크)로 적용했고, read-back이 전 항목 기대값과 일치했다(3버킷 잔존 0/0/0, 정의 관련 4항목 true, 스모크 반환값 확인). SQL 별칭을 `on_delete_action`으로 바꾸고 `count(*)` 대신 `count(1)`을 쓴 뒤로는 훅 차단이 재현되지 않았다.

## 다른 구현 / 변형

- `D:\Project\sellermate-contracts\migrations\058-purge-expired-slot-data.sql:198-202`(스냅샷 테이블·행 단위, DELETE 롤백용) vs `D:\Project\zero-landing\supabase-migrations\rollback\pick_landing_query.pre-v2.2026-09-16.sql`(함수 정의 텍스트·파일 전체, CREATE OR REPLACE 롤백용) — 둘 다 "적용 직전 스냅샷을 먼저 남긴다"는 같은 원칙이지만, 삭제형 변경은 영향받는 행을 통째로 별도 테이블에 복사하고 함수 교체형 변경은 `pg_get_functiondef` 텍스트를 파일로 저장한다는 점에서 스냅샷의 형태 자체가 다르다.

## 주의점 / 함정

- **"훅의 별칭 오인"** — 스크립트 문자열 안의 SQL 토큰 `del`이 PowerShell 별칭(`Remove-Item`의 alias)으로 해석되어 차단된다. `count(*)`의 `*`도 와일드카드 경로처럼 오인될 수 있어 `count(1)`로 피한다.
- **"plpgsql은 참조 함수 존재를 생성 시 검사하지 않음"** — `public.normalize_query_phrase()`처럼 함수 본문이 참조하는 다른 함수가 없어도 `CREATE OR REPLACE FUNCTION` 자체는 성공하고, 실제 호출 시마다 런타임 에러가 난다. 적용 전 `pg_proc`에서 의존 함수 존재를 확인해야 한다.
- **"staged 파일이 현행 프로덕션 정의와 다름"** — 마이그레이션 파일을 써 둔 뒤 다른 작업이 먼저 프로덕션 함수를 바꿨을 수 있다(예: `pool_expiry_enabled` 게이트). 적용 직전에 `pg_get_functiondef`로 현행 정의를 다시 읽어 차이를 반영해야 한다.
- **"FK 누락으로 DELETE 실패"** — 참조하는 FK를 `pg_constraint where confrelid = ...`로 전부 확인하지 않으면 삭제가 중간에 실패한다. 자식(참조하는 쪽) → 부모(참조되는 쪽) 순서로 삭제해야 한다.

## 배제된 접근 (dead-end)

- 리뷰 없이 초안 그대로 적용 — 058 초안은 `keyword_pipeline_pc_dispatches`가 `landing_keyword_pool`을 참조하는 FK(NO ACTION)를 빠뜨렸을 것으로 보이며(061 선례가 실제로 이 순서로 실패를 겪음), 그대로 적용했다면 Part 2 삭제가 FK 위반으로 중간에 멈췄을 것이다.
- 롤백 정본을 "원본 마이그레이션 파일"만으로 남기기 — 원본 마이그레이션이 적용된 뒤 현행 정의가 이미 달라져 있을 수 있어 무의미하다. 반드시 적용 직전에 뽑은 스냅샷이어야 한다.

## 관련

[[PostgREST-RPC-40001충돌-30초무응답-PT409]] · [[PC검증-적립해도-함대성공률-불변-surface-blind-pool]]
