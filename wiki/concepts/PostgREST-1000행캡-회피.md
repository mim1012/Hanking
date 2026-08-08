# PostgREST 1000행 캡 회피 (Supabase 최신값 오산 방지)

> Supabase/PostgREST의 기본 1000행 응답 캡에 `.in()`/`.select()` 벌크 조회가 잘려 "최신 N"을 잘못 계산하는 버그를, 서버측 집계(DISTINCT ON RPC) 또는 키셋(seek) 순회로 회피하는 패턴.

## 무엇 / 언제 쓰나
Supabase(PostgREST)에서 히스토리·로그처럼 행이 많은 테이블을 조회해 "그룹별 최신 1행" 또는 "전체 순회"가 필요할 때. PostgREST는 요청당 기본 1000행(`max-rows`)에서 응답을 조용히 잘라내므로, 클라이언트가 벌크로 끌어와 앱에서 max/latest를 계산하면 **일부만 보고 최신값을 오산**한다. 특히 정렬이 없거나 오름차순이면 "최신"이 캡 바깥에 있어 통째로 누락된다.

## 정본 구현 (canonical)
- **집계(aggregate) — DISTINCT ON RPC** — `adpang-production/supabase/migrations/20260707_sellermate_naver_slot_rank_summary.sql:18` `sellermate_naver_slot_rank_summary(p_slot_ids bigint[])`. 21만 행 히스토리를 클라로 안 끌어오고, DB에서 `DISTINCT ON (sid) … ORDER BY sid, rank_date DESC, created_at DESC`(`:37`)로 슬롯당 최신 1행 + 시작 1행(`earliest_r` `:41`)만 반환. `current_rank>0` 부분 복합 인덱스(`:13`)로 뒷받침. 호출부 `adpang-production/src/app/api/sellermate/slot-naver/route.ts:337`(bigint가 문자열로 오므로 `Number()` 정규화 `:358`).
- **순회(iterate) — 키셋(seek) 페이지네이션** — `adpang-production/src/lib/experiment-log-paging.ts:41` `buildSeekOrFilter(cursor, idColumn='slot_id')` — DB 호출 없는 순수 함수(vitest 가능). 컴파운드 seek 조건을 PostgREST `.or()` 문자열로 생성: `created_at.lt.X,and(created_at.eq.X,<idColumn>.lt.Y)`. **일부러 `(created_at, idColumn) DESC + .lt`로 스캔**해, 캡에 걸려도 잘리는 쪽이 최신이 아니라 "가장 오래된" 구간이 되게 함(`:37` 주석). `idColumn` 인자로 실험로그(`slot_id`)·순위히스토리(`slot_status_id`) 두 테이블에 재사용. truncated(캡에 걸림) vs incomplete(오류)를 `isCapTruncated`(`:53`)로 구분.

## 언제 어느 쪽
- **그룹별 최신 1행**만 필요 → 집계 RPC(왕복 1회, DB가 다 계산). 목록 화면의 "현재순위/시작순위"처럼.
- **전 구간을 실제로 다 읽어야** 함(집계로 못 줄임) → 키셋 순회. 커서를 이어가며 여러 페이지.

## 주의점 / 함정
- **"PostgREST 1000행 사일런트 캡"** — 응답이 잘려도 에러가 아니다. `.in()` + 앱단 max 계산은 행이 1000을 넘는 순간 조용히 틀린 값을 낸다(목록↔상세 모달 순위 불일치의 실제 근원 버그). 정렬 없거나 ASC면 최신이 캡 밖으로 사라진다.
- **"카운트 창(window) 방향과 소비(consume) 방향 불일치"(2026-08-08 실측)** — 위 캡과는 다른 함정: `created_at DESC LIMIT N` 창으로 "최근 N행 중 claimed 비율"을 세려 했는데, 실제 claim 함수(`claim_naver_visitor_cookie` 계열, FIFO)는 `created_at ASC`로 **가장 오래된 행부터** 집어간다. 풀이 N행을 넘는 순간, 창이 보는 "최근 N행"에서 정작 claimed로 바뀌는 행들(가장 오래된 것들)이 창 밖으로 밀려나 카운트가 체계적으로 과소집계(작은 풀에선 무증상이라 늦게 발견됨). 고침: `adpang-production/src/lib/android-admin-server.ts`의 `getAdminSnapshot()` — 카운트는 `select('id', {count:'exact', head:true})` 조건별 쿼리 2개(available/claimed-in-24h)로 분리하고, "최근 N건 미리보기"만 별도의 `created_at DESC LIMIT 10` 쿼리로 남김. **일반화**: 큐/풀 테이블에서 "최근 N행 창"으로 상태 카운트를 낼 때, 그 테이블을 소비하는 함수의 소비 순서(ASC/DESC, FIFO/LIFO)가 창의 정렬 방향과 반대면 캡이 없어도 과소/과대집계가 난다 — 카운트는 항상 exact-count 쿼리로, 미리보기는 별도 쿼리로 분리할 것.
- bigint PK/컬럼은 PostgREST에서 **문자열**로 온다 → 비교/계산 전 `Number()` 정규화 필수(안 하면 `"9" > "10"` 문자열 비교 오류).
- 키셋 커서는 정렬키와 tie-breaker(고유 컬럼)를 함께 물어야 중복/누락이 없다.

## 배제된 접근 (dead-end)
- **`.in()` 벌크 조회 후 앱에서 latest 계산** — 원래 방식. 1000행 캡에 잘려 오산. 이 버그를 고치려고 캡을 늘리거나(`Prefer: count`/`Range` 헤더 땜질) 페이지 크기를 키우는 것도 임시방편 — 데이터가 더 늘면 재발. 근본 해법은 "앱에서 계산하지 말고 DB에서 집계"(RPC) 또는 "캡을 전제하고 안전한 방향으로 순회"(키셋).

## 관련: [[네이버-순위-계산]] · [[요청-코얼레싱-인플라이트]] · [[project-adpang-production]] · [[레거시-코드체계-호환-필터]]
