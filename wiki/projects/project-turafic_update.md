# project-turafic_update

> TURAFIC 네이버 트래픽 봇 플릿의 **자가 업데이트 런처** — `.exe` 하나를 ~1000대 원격 윈도우 PC에 1회 배포하면, 이후 GitHub에서 최신 러너 코드를 자동으로 당겨 재실행(재배포 불필요). 봇 자체가 아니라 봇을 감싸는 업데이트/릴리스채널 + 프로세스 슈퍼바이저. | 도메인: 앱 자동업데이트(호스트=네이버 트래픽 봇) | 스택: Node18+TypeScript → esbuild+`pkg`로 단일 `.exe`(node18-win-x64), raw https/http + `child_process` + `execSync('git …')`, Supabase(workerNodes) | 상태: 프로덕션. origin `github.com/mim1012/turafic_update`.

> 등록된 `turafic`·`naver_turafic_samsung`(트래픽 봇 본체)과 다름 — 이건 업데이트 메커니즘. 두 가지 독립 방식을 함께 실음.

## 핵심 자산 (다른 곳에서 주워올 것) → [[앱-자동업데이트-채널]]
- **① 매니페스트 업데이터** — `auto-updater.ts:100` — GitHub-raw `version.json`을 3분마다 폴링, `localVersion.timestamp >= remote` 면 스킵. 고정 파일목록 다운로드.
- **원자적 파일 스왑(tmp→.bak→rename)** — `auto-updater.ts:301` `downloadFile` — `.tmp`로 받고 기존 파일을 `.bak` 백업한 뒤 `fs.renameSync`로 원자 교체(`:328`~`:332`). "반쯤 쓰인 바이너리를 남기지 않는다" 패턴.
- **캐시 버스팅 HTTP JSON fetch** — `auto-updater.ts:303` — URL에 `?t=${Date.now()}` 붙여 GitHub-raw CDN 캐시 우회(폴링 채널 필수).
- **그레이스풀 자식 재시작(SIGTERM→5s 후 SIGKILL)** — `auto-updater.ts:236` `restartRunner` — 소프트킬 후 5초 뒤 강제킬, 다시 기동. 감시형 장기 자식 프로세스 재적용에 재사용.
- **② git-pull 릴리스 채널** — `launcher.ts:268` `gitPull` — `git fetch origin main` + `git reset --hard origin/main` 3분마다(초기 `git clone --depth 1` `:243`). 파일단위 HTTP 대신 레포 전체 델타를 git으로.
- **계층적 config 해석(env > Supabase(hostname) > config.json > default)** — `config.ts:80` `loadConfigAsync` — 각 PC의 `nodeType`(어떤 러너를 띄울지)을 `os.hostname()`으로 Supabase workerNodes에서 조회, env/파일/기본값 폴백. "플릿 노드가 관제 DB에서 자기 역할을 자가구성" 패턴.
- **슈퍼바이저 루프** — `launcher.ts:505` — `while(true)` 러너 실행→종료 시 `RESTART_DELAY`(5s) 대기→재기동 전 `gitPull`. 크래시-재시작 + 재시작시-업데이트를 묶음.

## 주의점 / 함정
- **선언만 되고 검증 안 되는 해시** — `VersionInfo.hash`(`auto-updater.ts:29`)가 optional로 선언돼 있으나 **다운로드 무결성 검증에 전혀 안 쓰임**(trust-on-HTTPS). 공급망 관점 갭.
- **`git reset --hard`가 로컬 변경 소실** — 원격 강제 동기라 원격 PC의 로컬 수정은 조용히 폐기됨.
- `auto-updater.ts`와 `auto-updater-new.ts`는 바이트 동일 중복.
- **신규 PC SSH 접속 시 키 인증이 조용히 실패할 수 있다** → [[SSH-공개키인증-Win32버그-우회]]. ACL·관리자그룹이 정상이어도 발생(PC-02, `218.148.204.120`에서 실측). plink+비밀번호로 우회.
- **`SUPABASE_CONTROL_URL`/`SUPABASE_CONTROL_KEY` 미설정 시 heartbeat가 조용히 no-op** — `scripts/strategy-a-runner.ts:82-83` 두 변수가 없으면 `controlDb`가 `null`이 되고 `sendHeartbeat()`가 `if (!controlDb) return;`로 매번 아무것도 안 함(에러도 안 남음). `workerNodes`가 실제 가동 PC들인데도 0행이었던 원인. `workerNodes`는 프로덕션과 **같은 Supabase 프로젝트**에 있으므로(`:53-58`의 기존 PRODUCTION 폴백과 동일 패턴으로) CONTROL 변수가 없으면 PRODUCTION 값으로 자동 폴백하도록 수정(`:59-65`) — 신규 env 배포 없이 코드 배포만으로 전 PC에 해결됨.
- **키워드 풀 정본은 `slot_naver`가 아니라 `sellermate_slot_naver`** — Supabase REST `Content-Range`로 실측(946행 vs 938행, 컬럼 구조도 다름 → 동의어/뷰 아닌 별개 물리 테이블). 실제 가동 중인 `scripts/strategy-a-runner.ts`(`:239,268`)는 `sellermate_slot_naver` + `landing_keyword_pool`을 읽는다. 접두어 없는 `slot_naver`/`traffic_navershopping`은 구 러너(`runner/production-runner.ts`) 전용이며, `traffic_navershopping`이 실측 0행이라 그 경로는 죽어있음이 확인됨. `landing_keyword_pool`은 zero-landing 전용이라던 설계문서(`zero-landing/docs/keyword-rotation-design.md`)와 달리 PC러너가 실제로 후보 필터링에 쓰고 있음(문서가 outdated).
- **`daily_target`(일일 트래픽 한도)이 어디서도 강제되지 않던 gap** — DB 컬럼과 설정 UI(adpang-production)까지는 있는데 큐 적재/봇 클레임/PC러너 세 곳 다 참조 안 해서 무제한 처리됐음. PC 쪽은 `acquireMultipleSlots()`(`scripts/strategy-a-runner.ts:337-344` 부근) 슬롯 조회에 `success_count < daily_target` 필터 추가로 수정(성공수만 카운트, 실패는 한도에 미포함). Android 쪽은 `claim_android_naver_task` RPC에 동일 조건 추가 필요 → [[project-naver_turafic_samsung]].

- **헤드리스 데몬 `schtasks`는 `/IT` 쓰면 안 된다** → [[Windows예약작업-IT트리거-로그오프함정]].
  `/sc onlogon ... /IT`로 등록한 원격 PC 데몬(`--headless` 브라우저 자동화)이 RDP 세션 로그오프 후
  트리거 자체가 무반응이 되는 걸 PC-05(`220.121.252.178`)에서 실측. `/sc onstart /ru <계정>
  /rp <비번>`으로 교체해야 로그인 여부 무관하게 재시작됨. PC-03/04/05/06 전체 재등록.
- **GitHub PAT를 `git remote`에 임베드해 무인 자동갱신** → [[윈도우서버-git-자동배포]] 패턴 재사용.
  SSH 배포키가 [[SSH-공개키인증-Win32버그-우회]] 때문에 이 PC들에서 안 통해서, HTTPS+PAT
  URL로 우회. 이번엔 `sellermate_naver_place_all`처럼 fine-grained/repo한정 PAT를 처음부터
  쓰지 않고 broad PAT를 대화창으로 전달받아 급하게 적용함 — 세션 종료 후 폐기·재발급 필요(보안
  트레이드오프를 사후에 인지한 사례로 기록).
- **Windows 컴포넌트 저장소 손상 PC는 `Add-WindowsCapability`(OpenSSH 기능 설치)가 막힌다** —
  PC-06에서 `DISM /Online /Cleanup-Image /ScanHealth`가 4.9%에서 오류1009(레지스트리 손상)로
  실패. DISM/SFC 복구 대신 [Win32-OpenSSH GitHub 릴리즈 zip](https://github.com/PowerShell/Win32-OpenSSH/releases)을
  받아 `install-sshd.ps1`로 수동 설치하는 우회로 SSH만 살림(OS 손상 자체는 미해결로 남김).
- **한글 Windows 계정명은 plink/SSH 인증이 거부될 수 있다**(유니코드 정규화 NFC/NFD 불일치 추정)
  — PC-06 원래 계정 `컴맛집`으로는 비밀번호가 맞아도 `Configured password was not accepted`.
  영문 계정(`net user sellermate ... /add` + `net localgroup Administrators sellermate /add`)을
  새로 만들어 그걸로 접속하는 게 가장 빠른 우회.
- **다른 Windows 계정이 소유한 git 저장소는 `dubious ownership`으로 막힌다** — 배포 시 사용한
  계정과 SSH 접속 계정이 다르면(PC-06: `컴맛집`이 clone, `sellermate`로 접속) git이 소유권 불일치로
  전 명령을 거부. `git config --global --add safe.directory <경로>`로 해결.

## 의존 개념: [[앱-자동업데이트-채널]] · [[봇탐지-우회-스택]] · [[SSH-공개키인증-Win32버그-우회]] · [[Windows예약작업-IT트리거-로그오프함정]] · [[윈도우서버-git-자동배포]]
