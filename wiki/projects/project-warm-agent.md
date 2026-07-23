# project-warm-agent

> Telegram + DeepSeek 호환 API로 로컬 Windows에서 파일 분석·코드 실행하는 코딩 에이전트 봇. | 도메인: AI 에이전트 인프라 | 스택: Python, python-telegram, curl_cffi, Docker/Railway | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `insane_fetch.py:26,66,94,114,173` — WAF/TLS 임퍼소네이션 페처 (canonical). → [[TLS-임퍼소네이션-WAF우회]]
- `project_store.py:31` — 세션/워크스페이스 격리 스토어 (canonical). → [[파일기반-지속메모리]]
- `run_store.py:97,144,158` — 재개 가능한 실행 체크포인트 (canonical). → [[파일기반-지속메모리]]

## 스킬 브리지 (Claude 아닌 에이전트에 CC 자산 붙이기)
- `data/skills/*.md` — 봇 네이티브 스킬(frontmatter `name`/`description`/`triggers` + 본문). `tools.py:2207 load_skills()`가 매 호출 디스크 재로딩 → **재시작 없이** 추가/갱신 반영. `list_skills`/`use_skill` 툴로 노출.
- `tools/insane_search/engine/` + `isearch.py` — CC 플러그인 insane-search 엔진을 봇에 이식(subprocess CLI). 봇은 `run_batch`로 `python isearch.py "<url>" --json` 호출. deps: curl_cffi(기존)+pyyaml+beautifulsoup4. → [[TLS-임퍼소네이션-WAF우회]]
- **함정→해결**: 봇은 `C:\warm-agent`(게스트) 실배포본에서 돌고 호스트 `data/skills`와 자동 동기 안 됐음(2026-07-23: 호스트 5개, 게스트 0개였음). 게스트 claudevm 계정은 HGFS 공유폴더도 못 읽음(`\\vmware-host\Shared Folders` NO ACCESS) → 호스트가 push해야 함. **`sync-skills-to-vm.ps1`**(해시게이트: 변경분만 vmrun copyFileFromHostToGuest, 무변경 시 게스트호출 0)를 `host-vm-keepalive.ps1` 루프에 연결(로그온마다 기동, 매 틱 호출). PS 5.1 호환 필수(부팅 런처가 `powershell` 5.1 → 삼항연산자 금지). 로그 `dev-cache\vm-share\skills-sync.log`.
- **CC 하네스 스킬 이식 한계**: OMC autopilot/ralph/ultrawork/team 등 Task/agent/hook 의존 오케스트레이션은 봇 루프(자체 spawn_subagent)로 실행 불가. 순수 방법론(debug/verify/deep-research)만 지침 md로 이식 가능.
- 병렬 서브에이전트는 이미 내장: `telegram_bot.py:1764 run_subagents_parallel`(한 턴 다중 `spawn_subagent` 동시실행, `SUBAGENT_PARALLEL_MAX=4`).

## 의존 개념: [[TLS-임퍼소네이션-WAF우회]] · [[파일기반-지속메모리]]
