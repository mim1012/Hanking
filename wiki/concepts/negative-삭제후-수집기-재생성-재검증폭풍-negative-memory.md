# negative-삭제후-수집기-재생성-재검증폭풍-negative-memory

> negative 후보를 DB 트리거가 즉시 삭제하면 수집기가 같은 (슬롯·assignment·MID·정규화 검색어) 조합을 재생성해 같은 MID를 짧은 주기로 반복 재검증하는 폭풍이 남 — 삭제 전에 소형 memory 표(복합 PK)에 upsert하고, 수집기가 그 표를 읽어 재생성을 막는다.

## 무엇 / 언제 쓰나

verifier가 negative 판정된 `keyword_pipeline_candidates` 행을 보존(UPDATE)하지 않고 DELETE하는 정책(2026-09-16 옵션A, DB의 auto-delete 트리거와 정합)으로 바뀐 뒤, 상시 수집기(collect phase)가 pending 생성 시 "이미 검증된 조합"을 판단할 근거를 잃어 같은 조합을 매 사이클 다시 만들어내는 상황에서 쓴다. 같은 MID가 짧은 주기로 반복 재검증되는 로그가 보이면 이 패턴을 의심한다.

## 정본 구현 (canonical)

- `D:\Project\zero-landing\tools\pipeline\verifier-loop.mjs:402-430` — positive는 `keyword_pipeline_candidates`에 `verified_surface='m_tonggeom'`로 기록(UPDATE 유지). negative는 DELETE 전에 `keyword_pipeline_negative_memory`로 `sbUpsertIgnore`(on_conflict 4컬럼: `slot_id,assignment_id,mid_norm,query_norm`) 먼저 남긴 뒤 candidate를 DELETE한다.
- `D:\Project\zero-landing\tools\pipeline\collector-loop.mjs:100-113` — VM 상시 수집기(loadExistingCandidates)의 negative_memory 조회. fail-open(테이블 조회 실패해도 사이클 전체를 죽이지 않고 1회만 경고).
- `D:\Project\zero-landing\tools\pipeline\collector-loop.mjs:186` — 조합 대조 키 `${slot.keyword_assignment_id}:${normMid(slot.mid)}:${normalizeQueryPhrase(combo.q)}` — negative_memory의 mid_norm·query_norm과 동일한 정규화로 대조한다.
- `D:\Project\zero-landing\tools\keyword-pipeline.mjs:234-271` — 라이브(수동 실행) 수집기 쪽의 동일 가드. `order=slot_id.asc,assignment_id.asc,query_norm.asc&limit=&offset=` 페이지네이션(복합 PK라 id 기반 keyset 불가)에 안전 상한 50페이지를 두고, 상한 도달 시 경고만 남기고 계속 진행한다(fail-open).
- `D:\Project\zero-landing\tools\pipeline\db.mjs:32-44` — `sbFetchAll`의 offset 페이지네이션 구현(`SB_PAGE=1000`). 단발 `sbGet`은 PostgREST 기본 최대 응답 행수(1000행)에서 조용히 잘리므로, 계속 누적되는 negative_memory처럼 언젠가 1000행을 넘는 테이블은 반드시 이 함수로 조회해야 한다.
- `D:\Project\zero-landing\supabase-migrations\add-negative-memory.sql` — `keyword_pipeline_negative_memory` 테이블 정의(파일 전체, 줄번호 없음). PK `(slot_id, assignment_id, mid_norm, query_norm)` + `verified_surface`, `verified_at timestamptz default now()`.

검증된 성공: 2026-09-16 배포 후 negative_memory 0→281행(1시간 내), "memory 행 생성 이후 같은 조합 후보가 다시 생성된" 위반은 수집기·검증기 같은 사이클 경합 수준의 **1건**만 관측됐다. 반복 검증되던 MID 86230387310의 pending이 0으로 소진, 073 기준 negatives remembered 누적을 확인했다.

## 다른 구현 / 변형

- `D:\Project\zero-landing\tools\keyword-pipeline.mjs:245-246` — 라이브 수집기는 `NEGATIVE_MEMORY_PAGE_LIMIT=50`으로 명시적 페이지 상한을 두고 도달 시 경고를 남긴다.
- `D:\Project\zero-landing\tools\pipeline\collector-loop.mjs:105-106` — VM 상시 수집기는 같은 조회를 `sbFetchAll`(위 db.mjs:32-44, 자체 종료 조건이 있는 offset 루프) 하나로 위임해 별도 페이지 상한이 없다. 같은 가드를 두 경로에서 서로 다른 안전장치(명시적 50페이지 캡 vs sbFetchAll 위임)로 구현한 변형이다.

## 주의점 / 함정

- **"auto-delete 트리거 + 재생성 = 재검증 폭풍"** — 같은 MID가 `tried=3/3 +0 -3` 형태로 짧은 주기로 반복되는 로그가 이 문제의 신호다.
- **"단발 sbGet 1000행 절단"** — PostgREST가 조용히 잘라 가드가 부분적으로만 작동한다. negative_memory는 계속 누적되는 테이블이라 운영 기간이 길어지면 반드시 1000행을 넘긴다.
- **"배포 순서 역전"** — verifier(삭제 코드)를 수집기(memory 읽기)보다 먼저 재시작하면, 그 사이 라이브 수집기가 아직 못 읽은 negative를 재생성한다. 순서는 테이블 생성 → 수집기 코드 반영·재spawn 확인 → verifier 재시작.
- **"정규화 비대칭"** — memory는 정규화된 키로 저장되는데 대조 쪽이 원문 그대로면 누락된다. `normalizeQueryPhrase`(공백 정규화·trim)를 양쪽에서 같은 함수로 써야 한다.
- **"collector-loop.mjs가 같은 커밋에서 못 고쳐진 채 남은 import"** — `D:\Project\zero-landing\tools\pipeline\collector-loop.mjs:7`은 이 negative_memory 가드를 추가한 바로 그 커밋(6cbe742, 2026-09-16)에서도 여전히 `../keyword-pipeline-helpers.mjs`를 직접 import한다(git show 6cbe742로 확인 — 이 파일은 같은 커밋에서 49줄 바뀌었지만 이 import 줄은 그대로). 같은 커밋이 verifier-loop.mjs·pc-verifier-loop.mjs는 `./normalize.mjs` 직접 import로 새로 만들었는데(아래 dead-end 참고) collector만 남았다 — `tools/pipeline/normalize.mjs:1-5` 주석이 서술하는 "VM 배포 디렉터리는 tools/pipeline/만 포함" 전제가 맞다면 collector-loop.mjs는 VM에서 이 import가 깨질 수 있는 미해결 위험이다(이 세션 기준 미수정 확인, 실제 VM 배포 구성으로 재현까지는 못 함).

## 배제된 접근 (dead-end)

- negative를 UPDATE로 남겨두기(터미널 상태 보존) — 터미널 테이블이 무한 팽창하고, 이미 DB 트리거가 UPDATE 시 지우도록 되어 있어 애초에 의미가 없다.
- `tools/pipeline/` 안의 워커가 상위 경로 `../keyword-pipeline-helpers.mjs`를 직접 import — VM 배포 디렉터리가 `tools/pipeline/`만 포함한다는 전제(정본 구현의 normalize.mjs 주석)가 맞다면 import가 깨져 크래시루프 위험이 있다. 이 위험을 피하려고 이 커밋(6cbe742)은 `D:\Project\zero-landing\tools\pipeline\normalize.mjs:1-8`에 정규화 함수 단일 정의를 새로 만들어 verifier-loop.mjs(:18)·pc-verifier-loop.mjs(:23)가 거기서 직접 import하게 했고, 기존 `tools/keyword-pipeline-helpers.mjs:7-12`는 그 파일을 import한 뒤 re-export만 하도록 바꿨다. 다만 collector-loop.mjs(:7)는 같은 커밋에서도 이 회피를 적용받지 못했다(위 주의점 참고) — 즉 이 접근은 "전면 배제"가 아니라 "2/3 파일에서만 배제됨".

## 관련

[[PC검증-적립해도-함대성공률-불변-surface-blind-pool]] · [[키워드-파이프라인-러너호환-MID검증]]
