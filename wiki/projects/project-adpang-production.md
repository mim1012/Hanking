# project-adpang-production

> 단일 Next.js 15 코드베이스로 4개 테넌트(AddPangPang/Sellermate/Glossy/Quickslot)를 서빙하는 쿠팡·네이버 순위추적 + 슬롯관리 + 트래픽 SaaS. | 도메인: 커머스 순위 SaaS | 스택: Next.js15/React19/TS, Supabase, patchright, Vercel | 상태: 활성 (오늘도 작업)

## 핵심 자산 (다른 곳에서 주워올 것)
- `middleware.ts` — host 기반 멀티테넌트 라우팅 + HTML 리라이트 (canonical). → [[멀티테넌트-요청파이프라인]]
- `src/lib/naver-searchad-api.ts:34` — 네이버 검색광고 HMAC 서명 (canonical). → [[네이버-공식API-서명]]
- `src/lib/security/` — cors/csrf/headers/ssrf(`assertUrlAllowed`)/validation 중앙 보안 레이어.
- 슬롯 생명주기 — `slots↔slot_status↔keywords` 3단계(Create→Assign→Delete, 날짜 보존 재사용), `src/lib/slot-type-utils.ts`.
- `src/lib/glossy-settlement.ts`, `partner-reward-ledger-display.ts` — 대리점 정산/리워드. → [[정산-포인트-원장]]
- `vercel.json` crons(20+) — HTTP 엔드포인트 호출식 스케줄러 패턴.

## 주의점
- `.env.local`(service_role 키)이 폴더에 존재. `next.config.js`가 빌드시 타입/린트 체크 끔 → 그린 빌드 ≠ 타입세이프.
- `adpang-production-security-quality`는 동일 코드베이스의 보안/품질 리뷰용 클론(worktree).

## 의존 개념: [[멀티테넌트-요청파이프라인]] · [[네이버-공식API-서명]] · [[정산-포인트-원장]] · [[웹앱-인증-세션-계층]] · [[네이버-순위-계산]]
