# project-delivery-platform

> 배달 플랫폼 운영 콘솔의 실행환경·데이터 소스 분리. | 도메인: 배달 운영 SaaS | 스택: Next.js, TypeScript, Supabase | 상태: staging 연결 준비

## 핵심 자산 (다른 곳에서 주워올 것)

- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/src/lib/runtime/config.ts:1` — `APP_ENV`(`local|staging|production`)와 `DATA_SOURCE`(`seed|api`)를 파싱하고, local 이외의 seed 조합을 `RuntimeConfigurationError`로 거부하는 fail-closed 경계.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/src/data/deliveryHubSeed.ts:136` — 중앙 seed 생성 직전에 환경 가드를 호출해 실제 환경의 샘플 원장 선택을 차단.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/src/data/demoSync.ts:46` — 합성 동기화 결과 생성도 같은 가드를 사용해 seed 우회 경로를 차단.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/.env.example:1` — 로컬과 staging/production의 환경변수 구분 템플릿.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/frail-kolibri/docs/rider-approval-signup.md:1` — 코드 없는 수동 승인 가입 운영 절차, 설정 RPC 확인, staging→production rollout과 emergency pause 계약.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/frail-kolibri/src/app/api/branch/riders/applications/route.ts:28` — branch-scoped operator 신청 목록 GET과 private/no-store DTO 응답.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/frail-kolibri/src/app/api/branch/riders/applications/review/route.ts:19` — 최대 50건 sequential review contract와 item outcome/전역 중단 처리.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/frail-kolibri/src/app/branch/riders/approvals/page.tsx:549` — operator approval API 화면의 applicant/vendor 비교, version 선택, paused/public link 상태.

## 2026-09-11 엑셀 실지급액 정산 PR #12 병합·배포 메모 (origin/main 21d70ec 기준 경로)

- `src/server/settlements/importWorkbook.ts:56-57` `licenseIdentity` — 정산 엑셀 라이선스 ID = 지역 접두사 5자리 + 라이더 고유 7자리. 같은 라이더가 같은 날 두 지역 파일에 나오면 접두사만 달라지므로 동일 라이더 판정은 뒤 7자리(`:231`). 실파일 9/2 88행 중 2명이 이 경우였고 수정 전엔 전체 업로드가 `AMBIGUOUS_SOURCE_RIDER`로 막혔다. 같은 전제가 `src/server/settlements/riderResolver.ts:55-67`(매핑 external_rider_id가 licenseId의 접미사) 에 있다.
- `src/server/settlements/excelImportService.ts:36-40` — 파일 sha256 집합+기간+workflow의 fingerprint로 같은 파일 재저장은 기존 회차 ID를 반환, 재확정은 0건 적립(staging 실증).
- 운영 규칙(코드가 강제하지 않음): 주급 파일엔 라이선스 ID가 없어 매칭이 복호화 신원(실명+전화 뒤4)에만 의존 → 신원 행 없는 라이더가 있으면 그 지역 주급 저장이 막힘. 같은 주의 지역 주급 파일은 **한 번에** 업로드해야 일일 기반영액(전 지역 합) 차감이 맞는다(`docs/engineering-discipline/plans/2026-09-11-settlement-pr-handoff.md:91`).
- 검증 경로는 [[워크트리-격리-PR-staging-검증]], 마이그→병합 순서는 [[Supabase-마이그레이션-코드배포-분리]] 2026-09-11 절 참조.

## 운영 결정

- `local + seed`만 화면 개발·fixture 검증에 사용한다.
- `staging/production + api`는 Supabase API를 사용하며 seed를 선택할 수 없다.
- 브라우저에서 모드를 읽는 값은 `NEXT_PUBLIC_*`로 빌드 시 주입하고, 서버의 `APP_ENV`·`DATA_SOURCE`와 동일하게 관리한다.
- API repository/hydration이 연결되기 전 staging은 샘플 데이터를 보여주지 않고 명시적 구성 오류로 중단한다.

## 검증

- `bun test src/lib/runtime/config.test.ts` — 5개 계약 테스트 통과.
- `APP_ENV=local DATA_SOURCE=seed` — seed 허용 smoke 통과.
- `APP_ENV=staging DATA_SOURCE=api` — seed 호출 거부 smoke 통과.
- `npm run lint`, `npx tsc --noEmit`, local seed 모드 `npm run build` 통과.

## 의존 개념

- [[목데이터-수명-단일정본]]
- [[멀티테넌트-요청파이프라인]]
- [[Supabase-마이그레이션-코드배포-분리]]
