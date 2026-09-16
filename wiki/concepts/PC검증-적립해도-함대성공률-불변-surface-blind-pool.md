# PC검증-적립해도-함대성공률-불변-surface-blind-pool

> 키워드 pool을 소비하는 러너(claim RPC)가 surface(모바일/PC)를 구분하지 않으면, PC 검증 키워드를 아무리 쌓아도 pool이 커져 희석될 뿐 함대 성공률이 안 움직인다 — stale-surface active 행 archive + 현재 identity positive 후보 재큐 + 슬롯당 floor + 검증기 quota를 surface별 후보로 계산, 을 한 트랜잭션으로 묶어야 움직인다.

## 무엇 / 언제 쓰나

검증측(PC 통검 verifier)이 positive를 적립해도 함대의 `claim_strategy_slot_v3`가 `landing_keyword_pool` active 행을 surface 무관하게 뽑는 구조에서 쓴다. 20슬롯 pool이 991→2,289로 커지며 함대 pc-tonggeom 성공률이 오히려 51%→48%로 하락했던 상황(surface-blind 희석)에서, "PC 검증을 더 돌리면 되겠지"라는 직관만으로는 안 풀리고 pool 쪽(archive+재큐+floor+quota)을 같이 고쳐야 하는 경우에 적용한다.

## 정본 구현 (canonical)

- `D:\Project\zero-landing\supabase-migrations\pick-landing-query-v2-surface-strategy.sql:65-87` — 0-active 승격에서 archived 상태 제외(`state not in ('active','archived')`) + 쿨다운 지난 행 우선 정렬. 051 봉인/demote로 archive된 행이 0-active 승격으로 되살아나는 것을 막는다.
- `D:\Project\zero-landing\supabase-migrations\pick-landing-query-v2-surface-strategy.sql:89-115` — 전략(p_strategy)→surface 매핑(`pc-tonggeom`→`pc_tonggeom` 등)으로, 현재 전략 surface로 검증된 candidates positive와 일치하는 active pool 행을 우선 정렬(fail-open — 매핑이 없으면 순수 random, 제외가 아니라 정렬이라 트래픽 무중단).
- `D:\Project\sellermate-contracts\migrations\058-purge-expired-slot-data.sql:193-215` — pool 행 삭제 시 FK-안전 순서(061 정본 선례): 대상 pool id 확정 → `keyword_pipeline_pc_dispatches` 삭제 → `pool_identity_containment_snapshot_051` 삭제 → pool 삭제.

검증된 성공: 2026-09-16 demote 적용(pc-tonggeom 19슬롯 active 2,176→103, PC-검증 78건=76%, floor 25) 후 turafic 실측 함대 pc-tonggeom 성공률 **48% → 55~56% 지속**(candidates까지 재큐해 되살아남 없음=durable). 25분 후 archived 2,074건 중 2,045건 유지, 29건만 PC-positive로 정상 재활성.

## 다른 구현 / 변형

- `D:\Project\zero-landing\tools\pipeline\verifier-loop.mjs:89-106` — 모바일 검증기의 슬롯 quota를 `landing_keyword_pool`의 active(현재 identity·MID 일치, `verified_rank>0`) 개수로 계산한다. pool에는 surface 컬럼이 없어 그대로 세므로 surface-blind quota의 실물이며, 이 문서가 다루는 희석 문제의 원인 경로 중 하나다.
- `D:\Project\zero-landing\tools\pipeline\pc-verifier-loop.mjs:103-117` — 같은 굶주림 방지 로직을 PC 검증기는 pool이 아니라 `keyword_pipeline_candidates`의 `verified_surface='pc_tonggeom'` positive 개수로 계산하도록 바꿨다(주석: "pool(모바일검증 포함)로 세면 모바일이 슬롯당 100개+ 채워놔서 PC 검증이 quota 초과로 스킵→굶음"). 같은 `SLOT_POSITIVE_QUOTA=7` 상수를 surface별로 다른 소스에서 채우는 변형.

## 주의점 / 함정

- **"surface-blind pool 희석"** — 검증측 PC positive 적립만으로는 pool이 커져 모바일 키워드 비율이 함께 올라가고, claim이 surface를 모르면 함대 지표가 오히려 나빠진다.
- **"원목록 집계 함정"** — 슬롯 원목록으로 세면 전략이 바뀐 슬롯이 누수처럼 보인다. 항상 `slot_strategy` 조인 기준으로 집계해야 한다.
- **"0-active 승격의 archived 되살림"** — 승격 단계 조건이 `state <> 'active'`이면 archive(051 봉인·demote)가 무효화된다. 반드시 `state not in ('active','archived')`로 걸러야 한다.
- **"quota를 pool로 세면 PC 검증 굶주림"** — 모바일 pool이 슬롯당 100~273개로 quota(7)를 훌쩍 넘겨 보이면, pc-verifier가 그 슬롯을 매 사이클 스킵한다.

## 배제된 접근 (dead-end)

- `pick_landing_query`에만 surface 선호를 넣고 pool 쪽은 그대로 둠 — turafic 함대는 `claim_strategy_slot_v3` 경로를 쓰고, `pick_landing_query`/`pick_landing_query_v2`는 turafic nocdp 러너의 [LOCAL] 캐너리 모드 1곳에서만 호출되므로 함대 지표에 영향이 없다(`D:\Project\zero-landing\supabase-migrations\pick-landing-query-v2-surface-strategy.sql:22` 적용 기록 — "함대 무영향 근거: turafic nocdp 러너의 pick 호출은 [LOCAL] 캐너리 모드만").
- PC positive 적립만 하고 pool을 안 건드리기 — 4분창 실측 51%→48%로 오히려 하락, positive를 아무리 쌓아도 claim 경로가 안 바뀌면 무의미함을 확인했다.
- stale-identity positive(1만4천여 건) 재큐 — 053 gap-fill cron이 identity 불일치 행은 애초에 승격하지 않아 inert이므로, 이 재큐는 실제 활성화 없는 헛부하였다.

## 관련

[[키워드-파이프라인-러너호환-MID검증]] · [[negative-삭제후-수집기-재생성-재검증폭풍-negative-memory]]
