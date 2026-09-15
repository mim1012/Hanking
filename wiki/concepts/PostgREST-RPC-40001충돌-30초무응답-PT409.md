# PostgREST RPC 40001충돌 30초무응답 PT409

> Supabase RPC 충돌이 SQL 직접 호출은 1초인데 PostgREST(supabase-js) 경유는 30초+ 타임아웃(무응답) — plpgsql `raise exception ... using errcode = '40001'`(serialization_failure)은 PostgREST가 내부적으로 투명 재시도하다가 결국 멈춰버리는 반면, 애플리케이션 레벨 충돌(revision conflict·already-managed·source mismatch)은 PostgREST의 커스텀 HTTP-status SQLSTATE `PT409`(`using errcode = 'PT409'`)로 raise해야 즉시 409로 응답한다.

## 무엇 / 언제 쓰나
Supabase RPC(plpgsql 함수) 안에서 "동시성으로 인한 실제 트랜잭션 재시도가 필요한 충돌"이 아니라 "낙관적 락 revision 불일치·이미 처리됨·source 불일치" 같은 **애플리케이션 레벨 충돌**을 클라이언트에 알려야 할 때. `raise exception ... using errcode = '40001'`을 쓰면 SQL을 DB에 직접 붙어 호출할 땐 즉시 에러가 올라오지만(1초 미만), **PostgREST(`/rest/v1/rpc/...`, supabase-js 경유)로 호출하면 40001 = `serialization_failure`로 인식되어 PostgREST가 내부적으로 트랜잭션을 재시도**한다. 애플리케이션 충돌은 재시도해도 조건이 그대로라 계속 같은 에러가 나고, 이 재시도 루프가 클라이언트 쪽에는 응답 없이 30초 넘게 걸리는 "무응답 타임아웃"으로 관측된다. 낙관적 락(revision 체크)·멱등 가드(already-managed)·소스 일치 가드가 있는 RPC라면 이 충돌 코드를 SQLSTATE `PT409`로 raise하고, 클라이언트에서 `error.code === 'PT409'`를 HTTP 409로 매핑해 처리한다. 기존에 `40001`로 이미 배포된 함수는 `create or replace`로 추가 마이그레이션을 얹어 고친다(과거 마이그레이션 파일 수정 금지 원칙 유지).

## 정본 구현 (canonical)
- `BAGJO1/supabase/migrations/0165_cms_conflict_sqlstate.sql:1-3` — 헤더 주석: "PostgREST retries SQLSTATE 40001 (serialization_failure) transparently, so calls made through /rest/v1/rpc/... (supabase-js) hang 30s+ on conflict instead of returning; PT409 is PostgREST's non-retried custom-HTTP-status class, so these application-level conflicts now use it instead." `:33` `raise exception 'cms-revision-conflict' using errcode = 'PT409';` — `publish_cms_page` RPC의 revision 불일치 가드를 40001에서 PT409로 재발행(`create or replace function`, 기존 0164 함수를 대체).
- `BAGJO1/src/lib/cms/repo.ts:84` — `isRevisionConflict()` 매핑: `return code === 'PT409' || code === '40001' || (typeof message === 'string' && message.includes('cms-revision-conflict'));` — PT409를 1차 판정으로, 40001은 방어적 폴백으로 유지.
- `BAGJO1/src/lib/cms/importPublished.ts:54` — `if (error.code === 'PT409' || error.code === '40001') throw new AuditImportConflictError();` — audit import RPC 충돌도 동일 매핑.
- `BAGJO1/tests/admin/cms-foundation-contract.spec.ts:107` — `test('0165 replaces 40001 conflict raises with PT409 and keeps grants', ...)` — 마이그레이션 SQL에 `errcode = 'PT409'`가 6회 이상 등장하고 `'40001'` 문자열은 더 이상 없음을 계약으로 고정.

## 다른 구현 / 변형
없음(단일 프로젝트 canonical). 다른 프로젝트에서 동일 RPC 충돌 패턴이 나오면 이 함수 시그니처를 그대로 포팅.

## 주의점 / 함정
- **"SQL 직접은 1초, REST는 30초+ 무응답"** — 증상만 보면 네트워크·커넥션풀·RLS 문제로 오진하기 쉽다. 실제 원인은 SQLSTATE 클래스 자체: `40001`은 Postgres 표준에서 `serialization_failure`로 분류되고, PostgREST는 이 클래스를 "재시도하면 성공할 수 있는 에러"로 취급해 내부적으로 재시도한다. 애플리케이션 레벨 충돌은 재시도해도 조건이 그대로라 재시도가 반복되다가 클라이언트 타임아웃(30초)에 걸려서야 실패로 보인다 — 에러 자체는 로그에 안 남고 "그냥 안 끝남"으로만 보이는 게 함정.
- 기존에 배포된 함수의 errcode만 바꾸려고 마이그레이션 파일을 직접 수정하면 안 된다 — 이미 적용된 마이그레이션은 불변이어야 하므로 `create or replace function`을 담은 **새 추가 마이그레이션**으로 재발행한다([[Supabase-마이그레이션-코드배포-분리]] 원칙과 동일 계열).
- 클라이언트 매핑에서 `40001` 분기를 완전히 제거하지 않고 방어적 폴백으로 남겨둔다 — 다른 RPC나 실제 serialization_failure(진짜 동시성 충돌)가 여전히 40001로 올 수 있으므로.

## 배제된 접근 (dead-end)
- **PostgREST/Supabase 클라이언트 타임아웃 값을 늘려서 30초 무응답을 "정상 지연"으로 흡수** — 근본 원인(불필요한 재시도 루프)을 그대로 두고 대기시간만 늘리는 임시방편. 애플리케이션 충돌은 재시도해도 절대 성공하지 않으므로 타임아웃을 늘려도 결국 실패하며, 사용자 체감 지연만 악화된다. 실제 해법은 SQLSTATE 클래스를 애초에 재시도 대상(40001)에서 비재시도 대상(PT409)으로 바꾸는 것.

## 검증 (2026-09-15, BAGJO1 staging)
- 수정 전: 직접 SQL 호출 835ms 대 REST(PostgREST) 경유 30,014ms에서 abort.
- 수정 후: REST 경유 999ms, HTTP 409 `{code:"PT409", message:"cms-revision-conflict"}` 즉시 응답.

## 관련: [[project-BAGJO1]] · [[PostgREST-1000행캡-회피]] · [[Supabase-마이그레이션-코드배포-분리]]
