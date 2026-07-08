# 카카오-네이버-소셜로그인-AuthJS
> Auth.js(next-auth v5)로 카카오·네이버 OAuth를 붙이되, 키 없인 완전 비활성(플래그 게이트)이라 "머지=무해"가 보장되는 소셜 로그인 패턴. 브릿지 페이지로 기존 세션 계층에 무수정 합류.

## 무엇 / 언제 쓰나
- 한국 서비스 소셜 로그인(카카오/네이버)을 붙일 때. 특히 **백엔드/키 발급이 늦어지는 수주 프로젝트**에서 코드부터 머지해두고 키만 나중에 꽂는 구조가 필요할 때.
- Next.js 16 + next-auth `5.0.0-beta.31`부터 peerDeps `next ^16.0.0` 공식 지원 (npm view로 확인, 2026-07-08).
- 사업주(클라이언트) 계정 정책까지 포함: 운영 앱은 사업주 소유 + 개발자를 팀멤버/관리자로 초대 → 설정 변경에 사업주 로그인 불필요. 절차 문서: `BAGJO1/docs/social-login-setup.md`.

## 정본 구현 (canonical) — BAGJO1(백조오브제), 2026-07-08 머지(PR #1)
- `BAGJO1/src/lib/auth.ts:7` — NextAuth zero-config: `AUTH_KAKAO_ID/SECRET`·`AUTH_NAVER_ID/SECRET`·`AUTH_SECRET` 환경변수 자동 인식, JWT 세션(DB 도입 전), jwt 콜백에서 `token.provider` 저장.
- `BAGJO1/src/app/api/auth/[...nextauth]/route.ts:1` — `export const { GET, POST } = handlers` 3줄 라우트.
- `BAGJO1/src/lib/socialAuth.ts:8` `isSocialLoginEnabled` — `NEXT_PUBLIC_SOCIAL_LOGIN === '1'` 플래그 게이트(리터럴 참조로 빌드타임 인라인). 키·플래그 없으면 버튼이 "준비 중" 안내만 → **배포해도 기존 동작 무변화**.
- `BAGJO1/src/lib/socialAuth.ts:22` `completeSocialLogin` — OAuth 후 브릿지: Auth.js 세션을 앱 고유 `User`로 매핑, 이메일로 기존 회원 매칭/신규 등록 → 기존 화면(마이페이지 등)은 한 줄도 안 바뀜 ([[project-BAGJO1]]의 "콘센트" 파사드에 합류).
- `BAGJO1/src/lib/socialAuth.ts:44,62` — **admin 승격 차단**: 소셜 경로는 매칭/신규 모두 `role: 'user'` 강제.
- `BAGJO1/src/lib/socialAuth.ts:55` — 이메일 미제공(카카오 비즈앱 검수 전) 시 `social-<provider>-<ts>@placeholder.baekjo` 고유 플레이스홀더로 이메일 기준 중복제거 오병합 방지.
- `BAGJO1/src/app/auth/complete/page.tsx:11` — 브릿지 페이지: `useRef`로 StrictMode 이중실행 가드, setState 없이 redirect만(hydration lint 회피).
- 검증 상태: CI(tsc+build+lint) green + 프로덕션 카나리(/login 200·렌더) 확인. **OAuth 왕복은 키 발급 후 검증 예정** — Redirect URI 형식은 `/api/auth/callback/{kakao,naver}`.

## 주의점 / 함정
- **「소셜 이메일 매칭 admin 승격」** — OAuth가 돌려준 이메일(제공자 검증 여부 불명)을 기존 회원과 매칭해 레코드 전체(role 포함)를 상속하면, admin 이메일로 만든 소셜 계정이 관리자가 된다. security-reviewer가 CRITICAL로 검출 → role 강제 'user'로 차단(`socialAuth.ts:44`). 실DB 붙일 때도 이 규칙을 승계할 것.
- **「UI 플래그 ≠ 접근제어」** — `NEXT_PUBLIC_SOCIAL_LOGIN`은 버튼 노출 게이트일 뿐, `/api/auth/*`는 키가 설정된 환경에선 항상 살아있다.
- **「프리뷰 URL Redirect 불가」** — Vercel 프리뷰 URL은 매번 바뀌어 OAuth Redirect URI로 등록 불가 → 소셜 실테스트는 localhost/프로덕션 도메인 전용.
- 카카오 이메일은 **비즈 앱 전환(사업자등록번호) 후에만 필수 수집** — 이메일 없는 유저 처리(플레이스홀더)가 필수 전제. JWT-only 세션은 안정적 user id가 없어 이메일 없는 유저는 로그인마다 신규 레코드(Phase 1 한계, 실DB에서 provider id로 해소).

## 배제된 접근 (dead-end)
- **`signIn(provider, { callbackUrl })`** — beta.31 설치본 타입에서 `callbackUrl`은 @deprecated. 현행은 `redirectTo` (node_modules/next-auth/lib/client.d.ts에서 확인). 구버전 예제 복붙 시 함정.
- **OAuth 수제 구현(라우트 핸들러 직접 작성)** — next-auth의 Next 16 지원이 불확실하다는 가정에서 검토했으나, beta.31 peerDeps 확인으로 배제. CSRF/state 처리를 공짜로 얻는 쪽이 우위.
- **storage.ts에 next-auth 정적 import** — 세션 파사드(storage.ts)를 쓰는 모든 화면이 next-auth에 정적 의존하게 됨 → `logout()` 내부에서 동적 `import('next-auth/react')` fire-and-forget으로 회피 (`BAGJO1/src/lib/storage.ts:183`).

## 관련: [[웹앱-인증-세션-계층]] · [[project-BAGJO1]]
