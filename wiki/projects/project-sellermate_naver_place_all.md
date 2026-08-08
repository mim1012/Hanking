# project-sellermate_naver_place_all

> 단일 PC에서 4개 플랫폼(쇼핑/플레이스/쿠팡 순위 + 트래픽)을 순차 처리하는 모노레포. | 도메인: 네이버/쿠팡 순위·트래픽 | 스택: TS(tsx), Patchright, Supabase | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `place-check/check-place-rank-core.ts:675` — 플레이스 순위 로직 (canonical). → [[네이버-순위-계산]]
- `place-check/utils/humanBehavior.ts` — 사람행동 시뮬 (canonical). → [[사람행동-시뮬레이션]]
- `rank-check/extractors/` — MID 추출 팩토리 (canonical). → [[네이버-순위-계산]]
- `rank-check/CLAUDE.md` — 쿠팡 Akamai 우회(로그인 세션 + 워밍업). → [[봇탐지-우회-스택]]
- `REPORT-프로그램-구동-로직.md:35-71` — PC 단위 원자적 슬롯 락 패턴.
- `rank-check/parallel/parallel-rank-checker.ts:220,404,469` — 쇼핑탭 SPA 페이지 전환(스크롤→PointerEvent dispatch→3중 검증). → [[네이버-순위-계산]]
- `rank-check/parallel/parallel-rank-checker.ts:36` — `isTransientBrowserError`: CDP 일시 장애(session/target closed, ENOSPC 등) 판별 후 재큐. 영구 실패와 구분해 데이터 오염 방지.
- `rank-check/utils/recover-stale-keyword-locks.ts` — 죽은 PC가 잡은 `assigned_to` 락 회수. 큐 방식 분산 워커의 필수 짝. → [[project-adpang-production]] 슬롯 락과 같은 계열.

## 세션 메모
- **2026-08-08 병합** — 텔레그램 zip(서버 08_07본)과 로컬 `feat/place-exe-fix`가 갈라져 있었다. zip=순위 로직 최신, 로컬=exe 패키징 최신. zip 15파일을 정본으로 받고 로컬 3개(browser-eval 인라인 주입, git-sync `GIT_REMOTE`, `SKIP_SUPABASE_CHECK`)만 지켰다. 커밋 `a096874`.
- **차단 시 삭제 금지** — `rank-check/batch/check-batch-keywords.ts:52` `MAX_BLOCK_RETRIES`(기본 2). 구버전은 캡차 한 번에 `rank=-1`을 히스토리에 박고 큐에서 삭제해 그날 데이터를 오염시켰다.
- **캡차의 실제 구멍은 코드가 아니라 IP 로테이션** — 예방 코드(`isBlocked`, `BLOCK_THRESHOLD=3`)는 두 갈래가 동일했다. ADB 테더링 기기가 없으면 `rotateIP()`가 `스킵(disabled 또는 기기 없음)`으로 빠져 3연속 차단에도 IP가 안 바뀐다.
- **줄바꿈 함정** — zip은 CRLF, 로컬은 LF라 `diff`/`git diff`가 전 파일 재작성으로 보인다. 비교·커밋 전에 LF 정규화할 것(정규화 후 `parallel-rank-checker.ts` diff 2001줄 → 473줄).
- **노출된 시크릿** — `rank-check/batch/check-ab-ranks.ts:10` 외 3개 레거시 파일에 Supabase service_role 키가 하드코딩돼 이미 커밋돼 있다. 텔레그램 zip의 `launcher/embedded-install-env.gen.ts`와 동일 키. 회전 필요(미조치).

## 의존 개념: [[네이버-순위-계산]] · [[사람행동-시뮬레이션]] · [[봇탐지-우회-스택]] · [[IP-로테이션]]
