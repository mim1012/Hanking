# PostgREST 1000행 캡 회피 (Supabase 최신값 오산 방지)

> Supabase/PostgREST의 기본 1000행 응답 캡에 `.in()`/`.select()` 벌크 조회가 잘려 "최신 N"을 잘못 계산하는 버그를, 서버측 집계(DISTINCT ON RPC) 또는 키셋(seek) 순회로 회피하는 패턴.

## 무엇 / 언제 쓰나
Supabase(PostgREST)에서 히스토리·로그처럼 행이 많은 테이블을 조회해 "그룹별 최신 1행" 또는 "전체 순회"가 필요할 때. PostgREST는 요청당 기본 1000행(`max-rows`)에서 응답을 조용히 잘라내므로, 클라이언트가 벌크로 끌어와 앱에서 max/latest를 계산하면 **일부만 보고 최신값을 오산**한다. 특히 정렬이 없거나 오름차순이면 "최신"이 캡 바깥에 있어 통째로 누락된다.

## 정본 구현 (canonical)
- **집계(aggregate) — DISTINCT ON RPC** — `adpang-production/supabase/migrations/20260707_sellermate_naver_slot_rank_summary.sql:18` `sellermate_naver_slot_rank_summary(p_slot_ids bigint[])`. 21만 행 히스토리를 클라로 안 끌어오고, DB에서 `DISTINCT ON (sid) … ORDER BY sid, rank_date DESC, created_at DESC`(`:37`)로 슬롯당 최신 1행 + 시작 1행(`earliest_r` `:41`)만 반환. `current_rank>0` 부분 복합 인덱스(`:13`)로 뒷받침. 호출부 `adpang-production/src/app/api/sellermate/slot-naver/route.ts:337`(bigint가 문자열로 오므로 `Number()` 정규화 `:358`).
- **순회(iterate) — 키셋(seek) 페이지네이션** — `adpang-production/src/lib/experiment-log-paging.ts:41` `buildSeekOrFilter(cursor, idColumn='slot_id')` — DB 호출 없는 순수 함수(vitest 가능). 컴파운드 seek 조건을 PostgREST `.or()` 문자열로 생성: `created_at.lt.X,and(created_at.eq.X,<idColumn>.lt.Y)`. **일부러 `(created_at, idColumn) DESC + .lt`로 스캔**해, 캡에 걸려도 잘리는 쪽이 최신이 아니라 "가장 오래된" 구간이 되게 함(`:37` 주석). `idColumn` 인자로 실험로그(`slot_id`)·순위히스토리(`slot_status_id`) 두 테이블에 재사용. truncated(캡에 걸림) vs incomplete(오류)를 `isCapTruncated`(`:53`)로 구분.

- **키셋(seek) 순회 — raw fetch(.mjs)용** — `zero-landing/tools/backfill-slot-ids.mjs:33` `fetchAllPaged(pathBase)`. supabase-js 없이 `fetch`로 PostgREST를 직접 때리는 스크립트용. `&order=id.asc&limit=1000` + `&id=gt.<직전 페이지 마지막 id>` 커서. 마지막 페이지가 `PAGE_SIZE` 미만이면 종료하고, 커서가 진전 없으면(`nextCursor === cursor`) 무한루프 대신 throw. **uuid PK에서도 그대로 동작** — `keyword_pipeline_candidates`(uuid) 74,786행을 페이지 500으로 150회 순회해 누락·중복 0 실측(2026-08-20). 포팅 사례: `zero-landing/tools/keyword-verifier-resume.mjs:63`, `zero-landing/tools/kimtaedong-keyword-local-test.mjs:88`.
- **offset 페이징 — supabase-js용 저장소 로컬 포팅** (2026-08-20 시점 브랜치 `investigate-strategy-a-pc-migration` 미커밋 — main 머지 전까지 lint가 file missing으로 잡는다) — `turafic_update/shared/postgrest-paging.ts:21` `fetchAllRows` (adpang 정본의 직접 포팅), `:54` `fetchAllByIds(ids, build)` — `.in()` 배열을 200개씩 청크로 쪼갠 뒤 각 청크를 다시 페이징해서 **URL 폭발(HTTP 414)과 행 캡을 한 번에** 막는다. 호출 예 `turafic_update/scripts/reverify-keyword-pool.ts:38`.
- **RPC 결과 자체도 페이징 — fetchAllRows를 RPC 호출에 재사용** — `adpang-production/src/app/api/sellermate/slot-naver/route.ts:464` `fetchSummary()`가 `supabase.rpc('sellermate_naver_slot_rank_summary', { p_slot_ids: slotIds }).range(from, to)`를 `fetchAllRows`(`adpang-production/src/lib/postgrest-paging.ts:18`)로 감싸 페이징. RPC가 `TABLE(...)`을 반환하면 PostgREST가 **일반 select와 똑같이 `max-rows`(기본 1000)를 그 RPC 응답에도 적용**하므로, `p_slot_ids`에 5,000개 슬롯 ID를 한 번에 넘겨도 결과가 1,000행에서 잘린다. `.rpc()` 빌더도 `.range()`를 지원하므로 동일한 `fetchAllRows` 헬퍼를 그대로 재사용 가능.

## 언제 어느 쪽
- **그룹별 최신 1행**만 필요 → 집계 RPC(왕복 1회, DB가 다 계산). 목록 화면의 "현재순위/시작순위"처럼.
- **전 구간을 실제로 다 읽어야** 함(집계로 못 줄임) → 키셋 순회. 커서를 이어가며 여러 페이지.

## 주의점 / 함정
- **"PostgREST 1000행 사일런트 캡"** — 응답이 잘려도 에러가 아니다. `.in()` + 앱단 max 계산은 행이 1000을 넘는 순간 조용히 틀린 값을 낸다(목록↔상세 모달 순위 불일치의 실제 근원 버그). 정렬 없거나 ASC면 최신이 캡 밖으로 사라진다.
- **"카운트 창(window) 방향과 소비(consume) 방향 불일치"(2026-08-08 실측)** — 위 캡과는 다른 함정: `created_at DESC LIMIT N` 창으로 "최근 N행 중 claimed 비율"을 세려 했는데, 실제 claim 함수(`claim_naver_visitor_cookie` 계열, FIFO)는 `created_at ASC`로 **가장 오래된 행부터** 집어간다. 풀이 N행을 넘는 순간, 창이 보는 "최근 N행"에서 정작 claimed로 바뀌는 행들(가장 오래된 것들)이 창 밖으로 밀려나 카운트가 체계적으로 과소집계(작은 풀에선 무증상이라 늦게 발견됨). 고침: `adpang-production/src/lib/android-admin-server.ts`의 `getAdminSnapshot()` — 카운트는 `select('id', {count:'exact', head:true})` 조건별 쿼리 2개(available/claimed-in-24h)로 분리하고, "최근 N건 미리보기"만 별도의 `created_at DESC LIMIT 10` 쿼리로 남김. **일반화**: 큐/풀 테이블에서 "최근 N행 창"으로 상태 카운트를 낼 때, 그 테이블을 소비하는 함수의 소비 순서(ASC/DESC, FIFO/LIFO)가 창의 정렬 방향과 반대면 캡이 없어도 과소/과대집계가 난다 — 카운트는 항상 exact-count 쿼리로, 미리보기는 별도 쿼리로 분리할 것.
- **"집계 RPC를 썼는데도 재발"(2026-08-18 실측)** — 위 "그룹별 최신 1행 RPC"로 이미 캡을 피했다고 믿기 쉽지만, **입력 배열이 크면(슬롯 5,000개) RPC의 출력 자체가 슬롯 수만큼(최대 5,000행)이라 그 출력이 다시 1000행 캡에 걸린다.** `sellermate_naver_slot_rank_summary(p_slot_ids)`를 5,000개 ID로 단발 호출하면 `RPC_ROWS_RETURNED: 1000`, ID 1001~5000의 순위가 조용히 누락되고 앱은 `rank_summary_failed`로만 감지(원인은 못 씀). 격리된 스테이징 프로젝트에 운영 스키마를 read-only로 복제해 5,000행 실측을 하고 나서야 드러남 — `.select().limit(N)`으로 소량(3개) 테스트만 하면 이 증상이 재현 안 돼 "RPC는 정상"으로 오판하기 쉽다. 고침: 위 "RPC 결과 자체도 페이징" 항목. **일반화**: "그룹별 최신 1행" RPC라도 그룹(슬롯) 개수가 1000을 넘으면 출력 행 수도 넘을 수 있다 — RPC라서 안전하다고 가정하지 말고 호출부에서 입력 배열 길이 vs 반환 행 수를 항상 대조할 것.
- **"큰 limit / Range 헤더로 캡을 이겼다는 착각"(2026-08-20 실측)** — `limit`을 크게 주면 캡을 넘는다고 믿기 쉽지만 **서버 `db-max-rows`가 항상 이긴다.** 실측(`landing_keyword_pool` active+mid_verified, 실제 10,023행): 무조건 `1000` / `limit=50000` → `1000` / offset 페이징 11회 → `10023`(일치). `Range: '0-999'` 헤더를 직접 붙이는 것도 같은 함정이며, "내가 1000개만 원해서 그렇게 쓴 것"처럼 보여 리뷰에서 더 안 걸린다(`zero-landing/tools/keyword-verifier-resume.mjs` 수정 전 코드).
- **"resume/done 집합 도구에서의 영구 정지"(2026-08-20 실측)** — 위 캡이 **커서형/재개형 배치 도구**에 걸리면 단순 누락을 넘어 진행이 영구히 멈춘다. `전량 조회 → done 집합으로 필터 → 앞에서 N개 처리` 구조에서 조회가 앞쪽 1000행으로 잘리면, 그 1000행이 전부 done이 되는 순간 이후 실행은 매번 "남은 것 0"만 찍고 성공 종료한다 — 뒤쪽 행은 영원히 처리되지 않는데 **에러도 경고도 없다.** 같은 이유로 `landing_keyword_pool` 재검증 도구(`turafic_update/scripts/reverify-keyword-pool.ts`)는 5만 행 중 1000행, 커버리지 2%로 돌면서 로그엔 "active 키워드: 1000개"라고 정상처럼 찍고 있었다.
- bigint PK/컬럼은 PostgREST에서 **문자열**로 온다 → 비교/계산 전 `Number()` 정규화 필수(안 하면 `"9" > "10"` 문자열 비교 오류).
- 키셋 커서는 정렬키와 tie-breaker(고유 컬럼)를 함께 물어야 중복/누락이 없다.

## 배제된 접근 (dead-end)
- **`.in()` 벌크 조회 후 앱에서 latest 계산** — 원래 방식. 1000행 캡에 잘려 오산. 이 버그를 고치려고 캡을 늘리거나(`Prefer: count`/`Range` 헤더 땜질) 페이지 크기를 키우는 것도 임시방편 — 데이터가 더 늘면 재발. 근본 해법은 "앱에서 계산하지 말고 DB에서 집계"(RPC) 또는 "캡을 전제하고 안전한 방향으로 순회"(키셋).

## 관련: [[네이버-순위-계산]] · [[요청-코얼레싱-인플라이트]] · [[project-adpang-production]] · [[레거시-코드체계-호환-필터]]
