# Postgres-함수내-정규화비교-불일치
> 같은 SQL 함수 안에서 텍스트 비교 기준이 일부는 정규화, 일부는 원문이라 조인이 조용히 깨지는 패턴.

## 무엇 / 언제 쓰나
여러 단계로 구성된 Postgres 함수(RPC)가 같은 텍스트 컬럼을 여러 곳에서 조인/비교할 때, 정규화 함수(대소문자·유니코드 폭·공백 정리 등)를 일부 비교에만 적용하고 다른 비교엔 빠뜨리면, 검증/생성 단계에서 문자열 표현이 미세하게 달라진 행들이 그 특정 조인에서만 소리 없이 탈락한다. 함수를 처음 짤 때 정규화를 넣었다가 이후 조인을 추가·복제하면서 정규화 적용을 빠뜨리는 식으로 생기기 쉽다.

## 정본 구현 (canonical)
- `turafic_update/supabase-migrations/014-normalize-candidate-phrase-join.sql` — `claim_strategy_slot_group()` 내 `keyword_pipeline_candidates` ↔ `landing_keyword_pool` JOIN 3곳(대표 슬롯 후보 CTE / 대표 query 선정 / 멤버 슬롯 수집)을 `c.query_phrase = p.query_phrase`(원문 비교)에서 `normalize_query_phrase(c.query_phrase) = normalize_query_phrase(p.query_phrase)`로 통일했다. 같은 함수의 "멤버 슬롯 수집" 단계는 013까지도 이미 `normalize_query_phrase(p.query_phrase) = v_norm_query`로 정규화 비교를 쓰고 있었는데, 정작 JOIN 조건 자체(`c.query_phrase = p.query_phrase`)는 원문 비교로 남아 있던 게 원인이었다.
- 실측(2026-08-18, 프로덕션 Supabase `cwsdvgkjptuvbdtxcejt` 직접 SQL): 기본 자격 슬롯 153개 중 실제 claim 가능한 슬롯이 6개뿐이었는데, JOIN만 정규화 비교로 바꿔 재계산하니 65개로 늘었고, 마이그레이션 014를 실제 적용한 직후 `claim_strategy_slot_group()` 실호출이 성공(슬롯 308 정상 claim)했으며 claim 가능 슬롯 수가 88개로 확인됐다(검증된 성공).

## 다른 구현 / 변형
- Supabase Management API `POST /v1/projects/{ref}/database/query`(헤더 `Authorization: Bearer <SUPABASE_ACCESSTOKEN>`, 토큰은 `sbp_...` 접두어)로 SQL을 직접 실행해서 이 버그를 실측·수정·검증까지 전부 PostgREST anon/service_role 키 없이 처리했다. 레거시 `anon`/`service_role` 키가 비활성화됐거나(`Legacy API keys are disabled` 에러), `.env`가 여러 파일(워크트리 vs 메인 저장소)에 흩어져 있어 어느 게 진짜 프로덕션 키인지 헷갈릴 때 우회 경로로 재사용 가능. 프로젝트 ref는 `supabase/.temp/project-ref`(로컬 `supabase link` 캐시 파일)에서 확인했다.

## 주의점 / 함정
- **named failure: "부분 정규화 조인 유실"** — 정규화 함수가 함수 안 한 곳에라도 쓰이고 있으면 "이 함수는 정규화를 쓴다"고 착각하기 쉬운데, 실제로는 조인 조건마다 개별적으로 정규화 여부를 확인해야 한다. 이번 케이스는 같은 함수 안에 정규화 비교(멤버 수집 쿼리)와 원문 비교(대표 슬롯/쿼리 선정 JOIN)가 공존해서 더 발견하기 어려웠다.
- 정규화가 빠진 조인은 에러를 내지 않고 매칭되는 행 수가 조용히 줄어들 뿐이라 로그·예외로는 절대 드러나지 않는다. "후보 수 대비 실제 claim 수" 같은 깔때기(funnel)를 단계별로 직접 세어봐야 발견된다 — 이번엔 기본자격→pool검증→쿨다운통과→candidates매칭 4단계로 쪼개서 153→109→17→6이라는 숫자를 실측하고 나서야 병목 지점을 특정했다.

## 배제된 접근 (dead-end)
- `normalize_query_phrase()`(소문자화+NFKC+공백정리)만 적용하면 전부 해결될 거라 가정했으나, 일부 슬롯(예: slot 393 "당일경매 5kg 양배추")은 정규화 후에도 매칭되는 후보가 없었다. `keyword_pipeline_candidates`에 그 어순 조합 자체가 아예 생성돼 있지 않은 별개 문제였다(후보 테이블엔 "5kg 당일경매 양배추" 등 다른 어순만 존재). `normalize_query_phrase`는 표현 차이(대소문자/공백/유니코드 폭)만 흡수하지 어순 차이는 해결하지 못한다 — 어순까지 흡수하려면 토큰 정렬(단어 집합 비교) 같은 별도 정규화가 필요하며, 이번 세션에서는 여기까지는 손대지 않고 미해결로 남겼다(영향은 소수 슬롯으로 제한적이라 후순위로 분류).

## 관련: [[project-turafic_update]] · [[Supabase-무오프셋-타임스탬프-로컬시간오독]]
