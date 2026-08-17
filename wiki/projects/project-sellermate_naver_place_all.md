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
- `rank-check/utils/resolve-naver-slot-members.ts:33-76,113-173` — 키워드 trim/lowercase + URL query/fragment/trailing-slash 정규화로 canonical `keyword+url` 키를 만들고, 활성 슬롯을 1000행 페이지 단위로 복원한다. `uniqueNaverSlotMembers`/`missingNaverSlotIds`가 동일 `slot_id` 중복 저장과 coverage 누락을 제거한다.
- `rank-check/batch/check-batch-keywords.ts:210-311,693-790` — 큐의 keyword+URL 그룹을 동일 정본 키로 claim/fan-out하고, 양수 순위 결과는 기대 슬롯 전부가 정본 history에 최근 존재하며 read-back된 경우에만 큐를 삭제한다. 조회/저장/read-back 실패는 그룹 전체를 재큐한다.
- `rank-check/utils/save-rank-to-slot-naver.ts:279-363` — history INSERT의 반환 `id`를 받은 뒤 별도 SELECT로 `slot_status_id/current_rank`를 대조하는 웹 저장 정본. `historyVerified=false`이면 호출자가 성공 처리하지 못한다.
- `rank-check/test/_check-history-continuity.ts:29-82` — 정본 history를 재조회해 최근 적재 건수·고유 슬롯·최신 샘플을 대조하는 완료 검증 게이트. 모바일·웹 공통 기준. → [[골든플로우-실구동-게이트]]

## 세션 메모
- **2026-08-16 git 자동배포 전환** — 4대 서버(SRV-01~04) 배포를 SFTP 수동에서 push→5분 자동 반영으로 전환. 상세 패턴·함정·앵커는 [[윈도우서버-git-자동배포]]. 같은 날 keyword+url 그룹핑(검색 1회→활성 슬롯 전체 팬아웃 저장, `rank-check/utils/resolve-naver-slot-members.ts`)과 광고 제외 오가닉 순위([[네이버-순위-계산]] organic_expose_order 섹션)도 배포됨.
- **2026-08-17 완료 기준 고정** — 모바일/웹 모두 프로세스·heartbeat·큐·로그가 아니라 정본 history 실제 적재와 저장 직후 read-back 대조가 확인된 경우에만 검증 완료로 선언한다. 미확인 시 실패/미완료로 보고한다. 상세 기준은 [[골든플로우-실구동-게이트]].
- **2026-08-17 keyword+URL 중복 슬롯 보강** — `resolve-naver-slot-members.ts`의 canonical 키와 `check-batch-keywords.ts`의 그룹 claim/fan-out을 연결해 query/fragment/trailing slash 차이를 같은 그룹으로 묶고, 동일 `slot_id`는 1회만 저장한다. 양수 순위는 슬롯별 history coverage가 전부 확인될 때만 큐를 지운다. 로컬 회귀 통과 후 live `--once` 1사이클에서 history 총량 304→309건 증가, 독립 최근 양수행 read-back 10/10 일치, 최근 30분 중복 클러스터 0개를 확인했다.
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

## 의존 개념: [[네이버-순위-계산]] · [[골든플로우-실구동-게이트]] · [[사람행동-시뮬레이션]] · [[봇탐지-우회-스택]] · [[IP-로테이션]] · [[캡차-차단-대응]]

- **2026-08-17 Android 플릿 마감 재검증** — 17:32 KST 정식 `rankchecker-history-monitor -WindowMinutes 30` 판정은 `NO_HISTORY`: 정본 `sellermate_slot_rank_naver_history` total/positive/invalid `0/0/0`, queue `112`/assigned `1`, Android registered `3`/fresh `3`, `rankchecker_job_runs` `17`건/성공 `0`/실패 `17`. 세 기기 상태는 `ANDROID-f47c31710b353f30` stale(ADB serial `ce011601c850021404` 단절), `ANDROID-23f5713041ee286c` `NOT_FOUND_SUSPECT`, `ANDROID-854bbff616729c01` `NAVER_BLOCKED`였다. 이전 11:45 성공/history 4건은 현재 창의 성공 증거가 아니다.
- **self-ingested phantom serial 함정** — `rank-check/android-monitor/monitor-rankchecker.ps1:68`의 `serial=... worker=...` 정규식이 자기 자신이 남긴 `DEVICE_OFFLINE serial=ANDROID-*` 로그까지 registry에 재수집하고 `:92`에서 registry 전체를 tracked 기기로 열거한다. 그 결과 실제 ADB 연결 2대인데 monitor `tracked=5`가 되는 가짜 serial이 생겼다. registry/tracked 수를 live ADB 증거로 쓰지 말고, 매 회차 `adb devices`의 실제 serial과 worker ID를 분리해 교집합 검증해야 한다.

- **2026-08-17 Android 5p 운영 고정** — `rank-check/android-app/app/src/main/java/com/navertraffic/rankchecker/NaverBrowser.kt`의 동일-origin API 페이지네이션(`pagingIndex`)과 페이지 간 1.8~2.7초 지연을 정본으로 사용한다. 최대 10페이지, 차단은 `BLOCKED`, 미발견은 즉시 재요청하지 않고 5분 후 재검증한다. `rank-check/android-app/app/src/main/java/com/navertraffic/rankchecker/SupabaseApi.kt:97-156`은 history POST 반환행과 별도 GET read-back이 모두 맞을 때만 큐를 삭제한다. `rank-check/android-monitor/monitor-rankchecker.ps1` 및 작업 스케줄러 `SellermateRankcheckerAdbMonitor`가 ADB 3대의 새 로그와 DB 큐/history를 10분마다 감시한다. 2026-08-17 실측: 세 기기 online=True, `history_15m=1`, `positive=1`, `invalid_rank=0`, alerts 0.
- **2026-08-17 rankchecker 100대 운영 기반** — `rank-check/supabase/migrations/20260817000000_rankchecker_fleet_telemetry.sql`에 Samsung과 분리된 `rankchecker_devices`·`rankchecker_heartbeats`·`rankchecker_job_runs`·`rankchecker_alerts`·`rankchecker_releases`를 추가했다. APK는 60초 heartbeat와 작업 실행 telemetry를 직접 기록하고, `monitor-rankchecker.ps1`은 ADB 자동 discovery·오프라인 registry·30분 Telegram dedupe·stale heartbeat 감시를 수행한다. 실기기에서 APK heartbeat 경로가 실제 Supabase에 도달했으나 마이그레이션 미적용으로 `PGRST205/404`가 반환되어, SQL 적용 전에는 telemetry/OTA가 운영 완료가 아니다. `publish-rankchecker-release.mjs`는 release 서명 APK를 Storage에 올리고 rankchecker 채널 릴리스를 canary 대기 상태로 등록한다.

- **2026-08-17 6시간 커버리지 불균형 원인 확정** — 단순 `status=작동중` 슬롯 254개 중 실제 백필 조건(만료되지 않음 + `sellermate_slots` 마스터 active/작동중 + 유효 URL)을 만족하는 처리 대상은 189개였다. 65개는 만료 또는 마스터 비활성이라 운영 대상 집계에서 분리해야 한다. 처리 대상 189개 중 최근 6시간 양수 history는 153개, 오늘 자정 이후 history는 있으나 6시간 내 기록이 없는 슬롯은 22개, 큐에 남은 슬롯은 14개였다. `coverage-backfill.ts:103-108`이 6시간이 아니라 오늘 자정부터의 history만 조회하므로, 22개가 6시간 SLA 대상에서 조용히 제외된다.
- **재시도 hot-loop 교훈** — PC `check-batch-keywords.ts:377`와 Android `SupabaseApi.kt:29`가 모두 `last_check_date DESC`로 최신 재시도 작업을 먼저 선택한다. 최근 20개 Android job 중 16개가 동일한 `NOT_FOUND_SUSPECT` 작업의 반복이었고, 세 기기 모두 마지막 상태가 `ERROR`였다. 미발견 의심 작업을 5분 후 반환하는 정책 자체는 -1 오염을 막지만, 최신순 정렬과 결합하면 새 슬롯을 굶기는 큐 독점이 된다. 미발견 쿨다운, 시도 횟수/다음 시각, 공정한 FIFO 또는 별도 retry lane이 함께 필요하다.
- **운영 집계 교훈** — worker heartbeat/job telemetry와 PC event success는 처리 신호일 뿐 history 성공 증거가 아니다. 60분 모니터에서 history는 31건 양수였지만 Android job은 성공 13/실패 28, PC event에는 `success_without_history`가 164건 있었다. 슬롯별 커버리지·history read-back·큐 상태를 같은 창에서 조인해 판정해야 한다. → [[골든플로우-실구동-게이트]]
- **이전 Android 플릿 세션 교훈** — 2026-08-17 17:32 KST 마감 재검증은 `NO_HISTORY`(30분 history 0/0/0, Android jobs 17건 전부 실패)였다. `ce011...`은 ADB 단절/stale, `ce021...`은 `NOT_FOUND_SUSPECT`, `ce121...`은 `NAVER_BLOCKED`로 분리해야 한다. `monitor-rankchecker.ps1:68`이 자기 `DEVICE_OFFLINE` 로그를 다시 registry에 수집해 phantom serial을 만든 문제도 확인됐다. ADB discovery 결과와 telemetry registry를 동일시하지 말고, 실제 연결 serial·마지막 heartbeat·정본 history를 함께 대조한다.
