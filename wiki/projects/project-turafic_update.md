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

## 의존 개념: [[앱-자동업데이트-채널]] · [[봇탐지-우회-스택]]
