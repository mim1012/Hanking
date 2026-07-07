# 하네스 플러그인 — 흡수 vs 설치

> 외부 Claude Code 스킬/플러그인을 만났을 때 "설치"가 아니라 "고유한 규율만 SSOT로 흡수"를 기본값으로 삼는 결정 패턴. 에이전트 인프라 공통.

## 무엇 / 언제 쓰나
남의 하네스 플러그인·스킬(예: fablize, kulaxyz self-learning-skills)을 도입할지 판단할 때. 소스를 뜯어 3층(텍스트 규칙 / 훅 강제 / 실행 엔진)으로 분해하고, 기존 룰과 겹치는 텍스트 층은 버리고 고유 아이디어 한 줄만 기존 SSOT 파일에 흡수한다. 설치는 훅·엔진 층이 실제로 이 환경에서 작동하고, 그 값어치가 드리프트·의존성 비용을 넘을 때만.

## 정본 구현 (canonical) — 흡수 결과가 사는 곳
- **fablize investigation-protocol 흡수** — `.claude/rules/fable5-operating-mode.md` §6 — "디버깅=경쟁 가설 3개+·인과사슬·기각가설 보고" 한 항목만 흡수, 출처 주석 부착(2026-07-06). 플러그인 미설치.
- **kulaxyz self-learning-skills 흡수** — `_wiki/AGENTS.md:61` 규칙 7(승격 게이트 3가지: 검증된 성공·이름 붙인 실패·배제된 dead-end) — 설치 없이 규칙만 흡수(log 2026-07-06).
- **fable-5-orchestration-guide 강제 게이트 흡수(=이 저장소 최초의 "설치" 분기)** — `D:\.claude\hooks\orchestration-gate.py` + `D:\.claude\settings.json` PreToolUse 등록 + 토글 `D:\.claude\scripts\orchestration-gate.ps1 on|off|status`(2026-07-07). successwiki.io 가이드의 4계층 중 **강제 게이트만** 포팅: 메인 에이전트의 직접 코드수정을 턴(prompt_id)당 2파일로 제한, 초과분·Bash 우회(`sed -i`/리다이렉트/tee)는 exit 2 차단, 서브에이전트(`agent_id`/`agent_type` 존재)·비코드(md/json/yaml)·off 상태는 통과, 오류 시 fail-open. 나머지 3계층은 버림(dead-end 참조). **11개 합성 페이로드로 실차단 검증 완료** — 판단 절차 2층(훅 강제)이 "실작동+값어치"를 넘겨 처음으로 설치 분기가 발동한 사례. Windows 포팅 포인트: `~` 대신 스크립트 위치 기준 config 앵커, bash 심링크/`.bashrc` 래퍼→PowerShell 토글. [[멀티에이전트-역할오케스트레이션]]

## 판단 절차 (소스 3층 분해)
1. **텍스트 층** — 규칙/pack 문구. 기존 룰(`fable5-operating-mode.md` 등)과 대조 → 70~80% 중복이면 버림.
2. **훅 강제 층** — Stop/PostToolUse 훅으로 행동을 실차단하는가. 텍스트 룰이 못 하는 유일한 순증분. **단 이 환경에서 실제 작동하는지 검증 필수**.
3. **실행 엔진** — goals.py 같은 상태 엔진. 개념은 대개 기존 게이트와 중복.
→ 순증분(2·3)이 실작동+값어치 있으면 설치, 아니면 텍스트 1줄만 흡수.

## 주의점 / 함정
- **이중 지침 소스 드리프트** — 플러그인 setup이 자기 규칙 블록을 `CLAUDE.md`에 주입하면, 같은 always-on 행동 지침이 기존 `fable5-operating-mode.md`와 두 곳에서 나온다. 문구가 미묘히 달라지면 에이전트가 뭘 따를지 충돌 → AGENTS.md SSOT 단일화 원칙과 정면 배치. massage 환불 사고([[레거시-코드체계-호환-필터]])와 같은 계열의 "문서 드리프트" 리스크.
- **Windows fail-open 훅 = 조용한 무효과** — fablize 훅은 전부 `python3`+bash `.sh` 호출이고 win32엔 보통 `python`만/`.sh`는 Git Bash 필요. 게다가 전 훅이 fail-open 설계라 안 맞으면 조용히 `{}` 반환 → "설치는 됐는데 강제는 0"이 가장 유력한 실패 상태. 설치 전 훅이 실제 발동하는지(예: gate_stop `MAX_STOP_BLOCKS=2` 실차단) 테스트 프로젝트에서 확인해야 값어치가 생긴다. **회피법(2026-07-07 orchestration-gate 설치에서 적용): ① 호출을 `python3`→`python -X utf8`로 바꾸고 bash 심링크·`.bashrc` 대신 순수 PowerShell 토글로 포팅 ② `~` 앵커는 win32에서 `C:\Users`를 가리켜 config(`D:\.claude`)와 어긋나므로 스크립트 파일 위치 기준으로 앵커 ③ 설치 직후 합성 페이로드 N개를 stdin으로 흘려 exit 2(차단)가 실제로 나는지 검증(fail-open이면 전부 exit 0 → 즉시 발각).**

## 배제된 접근 (dead-end)
- **`setup.sh global` 주입 후 방치** — 배제. `~/.claude/CLAUDE.md`(= OMC 파일)에 fablize 블록이 들어가 기존 룰과 중복·드리프트원이 됨. 플러그인 훅은 `hooks.json`으로 설치만 해도 자동 등록되므로, 굳이 텍스트 블록까지 주입할 이유가 없다(원했던 순증분은 훅뿐).
- **통째 설치로 "혹시 몰라 전부"** — 배제. 텍스트 80% 중복 + Windows 훅 불확실 + 업데이트 의존성 1개 → 손익이 설치 쪽 손해.
- **fable-5-orchestration-guide 4계층 통째 설치** — 배제(2026-07-07). ① `sonnet->opus` 리매핑은 `model-role-division.md`(Sonnet=구현 담당)와 정면충돌 ② `deep-reasoner`/`runner` 서브에이전트는 OMC/ecc의 `executor`/`architect`/`planner`/haiku와 중복 ③ `fable.md` 선언·on/off 심링크 스위치는 기존 룰+OMC 킬스위치와 중복 + WSL/`.bashrc` 전용이라 PowerShell 환경 부적합. 유일한 순증분인 강제 게이트(권고→물리차단)만 포팅. 추가 배제: 구독(OAuth) 사용자에겐 가이드의 핵심 동기인 "종량제 토큰 절약"이 약해 리매핑층 가치가 더 낮음.

## 관련: [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]] · [[레거시-코드체계-호환-필터]]
