# project-warm-agent

> Telegram + OpenAI-호환 API로 파일 분석·코드 실행·레드팀을 수행하는 에이전트 봇. 실행지=**Kali Linux VM**(systemd `warm-agent.service`), 프로바이더=**OpenRouter**(메인 moonshotai/kimi-k2, light deepseek/deepseek-chat, 폴백 체인). | 도메인: AI 에이전트 인프라 | 스택: Python, python-telegram, curl_cffi, NordVPN/Tor | 상태: 활성 (2026-07-28: 로컬 Windows→Kali VM 이전 + GLM→OpenRouter 전환)

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

## 에이전트 인프라 학습 (2026-07-28, 재사용 가치)
- **OpenAI-호환 프로바이더 교체 + OpenRouter 네이티브 폴백**: `MODEL_CONFIG`가 base_url·key **하나를 main·light 공유** → 프로바이더 스왑 = `.env` base_url+key+모델명만. OpenRouter 폴백은 payload에 `models:[a,b,c]` 배열 주입(`call_model_message` `telegram_bot.py:960`; primary 레이트리밋/다운 시 순서대로 자동 라우팅) — env `MODEL_FALLBACKS`→`MODEL_FALLBACK_CHAIN`(`telegram_bot.py:212`), **main 호출만**(explicit light 호출 제외). 모델 선택은 지연 벤치("hi" 2회 타이밍)로 실측 후. → [[하네스-플러그인-흡수-vs-설치]]
- **이름 붙인 실패 — Anthropic 사이버 안전필터**: offensive-security 대화맥락에선 Claude(OpenRouter `anthropic/*`)가 `finish_reason=content_filter`+`refusal` 필드로 **거부**(무해 과제도 히스토리 오염 시). 봇 증상=빈 응답("no tool call"). ∴ **공격형 레드팀 봇엔 Claude 부적합** → kimi/deepseek/glm. 회복=`refusal`/`reasoning`을 content로 표면화(빈 응답 방지).
- **execv 제자리 self-restart (systemd, 재사용)**: 자기수정 프로세스가 systemd(`Restart=always`) 아래서 재시작할 때 `subprocess.Popen(main.py)`+exit는 **폴러 중복→Telegram 409**. 정답=`os.execv(python,[python,'main.py'])`로 이미지 제자리 교체(같은 PID, systemd 무자각, 새 코드 로드). 마커 저장은 execv 전에(반환 안 함). `_restart_process` `telegram_bot.py:4032`, `apply_self_cmd` `telegram_bot.py:3856`. self-modify 루프(`/self_improve`→`/apply_self confirm`)가 Kali에서 end-to-end 작동. → [[파일기반-지속메모리]]
- **연결드롭 자동재개**: 모델 `RemoteDisconnected`/timeout을 레이트리밋처럼 비종료 체크포인트+쿨다운+자동재개(`_is_conn_drop_error` `telegram_bot.py:906`). 국경/VPN 경유 엔드포인트에서 유용.
- **배포 기법 (vmrun MCP)**: `vmrun_run`은 stdout 미반환(OK/실패만)→`/tmp/*.log` 리다이렉트 후 `vmrun_copy_from`으로 회수. 인라인 복잡셸은 공백·메타문자로 mangle(exit127)→**스크립트파일(LF) `program=/bin/bash args=/tmp/x.sh` 단일인자 실행**. 안전배포=스테이징→백업→py_compile+import스모크→통과시만 restart.

## 의존 개념: [[TLS-임퍼소네이션-WAF우회]] · [[파일기반-지속메모리]]
