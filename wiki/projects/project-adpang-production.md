# project-adpang-production

> 단일 Next.js 15 코드베이스로 4개 테넌트(AddPangPang/Sellermate/Glossy/Quickslot)를 서빙하는 쿠팡·네이버 순위추적 + 슬롯관리 + 트래픽 SaaS. | 도메인: 커머스 순위 SaaS | 스택: Next.js15/React19/TS, Supabase, patchright, Vercel | 상태: 활성 (오늘도 작업)

## 핵심 자산 (다른 곳에서 주워올 것)
- `middleware.ts` — host 기반 멀티테넌트 라우팅 + HTML 리라이트 (canonical). → [[멀티테넌트-요청파이프라인]]
- `src/lib/naver-searchad-api.ts:34` — 네이버 검색광고 HMAC 서명 (canonical). → [[네이버-공식API-서명]]
- `src/lib/security/` — cors/csrf/headers/ssrf(`assertUrlAllowed`)/validation 중앙 보안 레이어.
- 슬롯 생명주기 — `slots↔slot_status↔keywords` 3단계(Create→Assign→Delete, 날짜 보존 재사용), `src/lib/slot-type-utils.ts`.
- `src/lib/glossy-settlement.ts`, `partner-reward-ledger-display.ts` — 대리점 정산/리워드. → [[정산-포인트-원장]]
- `vercel.json` crons(20+) — HTTP 엔드포인트 호출식 스케줄러 패턴.

### sellermate 신규 자산 (2026-07-07)
- **PostgREST 1000행 캡 회피 — 2가지 상보 해법** (canonical). 한 근원 버그(`.in()` 벌크 조회가 PostgREST 기본 1000행 캡에 잘려 21만 행 히스토리 중 일부만 보고 "최신 순위"를 오산 → 목록↔모달 순위 불일치)를 두 방식으로 해결. → [[PostgREST-1000행캡-회피]] · [[네이버-순위-계산]]
  - **집계(aggregate)** — `supabase/migrations/20260707_sellermate_naver_slot_rank_summary.sql:18` `sellermate_naver_slot_rank_summary(p_slot_ids)` RPC: 슬롯별 최신/시작 순위 + 최신 keyword/trade_name을 서버측 `DISTINCT ON`(`:37`)으로 슬롯당 1행만 반환. `current_rank>0` 부분 복합 인덱스(`:13`)로 뒷받침. 호출부 `src/app/api/sellermate/slot-naver/route.ts:337`(bigint 문자열 `Number()` 정규화 `:358`).
  - **순회(iterate)** — `src/lib/experiment-log-paging.ts:41` `buildSeekOrFilter` — 키셋(seek) 페이지네이션 순수 함수(DB 호출 없어 vitest 가능). `(created_at, idColumn) DESC + .lt`로 일부러 스캔해 캡에 걸리면 최신이 아니라 "가장 오래된" 구간이 잘리게 함(`:37` 주석). `idColumn` 인자로 실험로그(`slot_id`)·순위히스토리(`slot_status_id`) 양쪽 재사용. truncated(캡) vs incomplete(오류)를 `isCapTruncated`(`:53`)로 구분.
- **AI 위저드 ↔ 봇 큐 baseline-freshness 폴링** — `src/app/api/sellermate/shopping-rank/check/route.ts` — 네이버 쇼핑은 서버 IP 직접조회가 WAF(418)로 막혀, 외부 안드로이드 봇 기기군(`naver_turafic`)이 큐를 소비해 실측 기록. POST(enqueue)는 직전값을 `baselineAt`으로 반환하고 항상 `status:'pending'` 시작, 큐에 미처리 항목 없을 때만 insert(중복 제거). GET(poll)은 `since`(baseline) 이후 생긴 기록만 `isFresh`로 인정(`:259`) → **매 분석마다 새 측정 강제**. 클라 폴링 `src/app/sellermate/naver-ai/NaverAiWizardContent.tsx:215`(3s×30, `since=baselineAt`). 큐 유무 체크 dedup은 in-memory가 아닌 DB 큐 방식의 코얼레싱 변형. → [[요청-코얼레싱-인플라이트]] · [[네이버-순위-계산]]
- **append-only 로그 retention — pg_cron ctid 배치 삭제** — `supabase-migrations/add-strategy-experiment-log-retention.sql`(⚠️ 현재 체크아웃 브랜치엔 없음 — git 커밋 `3791989`에 존재, `git -C D:\Project\adpang-production show 3791989 -- supabase-migrations/add-strategy-experiment-log-retention.sql`로 열람) `purge_strategy_experiment_log` — `ctid IN (…LIMIT 5000)` 배치 루프(`:40`)로 큰 테이블 락 최소화, 90일 초과 행을 pg_cron 일일 삭제(`:62`), unschedule 후 재등록으로 idempotent(`:59`). retention 창(90일) > 읽기 화면 창(30일)이라 화면 무영향. 무한 적재 감사로그 정리에 재사용.
- `src/app/api/sellermate/naver-shopping-ad/route.ts:65` — 네이버 쇼핑 상품 URL 판별에 `brand.naver.com`(브랜드스토어) 허용 추가. 동일 규칙이 위저드·구모달에 하드코딩 중복 → 공용 유틸 추출 후보. → [[레거시-코드체계-호환-필터]]

## 주의점
- `.env.local`(service_role 키)이 폴더에 존재. `next.config.js`가 빌드시 타입/린트 체크 끔 → 그린 빌드 ≠ 타입세이프.
- `adpang-production-security-quality`는 동일 코드베이스의 보안/품질 리뷰용 클론(worktree).
- **`.env.local`의 `DATABASE_URL`이 로컬엔 빈 값이다** — `SUPABASE_URL`/`SUPABASE_SERVICE_ROLE_KEY`는 실값이 있는데 `DATABASE_URL=`만 13자 빈 줄. `VERCEL_OIDC_TOKEN`도 같이 있는 걸 보면 Vercel 배포 환경에서만 주입되는 값으로 추정. 로컬에서 `pg` 클라이언트로 DDL(트리거/함수 변경 등) 실행하려던 시도가 "SASL: client password must be a string"로 실패 — dotenv 파싱 문제로 오진하고 `require`/`import`/직접 정규식 파싱 3가지를 다 시도했으나 전부 실패(원인은 파싱이 아니라 값 자체가 없었던 것). DDL은 Supabase 대시보드 SQL Editor에서 수동 실행해야 한다.
- **리워드A/B(`ad_type=reward_a/reward_b`) 슬롯이 트래픽 큐에서 DB 트리거로 차단돼 있었다(named failure)** — `supabase-migrations/block-reward-ab-from-sellermate-traffic-navershopping.sql`의 `trg_block_reward_ab_sellermate_traffic`(BEFORE INSERT)이 리워드 슬롯의 `sellermate_traffic_navershopping` INSERT를 `RETURN NULL`로 조용히 무시. 크레딧은 정상 차감, 랭킹체크 큐엔 들어가는데 **트래픽(클릭) 큐에만 안 들어가** 리워드로 결제한 셀러가 트래픽을 못 받던 상태. `supabase-migrations/allow-reward-ab-into-sellermate-traffic-navershopping.sql`로 트리거+함수 제거해 해결 — 실측 검증: `slot_id=40`(reward_a)으로 테스트 INSERT → HTTP 201 성공(적용 전엔 트리거가 조용히 버려서 응답은 있어도 실제 행이 안 생김) → 테스트 행 즉시 삭제.
- **`sellermate_slot_naver.daily_target`(일일 트래픽 한도)이 큐 적재(`scheduler/traffic/route.ts`)·Android 클레임(`claim_android_naver_task` RPC)·PC러너 어디에서도 참조되지 않던 완전 미구현 상태** — 스키마·설정 UI(`NaverShoppingAdModal.tsx:286-303`)까지는 있는데 enforcement 로직이 통째로 없었음. `success_count`/`fail_count`는 매일 KST 00:00 크론(`scheduler/naver-success-fail-reset/route.ts`)으로 리셋만 될 뿐 한도 비교엔 안 쓰임. 한도 기준은 성공수만 카운트(실패/재시도 제외)로 결정 → `claim_android_naver_task` RPC의 candidate 조건에 `daily_target`체크 추가(`naver_turafic_samsung/supabase/migrations/20260816000000_claim_android_naver_task_daily_target.sql`), PC 쪽은 [[project-turafic_update]] 참고. **PC는 큐 테이블을 안 거치고 `sellermate_slot_naver`를 직접 읽으므로, 스케줄러 단계에서만 막으면 PC는 여전히 안 막힌다** — 두 소비 경로 각각에 개별 체크가 필요한 구조.

## 의존 개념: [[멀티테넌트-요청파이프라인]] · [[네이버-공식API-서명]] · [[정산-포인트-원장]] · [[웹앱-인증-세션-계층]] · [[네이버-순위-계산]]
