# project-delivery-platform

> 배달 플랫폼 운영 콘솔의 실행환경·데이터 소스 분리. | 도메인: 배달 운영 SaaS | 스택: Next.js, TypeScript, Supabase | 상태: staging 연결 준비

## 핵심 자산 (다른 곳에서 주워올 것)

- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/src/lib/runtime/config.ts:1` — `APP_ENV`(`local|staging|production`)와 `DATA_SOURCE`(`seed|api`)를 파싱하고, local 이외의 seed 조합을 `RuntimeConfigurationError`로 거부하는 fail-closed 경계.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/src/data/deliveryHubSeed.ts:136` — 중앙 seed 생성 직전에 환경 가드를 호출해 실제 환경의 샘플 원장 선택을 차단.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/src/data/demoSync.ts:46` — 합성 동기화 결과 생성도 같은 가드를 사용해 seed 우회 경로를 차단.
- `C:/Users/PC_1M/.paseo/worktrees/1xrnbl5g/tearful-sloth/.env.example:1` — 로컬과 staging/production의 환경변수 구분 템플릿.

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
