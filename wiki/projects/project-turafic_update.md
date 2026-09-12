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

- **전략인지 큐 + 노드 capability 디스패치(2026-09-12 프로덕션)** — `supabase-migrations/064-strategy-aware-queue.sql:128,213,278,636` + `066-customer-strategy-codes-and-rpc.sql:91` + `scripts/strategy-a-nocdp-runner.ts:150,748,1544,1378`. 고객이 슬롯별 A~E 다중선택 → enqueue 라운드로빈 → claim v3 capability 필터 → search-runner 자식(`shoppingtab/search-runner.ts:81` `--once`) 또는 nocdp 소셜 디스패치. 상세: [[전략인지-큐-노드-capability-게이트]]. 개통 함정 2건: [[크롬프로필-DPAPI-로그인-이식불가]](로그인 필요 셀은 노드 콘솔 로그인 필수), [[러너-인스턴스락-PID재사용]](`:197,208,281`).

- **실험 그래프 오케스트레이터** — `scripts/exp-graph/exp-graph.mjs` — 실험 상태를 DB(`experiments`/`experiment_runs`, 마이그 `supabase-migrations/060-experiment-graph.sql`)에 두고 PREFLIGHT→FIRE→HEALTH→REPAIR→DOSE→RANK_DELTA 를 결정적 노드로 돌림. 사람은 escalated/judged 에서만. 상세: [[실험-그래프-오케스트레이션]].

## 주의점 / 함정
- **슬롯 쿨다운/블랙리스트가 slot_id 기준이라 MID(상품) 단위로 걸리는 429를 막지 못한다(named failure: "슬롯≠MID 오매핑 쿨다운")** — `scripts/strategy-a-runner.ts:139,356` `slotCooldowns`가 `Map<slotId, cooldownUntil>`인데, 여러 슬롯(키워드 조합)이 같은 MID를 공유하는 게 정상 설계다(2026-08-16 PC-07 실측: slot 65/66/50/51/64/67/68 전부 `mid=10893254328` 하나를 가리킴). 슬롯 65가 429로 쿨다운 걸려도 같은 MID의 슬롯 66/67/68은 여전히 "가용"으로 잡혀 곧바로 재타격 — PC 1대만으로도 재현됨. [[네이버-순위-계산]]/기존 "429는 MID 단위" 지식과 직결되는데 구현이 slot_id로 잘못 키잉된 사례. 부가 요인: 8대 PC 플릿이 서로 조율 없이 같은 후보 풀을 순회해 슬롯 충돌까지 겹침(Supabase `pc_runner_events` 실측: 15분 창에서 슬롯 39개 중 38개를 여러 PC가 접근, 5분 이내 cross-PC 겹침 1442쌍) — 근본 원인(mid 미분리)과 부가 원인(fleet 조율 부재) 둘 다 있음, mid 기준 전환이 1차 수정 대상.
- **`PARALLEL_BROWSERS`(동시 브라우저 수) 2→1 하향 적용** — `scripts/strategy-a-runner.ts:79`. 동시 워커가 많을수록 좁은 후보 풀(43슬롯/15상품) 안에서 같은 MID를 여러 워커가 반복 타격해 429가 누적된다는 진단(아래 항목)에 따른 조치, 사용자가 이미 "1개로 하라"고 지시했었는데 코드에 반영 안 돼 있던 걸 재확인 후 수정. 커밋 `5064751d`, origin에 push 완료. **플릿 반영은 세션 종료 시점에 미완료** — sellermate-01(110.10.98.98) 원격 배포 중 막힘: SSH로 접속한 PowerShell 세션의 `$env:Path`에 git이 아예 없음(`C:\Program Files\Git`, `C:\Program Files (x86)\Git` 둘 다 없음 — 이 VPS는 표준 셋업 절차(PC-03~09) 이전에 만들어진 원본 PC라 git 설치 경로가 다르거나 PATH 등록이 안 됐을 가능성), 동시에 `Get-CimInstance Win32_Process`로 확인한 결과 node.exe 프로세스가 0개(직전까지 정상 가동 중이었는데) — daemon이 죽었는지 재시작 대기 중인지 미확인 상태로 세션 종료. **다음 세션 최우선 확인 사항**: sellermate-01 git 위치 특정(`Get-ChildItem -Recurse -Filter git.exe`류) 후 pull+재시작, 나머지 PC-02/04/05/06/07/09도 `PARALLEL_BROWSERS=1` 반영 확인 필요(전부 push 시점엔 아직 미배포).
- **"429는 IP 기반 아님, MID 단위"라는 기존 확정 지식이 사용자 반례로 뒤집힘** — 위 슬롯≠MID 쿨다운 버그를 조사하던 중, 계속 막히던 MID를 사용자가 자기 컴퓨터에서 정상 접속한 사례가 나와 "전역 MID 차단"으로는 설명 안 됨. 재해석: 차단은 (IP/세션, MID) 조합 단위 — 원격 PC 플릿은 프록시 비활성화라 PC마다 고정 IP로 같은 상품을 반복 타격하는 게 근본 문제(사용자의 1회 수동 접속은 새 신원이라 안 걸림). turafic_update Claude 프로젝트 메모리 `naver-login-redirect-hypothesis.md`(위키 링크 대상 아님) 갱신, CLAUDE.md "429 원인" 섹션도 갱신함. **이 (IP,MID) 조합 가설은 2026-08-20 CDP 부착 자체가 원인임이 확정되며 폐기됨** — 위 "CDP→noCDP 전환 확정" 항목 참고.
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

- **PC-07의 `BROWSER_ERROR`는 프록시 transport 오류로 분리됨** — 실측 최근 160건에서 `BROWSER_ERROR` 134건이 `ERR_TUNNEL_CONNECTION_FAILED`/`ERR_CONNECTION_RESET`/timeout 계열 `page.goto` 오류였고, 브라우저 수가 아니라 프록시 전송 계층이 1차 원인이었다. `engines/pc-strategy-a/proxy-health.ts:17-70`의 `isProxyTransportFailure()`/`recordConnectionFailure()`가 해당 프록시를 5분 격리하고, `scripts/strategy-a-runner.ts:440-448,561-570`이 시작 시 선택한 동일 프록시를 성공·429·연결실패 기록에 재사용한다. `engines/pc-strategy-a/strategy-a-engine.ts:639-655`는 `NAVIGATION_TRANSPORT_ERROR`로 별도 분류한다.
- **sellermate-01(VPS, 110.10.98.98)만 유독 성공률이 높은 이유는 세션 종료 시점까지 미해결** — 프록시(꺼짐, 확인됨) · 재기동 직후라서(틀림, 로컬 jsonl 8906건이 08-13~08-17 나흘 내내 40~80% 꾸준함으로 반증) · PARALLEL_BROWSERS(플릿 전체 동일 하드코딩값이라 배제) 세 가설 전부 기각됨. 실측: 같은 시간대(08-17 09~10시)에 sellermate-01은 59~70% 성공인데 나머지 5대(PC-02/04/06/07/09)는 0%. `pc_runner_events`(Supabase)엔 이 PC 기록이 최근 10건뿐이라 그동안 이벤트 업로드가 거의 안 되고 있었던 것으로 보임(로컬 jsonl은 정상 기록 중) — 이것도 원인 미상. 다음 세션에서 파볼 것: 이 VPS만의 프로필/딜레이/브라우저 채널 설정 차이, 호스팅 IP 평판, 또는 처리 슬롯 구성 차이.

- **`pc_runner_events` 중앙로그가 코드는 있었는데 한 대도 배포 안 돼 있어서 테이블이 0행이었다**(2026-08-16 세션) — `scripts/strategy-a-runner.ts`/`engines/pc-strategy-a/types.ts`에 삽입 로직은 커밋 `357eeca`로 있었지만 커밋 자체가 플릿 어디에도 배포된 적이 없었다. 6대 중 5대(sellermate-01, PC-02/04/05/06)에 배포+재기동해서 실제 적재 확인, PC-03만 접속 타임아웃으로 미배포 상태로 남음. 배포 과정에서 세 가지 PC별 함정을 추가로 발견: sellermate-01은 scp 백슬래시 경로가 무음 실패(→ [[SSH-공개키인증-Win32버그-우회]]), PC-02는 애초에 `StrategyADaemon`/`AutoUpdate` 스케줄 작업이 하나도 없어 크래시해도 재시작 안 되는 상태였음(신규 PC와 동일하게 처음부터 등록), PC-05는 `StrategyAAutoUpdate`가 "성공"을 찍으면서도 실제로는 git이 안 갱신되는 원인미상 버그(수동 git fetch는 정상 — 상세: `PC-INVENTORY.md:63`). 같은 배포로 `SUPABASE_CONTROL_URL/KEY` 폴백 수정(위 항목)도 같이 반영돼 `workerNodes` 하트비트도 5대 전부 살아남 → [[project-naver_turafic_samsung]]의 대시보드 검증으로 이어짐.
- **중앙 FleetAgent 배포 경로 추가** — `fleet-agent.ts`가 서버 `fleet-release.json`의 40자 SHA를 검증하고 각 PC에서 Git fetch/reset/npm ci/health/rollback을 수행한다. `deploy/install-one-click.ps1`/`setup-new-pc.bat`는 매 셋업마다 Git이 없으면 `Git.Git`, Node.js가 없으면 `OpenJS.NodeJS.LTS`를 WinGet으로 설치하고 Node.js 20+/npm을 검증한다. `deploy/install-fleet-agent.ps1`은 단일 PC 설치, `deploy/install-fleet.ps1`은 WinRM 동시 설치(기본 10대, 상한 20대)다. 5%→25%→100% rollout과 PC별 `agent-state.json` 수집을 운영 표준으로 삼는다. 상세: `docs/FLEET_DEPLOYMENT.md`.

- **원격 셋업 보안** — `deploy/install-fleet.ps1`은 IP 목록과 `Get-Credential`/DPAPI 자격증명을 받고, `-OpenSshPort -SshAllowedSource <관리자 IP/CIDR>`로만 OpenSSH 포트를 열도록 제한한다. 비밀번호 인증을 켜면 단일 `/32` 또는 `/128` 원본만 허용하며 대상 PC의 IP/포트를 결과와 `agent.json`에 기록한다.
- **2026-08-17 방화벽 설치 실패 교훈(재검증 필요)** — FleetAgent 설치가 Git/Node/예약작업 단계까지 통과한 뒤 `New-NetFirewallRule`에서 실패했다. `deploy/install-fleet-agent.ps1`의 splatting 인자 `Enabled = $true`가 해당 Windows PowerShell NetSecurity cmdlet의 `Enabled` enum 바인딩과 충돌해 `ParameterArgumentTransformationError`를 냈다. 방화벽 cmdlet은 OS/모듈별 enum 바인딩을 `-WhatIf`로 먼저 검증하고, 기본값이 enabled인 경우 불필요한 `Enabled` 인자를 제거하는 방식으로 호환성을 확보한다. 실제 대상 PC 재설치 성공은 아직 확인하지 못했다.

- **claim 시 "슬롯 목록 없음" 재발의 근본원인: 첫 실패부터 즉시 격리되던 quarantine 규칙**(2026-08-18) — `report_strategy_slot(_group)`가 `MID_NOT_FOUND` 보고를 받을 때마다 1회차부터 `landing_keyword_pool.mid_verified=false`로 꺼버려(`supabase-migrations/007-quarantine-repeated-mid-not-found.sql:67-79`), MID_NOT_FOUND가 흔한 이 시스템 특성상 검증된 후보 풀이 계속 줄어들다 claim이 빈 목록을 반환하는 지점까지 수렴했다. `009`(429 잠금을 연속 10회부터로 완화)·`010`(mid_verified를 2회차 실패부터만 끄는 two-strike로 전환)·`011`(과거 1회차-즉시퇴출로 잘못 격리된 행을 `verified_rank>0 and mid_not_found_count<=1` 조건으로 1회성 복구)로 수정. 라이브 재현으로 검증: MID_NOT_FOUND 발생 직후 pool row를 직접 대조해 1회차는 `mid_verified=true` 유지, 2회차만 `idle` 전환됨을 확인. Supabase REST 직접 조회로 011 백필 대상(`mid_not_found_count<=1 and mid_verified=false and verified_at is not null and verified_rank>0`)이 현재 0건임도 확인 — 백필이 실제로 적용된 상태.
- **quarantine 규칙의 2차 재발: 차단 구간의 MID_NOT_FOUND 폭주가 풀을 통째로 벗겨 "claim 빈반환"**(2026-08-22) — 위 two-strike(010)로도 못 막는 경로. 08-20 15:00~08-21 03:00 UTC CDP 플릿 스왑 중 차단(LOGIN_REDIRECT/429/렌더실패)을 러너가 `MID_NOT_FOUND`로 보고 → `report_strategy_slot`(`dev-cache/Temp/rebuild-0814-cdp/supabase-migrations/009-lock-slot-after-ten-consecutive-429.sql:82-90`)이 풀 행을 `mid_verified=false`/`idle`/6h 쿨다운으로 무효화 → 동시에 재검증 파이프라인(zero-landing)이 DB 포화로 정지해 복구 불가 → claimable 슬롯 28→2, 플릿 44대 전부 빈반환. 풀 데이터는 삭제된 게 아니라 플래그만 벗겨진 것이라 "오늘 마이그레이션이 지웠나"로 오판하기 쉬움 — **풀 행 `updated_at`의 시간대 분포**로 누가 언제 벗겼는지 바로 갈린다(오늘 갱신 0건 → 무관 확정). 복구: `037-recover-mid-not-found-pools.sql`(011의 `mid_not_found_count<=1` 제한을 뺀 전량 복원 + `refresh_claimable_slots()`) → 24,693행/3,818슬롯 복원, 캐시 2→522, 3분 내 신규 리스 167건 실측. 슬롯 자체는 MID_NOT_FOUND로 안 잠김(잠금은 429 10연속만, 같은 파일 :62-75). 근본 해결(미착수): 러너가 차단 신호를 MID_NOT_FOUND와 분리 보고 — [[키워드-파이프라인-러너호환-MID검증]]·[[Supabase-RPC-대량prepare-timeout-완화]] 참고. 같은 세션의 DB 포화 원인(kp-prepare 매분 실패 재시작, Micro 1GB에 525MB 테이블)은 [[project-adpang-production]] 쪽 이슈.
- **claim RPC 자체의 떼몰림 문제와 랜덤화 수정** → [[동시-클레임-랜덤화-떼몰림-방지]] 참고. 위 quarantine 버그를 고친 뒤에도 claim이 소수 슬롯(MID)에 쏠려 429가 계속 났는데, 원인은 `claim_strategy_slot_group`이 "가장 오래 방치된 슬롯 1개"를 결정론적으로 뽑아서 워커 9대가 동시에 같은 슬롯으로 수렴했기 때문. 상위 20개 후보를 잠근 뒤 무작위 선택하도록 교체(`013-randomize-claim-slot-pick.sql`), 배포 직후 실측으로 확인.
- **만료된 슬롯이 claim 통계를 오염시킨다** — `sellermate_slot_naver.expiry_date`가 지났는데도 `strategy_enabled=true`로 남은 슬롯 65개 발견(2026-03-13~2026-08-05 만료분). claim RPC 자체는 `expiry_date >= now()` 조건으로 이미 걸러내 동작엔 영향 없었지만, "활성 후보"로 잘못 집계돼 실제 가용 풀 크기를 오판하게 만듦. 1회성 정리(`012-disable-expired-slots.sql`) 후, adpang-production에 매일 자동 정리하는 크론 라우트(`src/app/api/sellermate/scheduler/naver-expired-slot-disable/route.ts`, 기존 `naver-success-fail-reset`과 동일 패턴)를 신설해 재발 방지.
- **같은 브랜치를 여러 세션이 동시에 작업 중일 수 있다 — 커밋 전 매번 재확인 필요**(2026-08-18) — 이 세션 중간에 `git log`를 다시 찍어보니 처음 확인했을 때 없던 커밋 4개(`f072ccef`~`f5f62559`, 러너를 2026-08-15 12:49 베이스라인으로 되돌리는 25% 카나리 롤아웃 + 좀비 프로세스 수정)가 다른 세션에 의해 이미 쌓여 있었다. 다행히 그쪽은 `pick_landing_query` 기반 로컬 라운드로빈 경로(`fleet-release.json` 카나리 대상 PC 전용, `claim_strategy_slot_group`과는 별개 코드 경로)라 직접 충돌은 없었지만, **같은 저장소를 다루는 세션이 병렬로 있다는 걸 가정하고 커밋 직전에 `git log`/`git status`를 다시 확인하는 습관이 필요**함을 실증. 참고로 이 로컬 라운드로빈(`slotCursor` 전역 변수 + PC별 독립 커서)도 위 떼몰림과 근본적으로 같은 취약점을 구조적으로 안고 있음(DB 조율이 아예 없어 오히려 더 무방비) — 다음 세션에서 확인·수정 필요.

- **jmha 고객 슬롯 PC 제외 임시조치(015, 2026-08-18)** — 모바일 안드로이드 플릿(APK 0.3.4, z1 계열+z90)이 `customer_id='jmha'` 슬롯 전담으로 전환되며 PC A 러너의 슬롯 획득 3경로를 모두 차단: ① `claim_strategy_slot_group`/`claim_strategy_slot`에 `customer_id is distinct from 'jmha'` 추가(`supabase-migrations/015-exclude-jmha-slots-from-pc-claim.sql`, 프로덕션 적용·검증 완료 — **멤버 수집에도 필터 필수**: 멤버는 mid/link_url/query 3요소로만 묶여 다른 고객 대표에 jmha가 딸려옴) ② 0815 카나리 브랜치 직조회에 `.neq('customer_id','jmha')`(커밋 `6bb87d10`) + `fleet-release.json` 핀 갱신(releaseId는 유지해야 25% 코호트가 재추첨되지 않음) ③ `pick_landing_query`는 **건드리면 안 됨** — zero-landing `/r/`(모바일 B 랜딩)과 공유라 jmha 가드를 넣으면 모바일까지 깨짐. 롤백=`015-rollback-restore-original-claims.sql` + 핀 원복(`7132310`). 부수 발견: 015 이전에도 풀 자격 통과 슬롯이 jmha 6개뿐이라 메인 claim 경로는 사실상 jmha만 잡고 있었음(master는 verified pool 9개가 있어도 `keyword_pipeline_candidates` 조인에서 전멸) — 015 이후 PC 메인경로 claim은 당분간 빈손.

## 관련: [[동시-클레임-랜덤화-떼몰림-방지]]

- **PC 트래픽 러너의 정본이 CDP(Patchright)에서 noCDP(Win32+UIA)로 전환 확정**(2026-08-19~20)
  — 같은 슬롯 풀·같은 시간대에서 noCDP 호스트(VM1) 287건 중 429 0건, 같은 시간 CDP 플릿은
  호스트당 3,000~4,500건. IP/헤더/롤아웃%가 아니라 **CDP 부착 자체**가 탐지 신호였음이 확정
  됨(그동안의 [[봇탐지-우회-스택]] 관련 헤더 위장 시도들이 전부 무의미했던 이유). noCDP 엔진
  `engines/pc-strategy-a-nocdp/nocdp-click-engine.ps1` + 러너
  `scripts/strategy-a-nocdp-runner.ts`(기존 claim/report RPC 그대로 재사용). 신규 PC는
  `zen-nocdp-fleet` 전용 격리 브랜치+매니페스트로 배포하고, 기존
  `investigate-strategy-a-pc-migration`(CDP)은 레거시/비교 기준으로 격하. 크롬 창이 visible
  콘솔에 가려 렌더링이 멈추던 별도 버그와 그 우회 → [[크롬창-오클루전-렌더차단]].
- **jmha 고객 슬롯 PC 제외 임시조치를 해제**(020, 2026-08-20) — 위 015에서 걸었던
  `customer_id is distinct from 'jmha'`를 `claim_strategy_slot_group`/`claim_strategy_slot`
  양쪽에서 제거(`supabase-migrations/020-remove-jmha-exclusion-restore-pc-claim.sql`), PC
  러너도 다시 jmha 슬롯을 claim할 수 있게 원복. 016→019에서 3일→24시간으로 완화한 pool
  재사용 규칙은 그대로 보존(jmha 조건만 제거, 다른 로직 불변 — 적용 전 Management API로
  라이브 `pg_get_functiondef`를 직접 재확인해 015/019 이후 다른 변경이 없었음을 검증한 뒤
  진행). 적용 직후 재확인: jmha 슬롯 200개가 기본 자격조건을 충족.
- **FleetAgent 설치기에 Chrome 무인설치 + `.env` 자동생성 추가** —
  `deploy/install-fleet-agent.ps1`이 기존엔 Git/Node/Patchright Chromium만 자동 설치했는데,
  noCDP는 시스템 Chrome이 필수라 공식 Enterprise MSI(`dl.google.com/edgedl/chrome/install/
  GoogleChromeStandaloneEnterprise64.msi`)로 무인 설치하는 단계를 추가(이미 있으면 스킵).
  `-SupabaseUrl`/`-SupabaseKey` 파라미터로 InstallDir에 `.env`가 없을 때만 자동 생성(RLS로
  보호되는 publishable key라 배포 파일에 넣어도 안전 — 기존 `deploy/README.md` 문서화된
  전제와 동일). `deploy/install-fleet-via-plink.ps1`은 CSV `Password`/`-GitHubToken`이
  선언만 되고 실제 원격 호출에 전달 안 되던 걸 연결. 50대 브로커 배포용 원클릭 ZIP
  (`deploy/TuraficFleetSetup.zip` + `install-zen-nocdp-broker.bat` + 지시서
  `ZEN-NOCDP-BROKER-RUNBOOK.md`)까지 패키징, 실제 PAT를 채운 배포본은 `.gitignore` 처리해
  git 유출 없이 별도 보관.
- **소프트웨어 "최신 버전" 확인은 API가 아니라 실물로 검증** → [[소프트웨어버전-실물검증-API불신]].
  Chrome versionhistory API의 stable 최상단이 실제 배포 설치파일 채널보다 앞서 있어서, API만
  보고 VM1(151.0.7922.170)을 "뒤처졌다"고 오판할 뻔했다 — 실제로는 이미 최신 배포판이었음.
- **FleetAgent로 이미 설치된 PC의 ref를 다른 브랜치로 전환하면 git이 깨진다** —
  최초 clone이 single-branch clone이라 `-Ref <새 브랜치>`로 재실행해도
  `git fetch origin <새 브랜치>`는 `FETCH_HEAD`만 채우고 `origin/<새 브랜치>` 추적 참조
  자체가 없어서, 이어지는 `git reset --hard origin/<새 브랜치>`가 `fatal: ambiguous
  argument: unknown revision`(exit=128)으로 실패한다. `git remote set-branches --add origin
  <새 브랜치> && git fetch origin`을 먼저 실행해 추적 참조를 만들어야 함 — sellermate-01을
  zen-nocdp-fleet으로 전환하며 실측(2026-08-20). `install-fleet-agent.ps1`의 저장소 준비
  단계 자체엔 아직 이 처리가 없어 다른 PC 전환마다 반복될 함정으로 남음.
- **PC의 배포 버전을 SSH 자격증명 없이 git ancestry로 확인하는 법** — 각 PC의 FleetAgent가
  Supabase `workerNodes.gitCommit`으로 하트비트를 보내므로, 그 커밋 해시를
  `git merge-base --is-ancestor <커밋> origin/<브랜치>`로 로컬에서 대조하면 어느 브랜치에
  속하는지 비밀번호 없이 알 수 있다. PC-09(`sellermate-09`)를 이 방법으로 확인한 결과
  `investigate-strategy-a-pc-migration`도 `zen-nocdp-fleet`도 아닌
  `strategy-a-runner-0815-1249-on-current-infra`라는 별도 브랜치에 머물러 있음을 발견
  (2026-08-20, 하트비트도 약 하루 정체 — 실사용 중인지 불확실).

- **`claim_strategy_slot_group()` 내 정규화 비교 누락으로 검증된 키워드 대부분이 claim 안 되고 있었다** → [[Postgres-함수내-정규화비교-불일치]]. 2026-08-18 실측: 기본 자격 슬롯 153개 중 실제 claim 가능이 6개뿐이었는데(2차 검증 완료 키워드는 `landing_keyword_pool`에 5,681건이나 쌓여 있었음), `keyword_pipeline_candidates` ↔ `landing_keyword_pool` JOIN 3곳을 정규화 비교로 통일하는 마이그레이션 `supabase-migrations/014-normalize-candidate-phrase-join.sql`을 프로덕션에 직접 적용해 88개로 회복 확인. 진단·적용·검증 전부 Supabase Management API(`SUPABASE_ACCESSTOKEN`)로 PostgREST 키 없이 처리.
- **sellermate-01(110.10.98.98)의 `C:\sellermate` 배포가 통째로 사라진 채 방치돼 있었다**(2026-08-18 확인) — 러너 코드/`node_modules` 전무, 71바이트 `unified.log`(`npx tsx` 패키지설치 확인 프롬프트에 걸려 멈춘 흔적)만 남음. `StrategyADaemon` 스케줄 작업도 "사용 안 함"(Disabled) 상태로 전환돼 있었고 마지막 실행(2026-08-17 02:58)이 강제종료 코드(`-1073741510`)로 끝난 뒤 재기동 안 됨 — 직전 세션에서 "브라우저 1개로 바꿔라"며 강제종료 후 재배포하다 중단된 것으로 추정. 재배포+daemon 재등록 필요(미해결로 남김).
- **이 워크트리의 `.env.local`은 프로덕션과 무관한 빈 Supabase 프로젝트를 가리키고 있었다** — `sellermate_slots`가 완전히 비어있고 `sellermate_slot_naver`엔 `mid` not-null 행이 0개, 관련 RPC(`claim_strategy_slot_group` 등)도 전무해서 로컬에서 슬롯 재현 테스트를 하면 오판하기 쉽다. 진짜 프로덕션 project ref(`cwsdvgkjptuvbdtxcejt`)는 메인 저장소(`D:\Project\turafic_update`) `.env`와 이 워크트리 `.env`(`.env.local` 아님)의 `supabase/.temp/project-ref`로 확인했다. 메인 저장소 `.env`의 `SUPABASE_PRODUCTION_KEY`는 레거시 포맷이라 2026-05-22부로 비활성화됨 — 위 Management API 우회로 대체.
- **키워드 파이프라인의 SERP positive와 noCDP MID 발견은 별도 계약** — 외부 엑셀의 URL·키워드만으로 `verified_rank`를 채워 active 처리하면, HTTP parser에서 확인되지 않은 MID가 러너에서 `MID_NOT_FOUND`가 된다. 최종 검증 단위는 `(slot_id, assignment_id, MID, query_phrase)`이며 noCDP와 같은 search surface·탐색 범위로 확인해야 한다. 외부 키워드는 실제 MID별 증거가 없으면 pending external로 보관하고, 실패는 삭제하지 않고 `mid_verified=false`/cooldown quarantine으로 남긴다 → [[키워드-파이프라인-러너호환-MID검증]].

## 의존 개념: [[앱-자동업데이트-채널]] · [[봇탐지-우회-스택]] · [[SSH-공개키인증-Win32버그-우회]] · [[Windows예약작업-IT트리거-로그오프함정]] · [[윈도우서버-git-자동배포]] · [[Postgres-함수내-정규화비교-불일치]] · [[동시-클레임-랜덤화-떼몰림-방지]] · [[크롬창-오클루전-렌더차단]] · [[소프트웨어버전-실물검증-API불신]]
