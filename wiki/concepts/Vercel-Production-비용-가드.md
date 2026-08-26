# Vercel Production 비용 가드
> 봇·에이전트·CI의 원격 요청이 서버리스 비용을 폭주시킬 때, 에이전트 규칙·실행기 fail-closed·robots·WAF·Preview 인증을 겹쳐 막는 패턴.

## 무엇 / 언제 쓰나
공개 Vercel 사이트가 AI crawler, 무설정 Playwright, 전수 스크린샷, Preview CI로 예상 밖 Edge Request·Observability·Function 비용을 만들 때 쓴다. 문서 규칙 하나가 아니라 요청 전 차단, 플랫폼 방화벽, 비용 관측을 각각 독립 방어층으로 둔다.

## 정본 구현 (canonical)
- `baekjo-obj/playwright.config.ts:14-36` — 알려진 Production 호스트를 명시하고 현재 요청의 승인 환경변수가 없으면 config 로드 단계에서 즉시 중단한다.
- `baekjo-obj/playwright.config.ts:45-86` — 브라우저 프로젝트만 localhost 서버를 자동 기동하고, 원격은 worker 1·retry 0으로 제한한다. 브라우저 없는 소스 계약 프로젝트는 서버를 띄우지 않는다.
- `baekjo-obj/playwright.config.ts:69-70,99,117,133,149` — Preview 인증은 `VERCEL_AUTOMATION_BYPASS`를 공통 브라우저 헤더로 주입해 보호를 끄지 않고 CI만 통과시킨다.
- `baekjo-obj/src/app/robots.ts:4-22` — Meta ExternalAgent/Fetcher/WebIndexer·GPTBot·Amazonbot을 차단하고, Facebook 링크 미리보기용 `facebookexternalhit`은 차단 목록에서 제외한다.
- `baekjo-obj/.github/workflows/golden-crud.yml:47-54` · `release-qa.yml:48-56` · `update-baselines.yml:80-88` — 수동 입력 URL이 Vercel Production 별칭 또는 커스텀 도메인이면 테스트 전에 실패한다.
- `baekjo-obj/scripts/layout-snapshot.mjs:11-28` · `scripts/qa-release-check.mjs:6-29` — 독립 실행 스크립트도 localhost 기본값과 동일한 Production 승인 토큰을 사용한다.
- Vercel 프로젝트 `baekjo-obj` — WAF 규칙 `Deny high-cost AI crawlers`가 Meta/GPT/Amazon User-Agent 변형을 1시간 persistent deny하고, Standard Deployment Protection이 Preview/생성 배포 URL을 보호한다. GitHub Actions 우회 시크릿은 Vercel 생성값과 동일하게 등록한다.

## 다른 구현 / 변형
- Production을 완전 비공개로 운영할 수 있으면 Vercel All Deployments 보호가 더 강하다. 공개 커머스처럼 Production 공개가 필수면 WAF + robots + 비용 한도를 조합한다.
- 링크 미리보기가 필요 없으면 `facebookexternalhit`도 WAF에서 차단할 수 있으나 Facebook·Instagram·WhatsApp 공유 카드가 깨지는 제품 영향이 있다.

## 주의점 / 함정
- **「원격 기본값 비용 증폭」** — Playwright·스크린샷 도구의 무설정 폴백이 Preview/Production이면 단순 테스트 명령이 과금 요청으로 바뀐다. 기본은 localhost, 원격은 항상 명시값이어야 한다.
- **「보호만 켜고 자동화 열쇠 없음」** — Preview 인증을 켠 직후 bypass 시크릿이 없으면 CI가 실제 앱 대신 Vercel 체크포인트를 캡처한다. 2026-08-26 실측: visual 14/14 실패·CRUD 로그인 필드 timeout. Vercel/GitHub 양쪽에 같은 시크릿을 등록하고 공통 Playwright 헤더로 배선한 뒤 visual·release-qa가 통과했다.
- **「로컬 안전 경로가 순수 테스트를 깨움」** — `isLocal`만 보고 webServer를 띄우면 browserless CI도 Supabase 없는 Next 서버를 시작해 120초 timeout 난다. 선택 project를 파싱해 브라우저 프로젝트에만 서버를 붙인다.
- User-Agent는 위조 가능하다. UA 분류는 비용 원인 추정에는 유용하지만 신원 증명은 아니며, robots는 비준수 봇의 접근 제어 수단이 아니다.

## 배제된 접근 (dead-end)
- **robots.txt만 추가** — 이미 진행 중인 폭주를 즉시 멈추지 못하고 비준수·사용자 트리거 fetcher를 강제 차단하지 못한다. WAF deny가 1차, robots는 2차다.
- **에이전트 prose 규칙만 추가** — 과거 세션의 직접 `curl`·브라우저·캡처 스크립트처럼 도구 경로가 규칙을 우회할 수 있다. 실행기 자체가 첫 요청 전 실패해야 한다.
- **Preview 보호를 다시 끄기** — CI는 즉시 편해지지만 봇에 billable Preview를 재노출한다. 자동화 bypass secret을 배선하는 것이 정답이다.
- **source-only 테스트에 dummy 원격 URL 주입** — 서버 기동은 피하지만 테스트 명령이 환경 속임수에 의존한다. project-aware webServer 선택으로 해결한다.

## 검증
- RED: 비용 안전 계약 3개 실패(원격 Preview 기본값·Production 캡처 기본값·robots 부재).
- GREEN: 로컬 robots HTTP 200, 안전 계약 5/5, source-only 561/561, Production 실행기 3종 첫 요청 전 차단.
- Preview 실증: 보호된 Preview에서 우회 헤더를 사용한 단일 cart 테스트 1/1, CI verify·visual·release-qa 통과.
- Vercel CLI read-back: Firewall enabled, custom deny 1개 active, `ssoProtection.deploymentType=prod_deployment_urls_and_all_previews`.

## 관련: [[시각-회귀-베이스라인-게이트]] · [[골든플로우-실구동-게이트]] · [[project-BAGJO1]]
