# project-BAGJO1 (백조오브제 / baekjo-temp)

> 프리미엄 반려생활 큐레이션 + 펫슈어런스(펫보험) + B2B 케어키트 에코시스템 커머스 플랫폼("백조오브제", Quiet Luxury 톤). | 도메인: 반려동물 커머스 SaaS(수주) | 스택: Next.js 16.2.10(App Router)/React 19.2.4/TS5, Tailwind v4, framer-motion 12, recharts | 상태: **Phase 1 진행 중** — 프론트 UI+mock 데이터만 구축, 진짜 백엔드/API/DB 미완성. 재사용 가치는 "프론트 아키텍처 패턴 + 수주 배포게이트 스캐폴드"이지 런타임 백엔드 로직이 아님. 폴더명 `BAGJO1`, package `baekjo-temp`.

## 핵심 자산 (다른 곳에서 주워올 것)
- **"콘센트/outlet" 데이터 접근 파사드 (drift-proof mock↔real-API seam)** — `src/lib/storage.ts:10` `getJSON`(SSR-safe: `typeof window==='undefined'` 가드). 모든 화면은 `getOrders`/`registerUser` 같은 named 함수로만 읽고 쓰고, 컴포넌트에서 `fetch`/`localStorage` 직접 호출 금지. 함수 시그니처(콘센트 모양)를 고정한 채 내부만 mock→API로 교체하면 프론트는 한 줄도 안 바뀐다 — 프론트/백엔드 병렬 작업의 drift를 구조적으로 차단. 이 프로젝트 제1원칙(AGENTS.md §4).
- **규칙기반 가중치 추천 엔진 (설문→상품/보험/키트)** — `src/data/survey.ts:129` `getSurveyResult` — 다중선택 답을 선언적 `SurveyResultRule[]`에 매칭해 가중 점수 합산(concern +3, ageGroup +2 등) + default fallback. Golden Flow #1(1분 맞춤 진단) 구동.
- **hydration 가드 `useMounted`** — `src/lib/useMounted.ts:7` — `useSyncExternalStore(subscribe, ()=>true, ()=>false)`로 서버=false·마운트후=true. `useEffect(setState)` 하이드레이션 플래시 패턴 대체, localStorage 의존 화면 게이트.
- **크로스탭 이벤트 동기 장바구니** — `src/lib/cart.ts:25` — 모든 변경 후 `window.dispatchEvent(new Event('cart-updated'))`로 헤더 배지 라이브 갱신. 옵션 병합 + 5만원 이상 무료배송 계산.
- **설정주도 관리자 CRUD 테이블 `AdminResourcePage`** — `src/components/admin/AdminResourcePage.tsx` — `columns`/`rows`/`filters`/`createFields` 스펙 하나로 임의의 admin 목록 화면(검색·필터칩·스탯타일·상태배지·생성 드로어·빈상태) 렌더. 빠른 admin 콘솔에 재사용.
- **수주 골든플로우 배포게이트 스캐폴드** — `AGENTS.md` §7 골든플로우 7종 표 + `tests/golden/*.spec.ts`(현재 `test.fixme()` 스텁) + `.github/workflows/ci.yml` typecheck+build+lint 게이트. 사용자의 massage 환불 사고 계기 contract-project-gates 규칙을 그대로 구현한 복제용 스캐폴드.

## 주의점 / 함정
- **잠재 drift 버그** — `src/types/index.ts`의 `InsuranceApplication`이 canonical(`name`/`breed`)과 레거시형(`ownerName?`/`petBreed?`) 필드를 동시에 들고 있고, admin 화면(`src/app/admin/insurance/page.tsx:45`)은 레거시 필드를 읽는다 → 폼이 `name`으로 쓰면 admin이 못 읽는 사일런트 shape drift 위험. [[레거시-코드체계-호환-필터]]가 경고하는 패턴.
- 골든플로우 Playwright 스펙 7종이 전부 `test.fixme()` 스텁이고 root `playwright.config.ts` 부재 → 게이트가 아직 실효 없음(화면 완성 시 활성화 예정).
- 데이터는 전부 mock + localStorage. `src/app/api/**`는 "생성 예정". 백엔드 자산 없음.

## 의존 개념: [[웹앱-인증-세션-계층]] (localStorage 세션+role RBAC, `storage.ts:133`~) · [[레거시-코드체계-호환-필터]]
