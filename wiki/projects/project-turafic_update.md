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

- **`pc_runner_events`(`supabase-migrations/001-pc-runner-events.sql`)가 adpang-production의 PC 플릿 "가용 슬롯 계산"(용량 계획) 기능의 데이터 소스로 재사용됨** — `hostname`/`success`/`ts` 컬럼이 안드로이드의 `strategy_experiment_log`(device_name/success/created_at)와 같은 모양이라 `fleet-capacity.ts`의 순수 계산 함수를 그대로 태울 수 있었음. 상세: [[project-adpang-production]].

## 주의점 / 함정
- **슬롯 쿨다운/블랙리스트가 slot_id 기준이라 MID(상품) 단위로 걸리는 429를 막지 못한다(named failure: "슬롯≠MID 오매핑 쿨다운")** — `scripts/strategy-a-runner.ts:139,356` `slotCooldowns`가 `Map<slotId, cooldownUntil>`인데, 여러 슬롯(키워드 조합)이 같은 MID를 공유하는 게 정상 설계다(2026-08-16 PC-07 실측: slot 65/66/50/51/64/67/68 전부 `mid=10893254328` 하나를 가리킴). 슬롯 65가 429로 쿨다운 걸려도 같은 MID의 슬롯 66/67/68은 여전히 "가용"으로 잡혀 곧바로 재타격 — PC 1대만으로도 재현됨. [[네이버-순위-계산]]/기존 "429는 MID 단위" 지식과 직결되는데 구현이 slot_id로 잘못 키잉된 사례. 부가 요인: 8대 PC 플릿이 서로 조율 없이 같은 후보 풀을 순회해 슬롯 충돌까지 겹침(Supabase `pc_runner_events` 실측: 15분 창에서 슬롯 39개 중 38개를 여러 PC가 접근, 5분 이내 cross-PC 겹침 1442쌍) — 근본 원인(mid 미분리)과 부가 원인(fleet 조율 부재) 둘 다 있음, mid 기준 전환이 1차 수정 대상.
- **`PARALLEL_BROWSERS`(동시 브라우저 수) 2→1 하향 적용** — `scripts/strategy-a-runner.ts:79`. 동시 워커가 많을수록 좁은 후보 풀(43슬롯/15상품) 안에서 같은 MID를 여러 워커가 반복 타격해 429가 누적된다는 진단(아래 항목)에 따른 조치, 사용자가 이미 "1개로 하라"고 지시했었는데 코드에 반영 안 돼 있던 걸 재확인 후 수정. 커밋 `5064751d`, origin에 push 완료. **플릿 반영은 세션 종료 시점에 미완료** — sellermate-01(110.10.98.98) 원격 배포 중 막힘: SSH로 접속한 PowerShell 세션의 `$env:Path`에 git이 아예 없음(`C:\Program Files\Git`, `C:\Program Files (x86)\Git` 둘 다 없음 — 이 VPS는 표준 셋업 절차(PC-03~09) 이전에 만들어진 원본 PC라 git 설치 경로가 다르거나 PATH 등록이 안 됐을 가능성), 동시에 `Get-CimInstance Win32_Process`로 확인한 결과 node.exe 프로세스가 0개(직전까지 정상 가동 중이었는데) — daemon이 죽었는지 재시작 대기 중인지 미확인 상태로 세션 종료. **다음 세션 최우선 확인 사항**: sellermate-01 git 위치 특정(`Get-ChildItem -Recurse -Filter git.exe`류) 후 pull+재시작, 나머지 PC-02/04/05/06/07/09도 `PARALLEL_BROWSERS=1` 반영 확인 필요(전부 push 시점엔 아직 미배포).
- **"429는 IP 기반 아님, MID 단위"라는 기존 확정 지식이 사용자 반례로 뒤집힘** — 위 슬롯≠MID 쿨다운 버그를 조사하던 중, 계속 막히던 MID를 사용자가 자기 컴퓨터에서 정상 접속한 사례가 나와 "전역 MID 차단"으로는 설명 안 됨. 재해석: 차단은 (IP/세션, MID) 조합 단위 — 원격 PC 플릿은 프록시 비활성화라 PC마다 고정 IP로 같은 상품을 반복 타격하는 게 근본 문제(사용자의 1회 수동 접속은 새 신원이라 안 걸림). [[naver-login-redirect-hypothesis]] 메모 갱신, CLAUDE.md "429 원인" 섹션도 갱신함.
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

- **"신규 PC인 줄 알았는데 이미 배포+운영중"이 반복되는 패턴**(PC-02, PC-03에 이어 PC-07/PC-09에서도 재현) — `C:\sellermate`가 이미 있고 `strategy-a-runner.ts --test`(비headless)가 수동으로 떠 있는데 `schtasks /query /tn StrategyADaemon`은 "지정된 파일을 찾을 수 없습니다"(미등록). 커밋도 12개 안팎 밀려있는 채로 방치. 새 PC 접속 시 `Test-Path C:\sellermate`(또는 `if exist`)로 기존 배포부터 확인하고, 있으면 clone 생략 후 stray 프로세스 종료→PAT pull+reset→schtasks 등록 순서로 "신규 셋업과 동일하게" 처리하면 됨 — 실제로 다른 절차가 필요하지 않았음.
- **SSH 접속 시 원격 기본 셸이 cmd가 아니라 PowerShell인 PC가 있다**(PC-09, DESKTOP-KOE2687) — `whoami & if exist ...` 같은 cmd 문법을 그대로 보내면 `앰퍼샌드(&) 문자를 사용할 수 없습니다` 파서 에러. `;`/`Test-Path`로 전환해야 함. 복잡한 다줄 명령은 셸 종류와 무관하게 `[Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($cmd))` 인코딩 후 `powershell -EncodedCommand`로 보내는 게 따옴표 중첩 이스케이프 문제를 가장 안전하게 피함(plink→cmd/PowerShell 중첩 시 반복적으로 깨짐).
- **PowerShell 실행정책이 `npm.ps1`을 막는 PC가 있다**(PC-09) — `npm install`이 `UnauthorizedAccess`(`about_Execution_Policies`)로 실패. `npm.cmd install`로 직접 호출하거나 `-ExecutionPolicy Bypass`로 우회.
- **RDP 방화벽 DisplayGroup이 한국어 Windows에선 한글로만 잡힌다** — `Enable-NetFirewallRule -DisplayGroup "Remote Desktop"`(영문)은 "그런 DisplayGroup 개체 없음" 에러. `Get-NetFirewallRule | Where DisplayGroup -like "*원격*"`로 실제 이름(`"원격 데스크톱"`) 확인 후 그 이름으로 실행 — PC-05 때는 이 원인을 못 찾아 RDP 활성화를 포기했었는데(`PC-INVENTORY.md` PC-05 항목) PC-07/09에서 근본 원인 특정.
- **cmd `echo VALUE>>file & echo VALUE2>>file` 체이닝은 `&` 앞에 공백이 없어도 트레일링 공백이 낄 수 있다**(PC-07 재현) — `.env`에 `PC_ID`/`EQUIPMENT_NAME` 추가할 때 cmd echo 대신 PowerShell `Get-Content`(필터)+`Set-Content`로 파일을 통째로 재작성하는 편이 안전. 트레일링 공백이 섞이면 값 매칭이 조용히 깨질 수 있어 발견이 늦어짐.

- **`ProxyHealth`(`engines/pc-strategy-a/proxy-health.ts`)가 429는 블랙리스트하지만 프록시 연결실패는 블랙리스트 안 함** — `record429()`만 있고 연결실패(`ERR_TUNNEL_CONNECTION_FAILED` 등)를 기록하는 메서드가 없어서, `proxy-list.txt`(2045개) 중 죽은 프록시가 계속 재선택되며 `BROWSER_ERROR`를 반복 유발함. PC-07에 `--use-proxy` 카나리 적용해서 실측(재시작 후 60건 중 `BROWSER_ERROR` 29건 > `RATE_LIMITED_DETAIL` 25건). 프록시를 실전 도입하려면 연결실패도 블랙리스트하는 로직 추가가 선행돼야 효과가 제대로 남. 수정 대기.
- **sellermate-01(VPS, 110.10.98.98)만 유독 성공률이 높은 이유는 세션 종료 시점까지 미해결** — 프록시(꺼짐, 확인됨) · 재기동 직후라서(틀림, 로컬 jsonl 8906건이 08-13~08-17 나흘 내내 40~80% 꾸준함으로 반증) · PARALLEL_BROWSERS(플릿 전체 동일 하드코딩값이라 배제) 세 가설 전부 기각됨. 실측: 같은 시간대(08-17 09~10시)에 sellermate-01은 59~70% 성공인데 나머지 5대(PC-02/04/06/07/09)는 0%. `pc_runner_events`(Supabase)엔 이 PC 기록이 최근 10건뿐이라 그동안 이벤트 업로드가 거의 안 되고 있었던 것으로 보임(로컬 jsonl은 정상 기록 중) — 이것도 원인 미상. 다음 세션에서 파볼 것: 이 VPS만의 프로필/딜레이/브라우저 채널 설정 차이, 호스팅 IP 평판, 또는 처리 슬롯 구성 차이.

- **`pc_runner_events` 중앙로그가 코드는 있었는데 한 대도 배포 안 돼 있어서 테이블이 0행이었다**(2026-08-16 세션) — `scripts/strategy-a-runner.ts`/`engines/pc-strategy-a/types.ts`에 삽입 로직은 커밋 `357eeca`로 있었지만 커밋 자체가 플릿 어디에도 배포된 적이 없었다. 6대 중 5대(sellermate-01, PC-02/04/05/06)에 배포+재기동해서 실제 적재 확인, PC-03만 접속 타임아웃으로 미배포 상태로 남음. 배포 과정에서 세 가지 PC별 함정을 추가로 발견: sellermate-01은 scp 백슬래시 경로가 무음 실패(→ [[SSH-공개키인증-Win32버그-우회]]), PC-02는 애초에 `StrategyADaemon`/`AutoUpdate` 스케줄 작업이 하나도 없어 크래시해도 재시작 안 되는 상태였음(신규 PC와 동일하게 처음부터 등록), PC-05는 `StrategyAAutoUpdate`가 "성공"을 찍으면서도 실제로는 git이 안 갱신되는 원인미상 버그(수동 git fetch는 정상 — 상세: `PC-INVENTORY.md:63`). 같은 배포로 `SUPABASE_CONTROL_URL/KEY` 폴백 수정(위 항목)도 같이 반영돼 `workerNodes` 하트비트도 5대 전부 살아남 → [[project-naver_turafic_samsung]]의 대시보드 검증으로 이어짐.

## 의존 개념: [[앱-자동업데이트-채널]] · [[봇탐지-우회-스택]] · [[SSH-공개키인증-Win32버그-우회]] · [[Windows예약작업-IT트리거-로그오프함정]] · [[윈도우서버-git-자동배포]]
