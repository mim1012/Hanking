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

- **2026-08-11 서버 5대 배포** — 110.10.98.98(01), 222.239.102.81(02), 1.234.35.146(03), 222.239.36.44(04), 58.236.58.176(05). `PC_ID=sellermate-0N`으로 슬롯 락 분리. SSH(plink) 원격 관리. 코드는 `release` 브랜치 zip 전송(`git archive` → `pscp` → `Expand-Archive`). 예약작업 `SellermateUnified` 5분 반복 트리거 + 실패 시 자동 재시작. srv05는 SSH 기본 셸이 cmd라 `powershell -NoProfile -Command`로 감싸야 함.
- **2026-08-11 수정 6건** — ①부분 로드 하한 ②플러스스토어 진입 방지 ③로그인 게이트 ④pagingIndex URL 폴백 복원 ⑤차단→재큐 ⑥차단해제+순위없음 누수 수정. 최종 성과: 30분 21건 중 20건 성공(95%), 185위까지 추출. → [[네이버-순위-계산]]
- **CDP vs puppeteer A/B** — 차이 없음. 빈 프로필 CDP 크롬은 puppeteer와 동일하게 차단됨. 로컬에서 CDP가 잘 된 건 사람이 오래 쓴 프로필 때문이지 CDP 자체의 이점이 아니었음.

- **2026-08-15 캡차 마라톤 세션** — 히스토리 저장률 0%(전량 -1) 원인 규명·수정.
  - **운영 코드가 로컬과 크게 드리프트**: `rank-check/parallel/parallel-rank-checker.ts`가 4대 서버 중 3대에서 로컬보다 ~440줄 짧은 구버전(`ensureNotBlocked` 함수 자체가 없음, 캡차 만나면 즉시 포기). git 자동동기화가 있어도 실제 반영 안 되고 있었음 — 배포 전 항상 라이브 파일 diff 확인 필요.
  - **`ReceiptCaptchaSolver.ts`는 서버에 있었지만 어디서도 호출 안 되는 죽은 코드였고, `ANTHROPIC_API_KEY`도 `.env`에 없었음** — 코드 존재와 실제 배선은 별개로 검증할 것.
  - **캡차 체이닝**: 하나 풀면 곧바로 다른 질문의 캡차가 또 뜸(예: "전화번호 앞 1번째"→풀이 성공→"가게 위치"→풀이 성공→"전화번호 앞 2번째"...). 1회 시도 후 포기하면 실패 처리됨. `ensureNotBlocked`에 최대 5라운드 재시도 루프 추가로 해결(`parallel-rank-checker.ts:45`).
  - **페이지네이션 2→3페이지 버그**: `getShoppingPageButtonRect`가 버튼을 화면 중앙으로 스크롤 시도(`absTop - innerHeight/2`) 실패 시 "재시도"가 오히려 덜 스크롤(`absTop - 200`)해서 영구 실패. `absTop - innerHeight + 20`(바닥 정렬)로 교체, 라이브로 1→6페이지 정상 진행 확인.
  - **IP 로테이션 단독으로는 해결 안 됨(위 2026-08-11 메모의 정정)**: 오늘은 로컬 KT IP, SSH SOCKS 프록시(SK브로드밴드), CoolIP 유동IP 프록시풀까지 **3개의 완전히 다른 IP**로 직접 검색 URL `goto()`를 테스트했는데 전부 동일하게 즉시 하드차단(418, "짧은 시간 내 너무 많은 요청"). → 이 하드차단은 IP 평판이 아니라 **레퍼러 없는 URL 직행 자체가 트리거**. 네이버 메인→검색창 타이핑→클릭 경유로 들어가면 하드차단은 피하고 정상 캡차 단계로만 감(같은 IP, 같은 프로필 기준). → [[캡차-차단-대응]]
  - **플레이스 체커 디스크 누수(별개 버그, 미수정)**: `deploy/prod-parallel-rank-checker.ts`의 `getWorkerProfilePath()`가 체크 1건마다 `%TEMP%\prb-whale-rank-worker-{id}-{timestamp}` 새 프로필을 만들고 `fs.rmSync` 삭제 로직이 없어 몇 시간마다 디스크(전체 ~39GB)가 다 참.
  - **히스토리 -1 저장 정책 변경**: 사유(캡차/차단/버그/진짜 순위밖) 구분 컬럼이 없어 노이즈와 실신호를 구별 못 함 → 사용자 결정으로 `save-rank-to-slot-naver.ts`에서 `isRankNotFound`면 히스토리 INSERT 자체를 스킵하도록 변경, 기존 -1 1,537행 삭제.
  - **RDP 세션 포커스가 로컬로 샌다**: 재접속 직후 등 시점에 pyautogui 입력이 로컬 PC로 감. `hostname`/`whoami` 비교로 원격인지 항상 검증(로컬 hostname=DESKTOP-L9DP2FA, 4대 서버 전부 hostname=desktop-qhdij7k로 동일 — VM 템플릿 클론이라 원격끼리도 hostname 구분 불가, whoami만 신뢰 가능).
  - **UAC 상승 앱은 비상승 프로세스의 synthetic input을 막음(UIPI)**: `ConsentPromptBehaviorAdmin=0`이면 `Start-Process -Verb RunAs`로 프롬프트 없이 우회.
  - **SFTP "Failure"(원인불명)는 디스크풀 신호일 확률 높음**: 작은 파일 성공·큰 파일만 실패 패턴으로 빠르게 isolate 가능.
  - 최종 성과: 저장 성공률 0% → 배포 직후 과도기 거쳐 75-80%로 안정화(라이브 확인).

## 의존 개념: [[네이버-순위-계산]] · [[사람행동-시뮬레이션]] · [[봇탐지-우회-스택]] · [[IP-로테이션]] · [[캡차-차단-대응]]
