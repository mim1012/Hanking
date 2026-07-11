# project-claude-cockpit

> Claude Code + Codex의 스킬/에이전트/커맨드/플러그인/훅/MCP를 한 대시보드에서 조회·관리하고 워크플로우로 조립하는 제로-디펜던시 로컬 도구. | 도메인: AI 에이전트 인프라 | 스택: Node.js v22(`node:http`만, npm 패키지 0개), vanilla JS 프론트엔드 | 상태: 활성 (이 머신에서 실사용 중, `/api/inventory` 실행 검증 931개 항목: skill 596·command 184·agent 133·plugin 9·hook 6 + codex 3)

## 핵심 자산 (다른 곳에서 주워올 것)
- **멀티소스 통합 인벤토리 스캐너** — `lib/scan.mjs:318-337` `scanInventory()` — Claude 루트마다 standalone skills(`:57`)·agents(`:85`)·commands(`:106`)·플러그인 번들(`:127`, `plugins/cache/<marketplace>/<plugin>/<version>/`)·`installed_plugins.json`(`:165`)·`settings.json`/`settings.local.json` 훅(`:194`)을 훑고, Codex는 `:253` `scanCodex()`가 skills + `config.toml`(미니멀 TOML 테이블 리더 `:233` `parseTomlTables()`)의 `plugins.*`/`mcp_servers.*` 테이블을 읽어 하나의 아이템 배열로 합친다. 읽기 전용(주석 `scan.mjs:1-3`에 명시), 카운트는 `runtime.kind` 키로 집계(`:331-335`). 2026-07-11 세션에서 실제 실행 검증: 596/184/133/9/6 + codex 1/1/1 = 931.
- **YAML 프런트매터 폴딩 파서** — `lib/scan.mjs:11-31` `parseFrontmatter()` — `---` 블록에서 `name`/`description` 추출, 들여쓰기 연속줄을 값에 접어넣는(fold) 미니 파서. 정규식 기반, 외부 YAML 라이브러리 불필요.
- **세이프 뮤테이션 레이어(백업 우선 + 트래시 + .disabled 토글)** — `lib/write.mjs:31-96` — `guard(p)`(`:31`)가 모든 쓰기를 `resolveRoots()`가 반환한 Claude/Codex 루트 밖이면 예외로 거부, `backup(p)`(`:38`)가 기존 파일을 `.data/backups/`에 타임스탬프 복사 후에만 덮어쓰기(`writeManagedFile:46`), `trash(target)`(`:61`)은 `rmSync` 대신 `.data/trash/`로 `renameSync`(하드 삭제 없음), `toggleSkill(path, enable)`(`:74`)은 스킬 디렉터리를 형제 `.disabled/` 폴더로 옮기고 되돌리는 것만으로 활성/비활성을 구현.
- **워크플로우 CRUD + 3포맷 익스포트** — `lib/write.mjs:110-162` — `.data/workflows.json`에 로컬 저장(`listWorkflows`/`saveWorkflow`/`deleteWorkflow`), `exportWorkflow(id, format)`(`:142`)가 같은 스텝 배열을 번호 매긴 `/slash` 리스트, `omc-team` 파이프라인 스니펫, 제네릭 `workflow` phase 스크립트 3형식으로 변환.
- **제로-디펜던시 REST + 정적 서빙** — `server.mjs:82-153` — `node:http`만으로 `/api/inventory`·`/api/item`·`/api/skill`·`/api/file`·`/api/toggle`·`/api/delete`·`/api/workflows`(GET/POST/DELETE)·`/api/workflow/export` 라우팅, 정적 서빙은 `serveStatic()`(`:61`)이 `resolve()`한 경로가 `PUBLIC_DIR` 밖이면 403으로 차단(path traversal 방지).
- **루트 자동탐지 + config.json 오버라이드** — `lib/roots.mjs:34` `resolveRoots()` — `~/.claude`/`D:\.claude`/`C:\.claude`, `~/.codex`/`D:\.codex` 후보 중 실존 디렉터리만 채택, `config.json`(`:8` `CONFIG_PATH`)이 있으면 우선.
- **키보드 내비게이션 검색/필터 UI** — `public/app.js:908-963` — 아이템 리스트(`handleListKeydown:908`)와 전역(`handleGlobalKeydown:935`, `/`→검색 포커스)에 `keydown` 리스너를 달아 ArrowUp/ArrowDown/Enter로 리스트를 순회, 검색 인풋의 ArrowDown(`:979`)으로 첫 행 진입.
- **반응형 오프캔버스 모바일 레이어** — `public/style.css:706-827`(폰 `@media ≤640px`) + `public/app.js:1021-1035`(오프캔버스 토글 배선) — 데스크톱 3컬럼을 폰에서 리스트 단일 컬럼 + 사이드바 오프캔버스 슬라이드 + 상세 바텀시트로 전환. 터치타깃 48/40px, 입력창 16px(iOS 줌 방지), Esc 우선순위 닫기(`app.js:935-957`). 상세는 [[반응형-오프캔버스-모바일]] concept 참조.

## 빌드 방식 메모 (concept 미승격)
3레인 병렬 에이전트 워크플로우(작성 레인 ≠ Fable UI/UX 리뷰 레인)로 빌드·리뷰됨 — [[멀티에이전트-역할오케스트레이션]]의 원자분해+포크 레버와 같은 계열이나, claude-cockpit 자체는 역할 프롬프트 라이브러리나 팀 파이프라인 코드 자산을 갖고 있지 않으므로 그 개념 페이지의 변형 항목으로는 등재하지 않음(프로세스 메모로만 남김).

세이프 뮤테이션 레이어(백업+트래시+.disabled 토글)와 멀티소스 인벤토리 스캔은 둘 다 승격 게이트의 "배제된 dead-end" 요건(시도했으나 실패한 대안 1건)을 이 세션에서 확인할 수 없어 concept로 승격하지 않음 — 이름 붙은 실패 패턴(예: "실수 삭제 비가역")과 검증된 성공(위 스캔 실행 결과)은 있으나 dead-end 근거가 없어 [[하네스-플러그인-흡수-vs-설치]] 페이지의 승격 게이트 정신에 따라 프로젝트 메모로만 유지. 재방문 시 실제 사용 중 발견되는 실패 접근이 생기면 concept 후보로 재평가.

## 의존 개념: [[멀티에이전트-역할오케스트레이션]] · [[반응형-오프캔버스-모바일]]
