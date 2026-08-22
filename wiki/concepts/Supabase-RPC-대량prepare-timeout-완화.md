# Supabase RPC 대량 prepare timeout 완화

> 대기 후보를 한 번에 task로 준비하는 PostgreSQL RPC가 statement timeout으로 중단될 때, 후보 seed 인덱스·미연결 후보 필터·작은 batch로 복구하는 패턴.

## 무엇이 문제였나

`zero-landing`의 `keyword_pipeline_prepare_query_tasks`가 `nvMid-v1` pending/retryable 후보 전체를 매 호출 스캔했다. 후보량이 커지면서 동일 호출이 HTTP 500과 PostgreSQL `57014 canceling statement due to statement timeout`으로 실패했고, collect 이후 verify 단계에 진입하지 못했다.

## 정본 구현

- `D:\Project\adpang-production\supabase-migrations\optimize-keyword-query-prepare.sql:3` — `(status, run_id, created_at, id)` partial index.
- 같은 파일 `:47-61`, `:78-92` — `keyword_pipeline_query_task_candidates`에 이미 연결된 `candidate_id`를 `NOT EXISTS`로 제외하고, 한 번에 100개만 선택한다.
- 호출부 `D:\Project\zero-landing\tools\keyword-pipeline-supervisor.mjs`는 collect 후 prepare barrier를 지나 verify를 시작한다.

## 검증 증거

- 동일 `nvMid-v1` RPC의 수정 전 결과: HTTP 500, 8.16초, `57014`.
- 수정 후 결과: HTTP 200, 1.81초, `batch_limit=100`, `created_tasks=53`, `linked_candidates=98`.
- supervisor 로그가 `query_task_prepare_finish` → `barrier_reached` → `verify_start` 순서로 재개되고 verify 로그에 실제 query 처리 행이 남았다.

## 운영 규칙

- batch를 무제한으로 키우지 말고 statement timeout 안에 끝나는 작은 단위로 반복한다.
- `created_tasks=0`은 오류가 아니다. 이미 연결된 후보가 없다는 정상 결과일 수 있으므로 HTTP 상태와 verify 진입 여부를 함께 본다.
- SQL migration 적용과 애플리케이션/Vercel 배포를 별도 게이트로 확인한다.

## 관련

[[PostgREST-1000행캡-회피]] · [[Supabase-마이그레이션-코드배포-분리]] · [[project-adpang-production]]
