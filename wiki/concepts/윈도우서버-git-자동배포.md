# 윈도우 서버 git 자동배포 (PAT 내장 + GCM 차단 + schtasks)

> 헤드리스 Windows VPS 봇 서버에 "push만 하면 5분 내 반영"되는 git 자동배포를 붙이는 패턴.
> SFTP 수동 배포의 드리프트 문제를 `reset --hard` 동기화로 구조적으로 제거한다.

## 무엇 / 언제 쓰나
Windows 서버 여러 대에 코드를 반복 배포하는 프로젝트(트래픽봇·순위체크 등). SFTP 수동 배포가 서버별 드리프트를 만들기 시작하면 이걸로 전환.

## 정본 구현 (canonical) — sellermate_naver_place_all (2026-08-16, 4대 서버 E2E 검증)
- **셋업 스크립트** — `sellermate_naver_place_all/deploy/setup-git-autodeploy.py:51` (`setup()`): 기존 배포 폴더를 `git init`(재init 무해) → PAT 내장 URL로 `remote add fork`(`:60`) → shallow fetch → `reset --hard fork/{branch}`(`:66`)로 드리프트 일괄 제거. `.env`/`node_modules`/브라우저 프로필은 untracked라 보존됨.
- **동기화 배치** — `setup-git-autodeploy.py:30-37` `SYNC_BAT`(ASCII 전용, SFTP `wb`로 전송): `GIT_TERMINAL_PROMPT=0` + `fetch --depth 30` + `reset --hard`. 예약작업 `schtasks /create /sc minute /mo 5`(`:75`)로 5분 주기 — 콘솔 전용이라 RDP 세션 분리 문제(schtasks GUI 함정) 없음.
- **GCM 차단** — `deploy/finalize-git-autodeploy.py:32-36`: `.git/config`에 `[credential]\n\thelper =`(공란)를 SFTP로 직접 기입. PAT가 URL에 있으므로 헬퍼 자체가 불필요.
- **런타임 무중단 반영**: 러너가 라운드마다 자식 프로세스를 `spawn()`(tsx 재트랜스파일)하는 구조면 파일 교체만으로 다음 라운드부터 반영 — 프로세스 재시작 로직이 아예 필요 없다. (상시 프로세스 재시작까지 필요하면 `rank-check/launcher/remote-watch-launcher.ts` 참고)

## 승격 게이트
- **검증된 성공**: 빈 커밋 ae5b2e4 push → 4대 서버 전부 1사이클(5분) 내 HEAD 자동 갱신 확인.
- **이름 붙인 실패**: `fatal: Unable to persist credentials with the 'wincredman' credential store` — fetch는 성공하지만 GCM이 URL 내장 자격증명을 저장하려다 실패해 로그를 오염. `credential.helper` 공란 기입으로 해소.
- **dead-end**: ① SSH 세션에서 `git clone` — GCM이 `/dev/tty` 없는 환경에서 대화형 프롬프트를 못 띄워 인증 실패(PAT URL도 GCM이 가로챔). credential.helper를 죽이기 전엔 재현됨. ② 서버 기본 셸이 PowerShell/cmd 혼재라 SSH 명령의 **중첩 따옴표는 서버별로 다르게 깨진다** — 명령에 따옴표를 아예 안 쓰는 설계(공백 없는 경로 전제) + 파일은 전부 SFTP `wb`로 우회.

## 보안 노트
PAT는 **fine-grained, 해당 repo 한정, Contents: Read-only**로 발급 — 서버 `git remote -v`에 노출되지만 읽기 전용이라 유출 시 revoke만 하면 됨. 발급 시 "Repository access → Only select repositories" 지정 누락하면 owner가 맞아도 404(실측).

## 무조건 reset 자동배포 위에서 카나리 하는 법 (2026-08-17 실측)

- **함정(이름 붙인 실패)**: 이 패턴의 동기화 배치는 해시 비교 없이 매 사이클 `reset --hard`라, SFTP로 넣은 카나리 파일이 **5분 내 소리 없이 원복**된다. 같은 날 두 번 실측(배포 byte 검증까지 통과한 뒤 덮임) — 로그에는 "HEAD is now at ..."만 남는다.
- **검증된 절차**: ① 카나리 대상 1대의 동기화 예약작업만 `Disable-ScheduledTask` ② SFTP 배포+byte 재조회 검증 ③ 그린 게이트(성공 N회 + 정본 history read-back) ④ git push로 승격(전 서버 자동 반영) ⑤ 예약작업 `Enable` — reset이 카나리본을 동일 내용의 커밋본으로 교체하며 수렴. 카나리 스크립트: `sellermate_naver_place_all/deploy/deploy-json-rank.py`의 `DEPLOY_ONLY` env 필터.
- **부수 함정**: `schtasks /End`로 세운 러너 작업은 RestartCount(실패시만)로 안 살아난다 — 수동 /End엔 수동 `/Run`이 짝. 진단용 원격 실행은 SSH 세션 종료 시 자식이 죽으므로(OpenSSH) 예약작업으로 띄울 것.

관련: [[project-sellermate_naver_place_all]] · 대비되는 수동 방식과 서버 함정 목록은 memory `project_remote_server_deploy` 참조

## 플릿 에이전트 + 100대 이상 점진 배포 (turafic_update, 2026-08-17)

- **원격 셋업 입력** — `turafic_update/deploy/install-fleet.ps1:3-57`은 PC 이름/IP 목록, 실행 시 `Get-Credential` 비밀번호 입력, 현재 계정에 귀속된 `Import-Clixml` 보호 자격증명을 지원한다. 비밀번호는 목록·배치·Git에 저장하지 않는다.
- **SSH 포트 개방** — `turafic_update/deploy/install-fleet-agent.ps1:120-208,390-420`은 명시적 옵션과 관리자 IP/CIDR allowlist가 있을 때만 OpenSSH Server/sshd와 제한 방화벽 규칙을 구성한다. `Any`, zero CIDR, 잘못된 prefix를 거부하고 SSH 비밀번호 인증은 `/32`·`/128` 단일 호스트만 허용한다.

- **서버 선언 상태** — `turafic_update/fleet-release.json`이 릴리스 SHA, 브랜치, 러너, 최소 Node 버전, rollout 비율을 선언하고 `turafic_update/scripts/publish-fleet-release.ts:1-100`이 커밋과 매니페스트를 분리된 release-control 커밋으로 발행한다.
- **PC 에이전트** — `turafic_update/fleet-agent.ts:36-224`, `fleet-agent-config.ts:63-89`, `fleet-agent-support.ts:1-163`이 JSON/env 설정, Git/npm 명령, runner health를 분리해 담당한다. 매니페스트를 검증한 뒤 shallow fetch → 선언된 대상 SHA 직접 fetch/확인 → `reset --hard` → lockfile 변경 시에만 `npm ci` → health window를 수행하고 실패 시 직전 SHA로 되돌린다.
- **100대 이상 폭주 방지** — `turafic_update/fleet-agent-core.ts:89-117`의 SHA-256 고정 코호트와 jitter, `turafic_update/deploy/install-fleet.ps1:14-57`의 WinRM `ThrottleLimit`(1~20, 기본 10)으로 동시 설치·polling을 분산한다.
- **각 PC 의존성 부트스트랩** — `turafic_update/deploy/install-fleet-agent.ps1:16-67,117-138`은 Git이 없으면 `Git.Git`, Node.js가 없으면 `OpenJS.NodeJS.LTS`를 WinGet으로 설치하고 PATH/알려진 설치 경로를 다시 확인한 뒤 Node.js 20 이상과 npm을 검증한다.
- **Windows 작업 등록** — `turafic_update/deploy/install-fleet-agent.ps1:198-209`은 `/IT` 없이 `SYSTEM + onstart + HIGHEST`로 에이전트 하나를 등록하고 최초 `agent-state.json`까지 확인한다. 기존 전역 `node.exe`/`chrome.exe` 종료 방식은 사용하지 않는다.
- **검증된 경계** — `turafic_update/fleet-agent.test.ts`, `turafic_update/engines/pc-strategy-a/proxy-health.test.ts`와 2026-08-17 로컬 Git 원격 수동 QA에서 설정 매핑, 0% hold, 이전 SHA→새 SHA 업데이트와 health 상태 기록을 확인했다.
