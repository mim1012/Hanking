# 상품명-동기화가-assignment-회전-검증-무한반복-churn가드

> 슬롯의 상품명(keyword_name) 동기화가 파이프라인 identity 키(keyword_assignment_id)까지 재발급하면, 상품명이 두 소스 사이에서 flip-flop하는 슬롯은 10~40분마다 identity가 바뀌어 수집기 재생성·검증 quota 리셋·pool archive가 무한 반복된다 — 파이프라인엔 "24h 내 assignment N개 이상 슬롯 스킵" churn 가드, 상류엔 "이름 변경은 identity 변경이 아님" 수정이 필요하다.

## 무엇 / 언제 쓰나

`keyword_pipeline_candidates`의 dedup 키, 검증기 quota, 051 pool archive가 모두 `assignment_id`를 identity의 일부로 쓰는 구조에서, 다른 서비스가 같은 슬롯의 assignment를 짧은 주기로 재발급하면 그 슬롯 하나가 파이프라인 자원 대부분을 잠식한다. 2026-09-16 실측: 24시간 동안 슬롯 7341의 assignment가 82회, 7333이 78회 바뀌었고(같은 기간 나머지 84개 작동중 슬롯은 전부 1회) 각각 후보 1,731건·1,361건이 쌓였다. 같은 기간 PC 검증 6시간 3,053건 중 1,194건이 슬롯 7341 하나였다. "같은 MID가 짧은 주기로 반복 재검증된다"는 로그가 보이면 이 패턴을 의심한다.

## 정본 구현 (canonical)

- `D:\Project\zero-landing\tools\pipeline\churn.mjs:11-12` — 임계값. 기본 windowMs=24시간, max=3개(env `KEYWORD_ASSIGNMENT_CHURN_MAX` / `KEYWORD_ASSIGNMENT_CHURN_WINDOW_HOURS`로 오버라이드).
- `D:\Project\zero-landing\tools\pipeline\churn.mjs:21-58` — `churnSlotIds(rows, currentAssignmentBySlot, nowMs, opts)`: 최근 windowMs 안에 관측된 assignment_id들과 "현재" assignment(막 바뀐 직후라 후보 행이 아직 없어도 감지되도록 합집합에 포함)를 슬롯별로 묶어, 서로 다른 assignment_id 수가 max 이상이면 churn 판정. rows/currentAssignmentBySlot이 기대한 타입이 아니거나 시각 파싱이 실패하면 그 값만 무시하고, 인자 자체가 유효하지 않으면 빈 Set을 반환한다(fail-open — 가드가 막지 않는 방향으로만 실패).

검증된 성공: 2026-09-16 가드 배포 후 8분간 슬롯 7341·7333의 신규 후보 생성 0건(전체 신규 3건), supervisor 로그에 `[churn-guard] 스킵 2: 7333,7341`, VM 6대 수집기가 정상 사이클을 유지했다. 단위 테스트 8건(`churn.test.mjs`) — 재실행 확인 결과 8/8 pass.

## 다른 구현 / 변형

- `D:\Project\zero-landing\tools\pipeline\collector-loop.mjs:90-122` — VM 상시 수집기의 `loadExistingCandidates`: terminal 후보 조회(select에 `slot_id,created_at` 포함)와 negative_memory 조회 결과를 `churnSlotIds`에 그대로 넘겨 `{existing, churn}`을 한 번에 반환한다. 호출부(`collectCycle`)는 반환된 churn Set만 보고 해당 슬롯의 이번 사이클 신규 후보 생성을 건너뛴다.
- `D:\Project\zero-landing\tools\keyword-pipeline.mjs:924-937` — 라이브(수동 실행) 수집기는 `loadVerifiedPhrasesBySlot`(:198-245, 내부에서 `churnSlotIds`를 직접 호출)가 돌려주는 churn Set을 슬롯 루프 안에서 받아, 조합 생성(`combosForSlot`, :938) 직전에 인라인 `continue`로 건너뛴다. 사이클 종료 시 `:967`에서 `[churn-guard] 스킵 N: id,id,...` 요약을 남긴다. 같은 가드를 "조회 결과에 얹어 함께 반환" vs "루프 중간에서 즉시 continue"로 다르게 통합한 변형이다.

## 주의점 / 함정

- **"상품명 flip-flop = identity 회전"** — 슬롯 7341은 슬롯 `keyword_name`("필터링 크림 40ml(SPF50+)")과 순위 히스토리 최신값("[듀이셀] 필터링크림 40ml 1+1 (SPF 50+)", 24시간 85행 전부 동일값)이 영구히 어긋난 상태였다. `sync-keyword-name`은 호출마다 이 불일치를 감지해(`D:\Project\adpang-production\src\app\api\sellermate\slot-naver\sync-keyword-name\route.ts:133-150` — 히스토리 최신 `keyword_name` ≠ 슬롯 `keyword_name`이면 이름을 갱신하며 `keywordRegistrationFields(new Date(), 'keyword_name_sync')`를 스프레드) 새 `keyword_assignment_id`를 발급한다(`D:\Project\adpang-production\src\lib\keyword-pipeline-cutoff.ts:5-13` — `crypto.randomUUID()` + `keyword_assignment_created_at`/`keyword_registered_at`/`keyword_process_after`를 동시에 now로 설정). 그 직후 다른 writer(히스토리→슬롯 트리거 `trg_sync_naver_history_keyword_name_to_slot`, 순위 persist RPC가 클라이언트로부터 받는 이름, 5분 주기 projection cron)가 이름을 되돌려 다음 sync 호출에서 또 재발급되는 순환이 생긴다. 판별 신호: `keyword_registration_source='keyword_name_sync'` + `keyword_assignment_created_at=keyword_registered_at=keyword_process_after`가 동일 시각값 + candidates의 assignment_id가 시간별로 계속 새 값으로 바뀜.
- **"DB만 뒤지면 못 찾는 writer"** — `pg_proc`/`pg_trigger`/`cron.job`을 전수 조사해도 assignment를 쓰는 서버측 오브젝트가 안 나오면, 원인은 PostgREST를 경유하는 애플리케이션 코드(Next.js 라우트)일 수 있다. 프로젝트 레포에서 문제 컬럼명(`keyword_assignment_created_at` 등)을 grep해야 writer가 잡힌다.
- **"피어 진단 맹신"** — 다른 세션이 지목한 RPC(prepare 계열)를 원인으로 바로 믿지 않고 본문을 직접 읽었더니 읽기 전용이었다. 함수 정의(`pg_get_functiondef`)를 직접 읽고 판단해야 한다.

## 배제된 접근 (dead-end)

- 파이프라인 identity 키를 assignment_id 대신 (slot_id, MID)로 바꾸기 — 검증 identity는 계약상 `(slot_id, assignment_id, MID, normalized_query_phrase)` 네 값이고(`D:\Project\sellermate-contracts\DATA-CONTRACT.md:5-9`), `landing_keyword_pool`의 active 상태도 pool 행의 `keyword_assignment_id`가 현재 슬롯과 일치해야만 유효하며 슬롯의 assignment/MID가 바뀌면 이전 identity의 pool 행은 archived로 전환된다(같은 문서 14-23행, 051 마이그레이션 근거) — assignment를 identity에서 빼면 이 불변조건과 051 archive 의미 자체가 깨지므로 배제.
- 검증기 quota를 pool 기준으로 유지 — 위 불변조건 때문에 assignment가 회전하는 슬롯은 identity가 바뀔 때마다 pool도 archived로 전환돼 quota 계산에 쓰는 pool이 매번 0으로 보인다. 실제로는 과잉 검증인데 "굶주림"으로 오판하게 된다.
- `started_at` NULL을 churn의 원인으로 추정 — NULL인 슬롯 76개 중 74개는 안정적이어서 상관관계가 기각됐다.

## 관련

[[negative-삭제후-수집기-재생성-재검증폭풍-negative-memory]] · [[PC검증-적립해도-함대성공률-불변-surface-blind-pool]] · [[키워드-파이프라인-러너호환-MID검증]]
