# 키워드 파이프라인 러너 호환 MID 검증

## 핵심 결정

키워드 파이프라인의 SERP HTTP 검증 positive와 트래픽 러너의 MID 발견 성공은 같은 검증이 아니다. 최종적으로 러너가 사용할 수 있는 키워드는 `(slot_id, assignment_id, MID, query_phrase)` 조합을 noCDP 러너와 같은 검색 surface, URL 파라미터, 스크롤·캐러셀 탐색 범위로 확인한 경우에만 `landing_keyword_pool.state='active'` 및 `mid_verified=true`로 승격한다.

## 왜 필요한가

- `keyword-pipeline.mjs`는 모바일 SERP HTML의 쇼핑 캐러셀 parser로 MID를 판정한다.
- noCDP 러너는 실제 브라우저 화면에서 검색어를 검색하고, 정해진 스크롤·캐러셀 범위 안에서 `nv_mid`를 찾아 클릭한다.
- 따라서 HTTP parser에서 positive였어도 noCDP 화면에서 MID가 안 보이면 `MID_NOT_FOUND`가 된다.
- 외부 엑셀은 URL·키워드만 있고 MID·실제 rank·검증 surface가 없을 수 있다. URL 하나에 여러 MID 슬롯이 연결되면 URL 단위 positive를 모든 MID에 복사하면 안 된다.

## 운영 규칙

1. 외부 키워드는 실제 MID별 검증 증거가 없으면 pending external로 보관한다.
2. 검증 키는 반드시 `(slot_id, assignment_id, MID, query_phrase)`다.
3. 같은 `query_phrase + MID`는 한 번만 검증하고, 같은 조합을 가진 슬롯에 결과를 fanout한다. 실제 트래픽 claim은 슬롯별로 유지한다.
4. `verified_rank`는 claim eligibility와 정렬용 값이며, 러너가 그 숫자 위치를 직접 클릭한다는 뜻이 아니다.
5. `MID_NOT_FOUND` 1회는 삭제하지 않는다. pool row를 `mid_verified=false`와 cooldown으로 격리하고, 반복 실패 시 idle로 내리되 이력을 보존한다.
6. active 승격에는 query, MID, 실제 rank, engine/parser version, search surface, scan 범위, 검증 시각을 남긴다.

## 확인된 운영 사례

2026-08-21 외부 엑셀 키워드를 `verified_excel_import`로 직접 active 처리한 뒤, 김태동 슬롯의 최신 `MID_NOT_FOUND` 샘플 30건이 모두 해당 import 출처였다. `verified_rank=5`는 엑셀에 실제 순위 컬럼이 없어 claim 가능하게 넣은 sentinel 값이었고, noCDP 실행에서 같은 MID를 찾았다는 증거가 아니었다.

## 2026-08-27 현행 구현 (positive→pool 자동 브리지, 굶주림 해소)

- 문제: SRV-01~05는 `verifier-loop.mjs`만 돌아 `candidates.terminal_result=positive`만 남기고 **pool 승격이 없음**. pool을 채우는 정식 경로(`keyword-pipeline.mjs` verifyQueryTasks → `keyword_pipeline_fanout_query_task` → `landing_keyword_pool` upsert)는 SRV 예약작업에 없었고 죽어 있었음 → claimable 슬롯 ~16개로 전멸(러너 굶음).
- **러너는 verified_rank를 실제로 사용하지 않는다** (`strategy-a-nocdp-runner.ts` rank 참조 0, `claim_strategy_slot`이 러너에 반환하는 값에 rank 없음). 즉 `verified_rank>0`은 claim 게이트 proxy일 뿐(운영 규칙 4의 실증) → **sentinel rank=1**로 positive를 승격해 claim 함수(300줄) 재작성 없이 해금.
- 마이그레이션(sellermate-contracts, 커밋 0cff5f7): **051** pool identity fail-closed 트리거(active pool은 자기 assignment+mid가 현재 슬롯과 일치해야; 슬롯 변경 시 자동 archive) / **052** 현재-identity positive 1회 대량 승격(21,676행) / **053** `pg_cron promote-positives-1min` 매분 gap-fill 승격(재발방지). candidates 핫테이블 per-row 트리거는 DB CPU 포화(migration 050) 재현 위험이라 배치 cron 선택.
- ⚠️ **sentinel 위험 상속**: verifier positive(Patchright 브라우저 캐러셀)는 2026-08-21 엑셀 무증거보다 안전하나 noCDP 클릭 surface와 완전 동일은 아님 → MID_NOT_FOUND 잔존 가능. 굶는 슬롯 중 12개는 candidate 전부 negative(MID 캐러셀 실부재)로 구제 불가(상품/키워드 교체 필요).

## 2026-09-26 함정: 053 크론이 러너 강등을 1분 안에 되살림

- 증상: "검증풀 키워드인데 같은 슬롯이 하루 700타씩 0% MID_NOT_FOUND". 풀은 active/confirmed, verified_at=방금.
- 원인: `promote_current_identity_positives`는 "active 행이 없으면" 과거 positive를 upsert하며 state='active'·verified_at=now()로 덮는다 → `report_strategy_slot`의 MNF 쿼럼 cooldown이 다음 분에 무효화(slot 7350: 13:35:53 cooldown → 13:36:00 active). claim_v3는 state만 보므로 즉시 재배정. v2 quorum reject는 `probation` 전용이라 053의 `confirmed` 행엔 불발.
- 고침: adpang PR#91 `supabase-migrations/fix-promote-positives-no-resurrect.sql` — 쿨다운 중 행은 이후 새 positive가 있을 때만 재승격 + 부활행 1회 복구.
- 주의: 풀 `verified_at`은 신선도가 아니다(진짜 검증시각=`keyword_pipeline_candidates.updated_at`). pc-쇼핑 surface 검증기는 없고 pc_tonggeom 검증은 9/17 이후 0건이라, 신선도·surface 일치 게이트를 걸면 pc 슬롯이 전멸한다.
- 운영 DB 무흔적 드라이런: DO 블록에서 DDL·UPDATE 실행 후 `RAISE EXCEPTION`에 수치를 담아 강제 롤백 — `D:\Project\turafic_update\scripts\_promote-guard-dryrun.mjs`.

## 구현 앵커

- `D:\Project\zero-landing\tools\keyword-pipeline.mjs` — HTTP SERP 수집·검증 및 query dedup (정식 pool 승격 경로 verifyQueryTasks:590)
- `D:\Project\zero-landing\tools\pipeline\verifier-loop.mjs` — SRV 실행분: candidates positive/negative만 기록(pool·rank 없음, :331-345)
- `D:\Project\turafic_update\scripts\strategy-a-nocdp-runner.ts` — claim된 query와 MID를 noCDP 엔진에 전달 (verified_rank 미사용)
- `D:\Project\turafic_update\engines\pc-strategy-a-nocdp\nocdp-click-engine.ps1` — 실제 화면 탐색과 MID 발견·클릭
- `D:\Project\sellermate-contracts\migrations\051-strict-pool-identity-containment.sql` — pool identity fail-closed 트리거
- `D:\Project\sellermate-contracts\migrations\052-bulk-promote-current-identity-positives.sql` · `053-recurring-promote-positives.sql` — 대량/상시 positive→pool 승격 (sentinel rank)
- `D:\Project\turafic_update\supabase-migrations\023-mid-sibling-cooldown.sql` — MID_NOT_FOUND quarantine 규칙

## 관련

[[Postgres-함수내-정규화비교-불일치]] · [[CDP없는-OS레벨-브라우저자동화]] · [[요청-코얼레싱-인플라이트]]
