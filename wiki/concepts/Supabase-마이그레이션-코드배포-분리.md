# Supabase-마이그레이션-코드배포-분리
> Supabase DB 스키마 변경(마이그레이션)과 그 스키마를 쓰는 애플리케이션 코드는 서로 다른 배포 파이프라인이다. 하나가 끝났다고 다른 것도 끝난 게 아니다.

## 무엇 / 언제 쓰나

Vercel(또는 다른 서버리스) + Supabase 조합에서 "컬럼 추가 + 그 컬럼을 채우는 API 핸들러 수정"을
한 세트로 작업할 때. 대시보드 SQL Editor에서 마이그레이션을 실행하는 행위는 **즉시** 라이브 DB에
반영되지만, 같은 세트의 애플리케이션 코드는 **git 커밋 → push → (PR 머지) → Vercel 빌드/배포**를
거쳐야 라이브가 된다. 이 둘의 완료 시점이 다르다는 걸 잊으면 "컬럼은 생겼는데 계속 null"을
버그로 오인한다.

판별 질문: **"방금 확인한 코드 변경이 실제로 저 URL이 응답 중인 배포에 들어있는가, 아니면 로컬
워킹트리/미머지 브랜치에만 있는가?"**

## 정본 구현 (canonical)

- `adpang-production/supabase/migrations/20260804_android_task_leases_actual_engine_fields.sql`
  — `android_task_leases`에 `actual_persona`/`actual_chromium_major`/`actual_user_agent` 3개
  nullable 컬럼 추가. Supabase 대시보드에서 즉시 적용됨(REST로 컬럼 존재 확인: 200 응답, 값은 전부 null).
- `adpang-production/src/lib/android-admin-server.ts:2297-2299` (PR #51)
  — `reportTask()`의 `optionalReportFields`에 위 3개 컬럼 매핑 추가. 이 커밋이 로컬 워킹트리에만
  있고 아직 push/머지 전이었을 때, 실기기가 새 필드를 정상 전송했는데도 DB엔 계속 `null`이었다
  — **마이그레이션은 라이브인데 코드가 라이브가 아니었기 때문**.

## 다른 구현 / 변형

- 반대 방향 실패도 같은 원인이다: 코드가 새 컬럼을 참조하는데 마이그레이션을 안 돌리면 그 컬럼에
  대한 `.update()`가 "unknown column" 에러로 실패한다(같은 세션에서 `target_group_id` 컬럼을
  실제로 대시보드에서 실행하기 전까지 이 상태였다 — `adpang-production/supabase/migrations/
  20260804_naver_slot_target_group_id.sql`).

## 주의점 / 함정

- **"마이그레이션 적용함" ≠ "기능 라이브".** 사용자가 SQL을 실행했다고 보고해도, 그 세트의 코드가
  아직 `git push`/PR 머지/Vercel 재배포를 안 거쳤으면 응답은 여전히 이전 동작이다. 두 산출물을
  분리해서 각각 "적용됨" 여부를 확인해야 한다.
- **REST API로는 DDL을 못 돌린다.** Supabase의 PostgREST 엔드포인트(service-role 키 포함)는
  테이블 CRUD만 가능하고 `ALTER TABLE`/`CREATE INDEX` 같은 임의 SQL 실행 경로가 없다. Management
  API(개인 액세스 토큰, `sbp_...`)나 직접 Postgres 연결 문자열이 있어야 자동화할 수 있다 — 둘 다
  없으면 대시보드 SQL Editor 수동 실행이 유일한 길이다.
- **"컬럼이 null"을 보고 코드가 안 됐다고 성급히 결론 내리지 말 것.** 코드가 이미 배포됐는지부터
  확인(`git log -1 origin/main -- <file>` 또는 실제 응답 헤더/타임스탬프)한 다음에 데이터를 봐야
  한다.

## 배제된 접근 (dead-end)

- **Supabase Management API로 마이그레이션 자동 적용** — `SUPABASE_ACCESS_TOKEN`이 두 번의 서로
  다른 호출(`/v1/projects/{ref}/database/query`, `/v1/projects/{ref}`)에서 전부 401 Unauthorized.
  이 토큰은 이미 만료/무효였다 — 재시도 대신 사용자에게 새 토큰 요청 또는 대시보드 직접 실행으로
  전환.
- **`pg` npm 패키지로 직접 Postgres 연결** — 프로젝트 ref로 pooler 호스트(`postgres.<ref>@aws-0-
  ap-northeast-2.pooler.supabase.com:6543`)와 direct 호스트(`db.<ref>.supabase.co:5432`) 둘 다
  추측해서 시도했으나 각각 "tenant/user not found"와 DNS 해석 실패로 불통. 저장소에 실제
  `DATABASE_URL`이 기록돼 있지 않으면 호스트명을 추측으로 맞히는 건 비용 대비 낮은 성공률 —
  대시보드 SQL Editor가 더 빠르다.
- **(2026-08-06 재확인) direct 호스트는 IPv6 전용 라우트다 — 로컬 샌드박스도 GitHub Actions
  러너도 둘 다 막힌다.** DNS는 정상 해석되지만 `connect ENETUNREACH 2406:...5432`로 실패(IPv6
  라우트 없음). CI로 마이그레이션 자동적용 워크플로우(`scripts/apply-supabase-migrations.mjs`
  + `SUPABASE_DB_URL` 시크릿)까지 만들었는데 **GitHub Actions 러너에서도 같은 에러로 실패** —
  로컬 문제가 아니라 direct 호스트 자체의 구조적 제약이라는 뜻. Session Pooler(리전 추측,
  `aws-0-{us-east-1,us-east-2,us-west-1,eu-central-1,eu-west-1,eu-west-2,sa-east-1,ap-south-1,
  ap-northeast-1,ap-northeast-2}`)도 전부 "tenant/user not found"로 실패 — 이 프로젝트의
  진짜 pooler 리전을 못 찾음(대시보드에서 직접 복사한 연결 문자열이 아니면 리전 추측 자체가
  비용 대비 낮은 성공률이라는 위 교훈이 CI 환경에서도 그대로 적용됨). **다음 세션 숙제**: 대시보드
  Connection Pooling 탭에서 정확한 Session Pooler 문자열을 사용자에게 직접 받을 것 — 추측 재시도
  금지(스톱-리트라잉 원칙).

## 2026-08-17 재검증 결과

이 프로젝트의 `SUPABASE_DB_URL`은 direct IPv6 URL이었지만, GitHub Actions runner에서
`aws-1-ap-northeast-2.pooler.supabase.com:5432`로 연결하는 후보 경로가 실제로 성공했다.
`aws-0` 후보는 `tenant/user not found`였으므로 pooler 세대/샤드 prefix도 실제 동작 로그로
검증해야 한다. 그래도 일반화된 정본은 여전히 Supabase Dashboard의 Session Pooler 연결
문자열을 secret에 그대로 저장하는 방식이다.

## 검증된 성공

마이그레이션(대시보드 실행) + 코드(PR #51 머지 → Vercel 자동 재배포) 둘 다 완료된 뒤, 실기기(z1-1)
리포트에서 `actual_persona="CHROME"`, `actual_chromium_major="138"`이 DB에 정상 저장되는 것을
`android_task_leases` REST 조회로 확인함(연속 10건 이상, null 아님).

## 관련: [[검증리그-데이터형태-재현]] · [[project-adpang-production]] · [[project-naver_turafic_samsung]]
