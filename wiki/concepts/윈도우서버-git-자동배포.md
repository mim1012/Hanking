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

관련: [[project-sellermate_naver_place_all]] · 대비되는 수동 방식과 서버 함정 목록은 memory `project_remote_server_deploy` 참조
