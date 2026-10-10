# usage-widget

> AI 모델(Claude/Codex/DeepSeek/Kimi/GLM) 남은 사용량을 표시하는 Windows 플로팅 위젯 + 로컬 브라우저 대시보드. | 도메인: 에이전트 인프라/데스크톱 유틸 | 스택: Python 3.11, tkinter, requests, stdlib HTTP, vanilla JS | 상태: 동작 (2026-09-21)

## 핵심 자산 (다른 곳에서 주워올 것)
- `usage-widget/widget.py:88` — Claude/Codex OAuth 토큰 자동 refresh + 인증파일 회수 기록
- `usage-widget/widget.py:451` — 프로바이더 병렬 조회(fetch_all, 스레드 fan-out) + 정규화 결과 스키마
- `usage-widget/widget.py:478` — tkinter 프레임리스 always-on-top 위젯 (드래그 이동, 위치 영속화, 우클릭 메뉴, 게이지 바)
- `usage-widget/widget.py` `_codex_profiles`/`fetch_codex` — Codex 멀티계정 병렬 조회.
  `config.json`의 `codex_profiles`에 계정별 **완전히 분리된 CODEX_HOME**(`D:\.codex-1`,
  `D:\.codex-2`)의 auth.json을 등록해 각각 독립 refresh·조회, `Codex (라벨)`로 표시.
- `%USERPROFILE%\.local\bin\codex1.cmd` / `codex2.cmd` — `CODEX_HOME`만 바꿔 codex CLI를
  실행하는 계정 전환 래퍼(PATH 등록, 터미널/Paseo 등 어디서든 `codex1 ...`/`codex2 ...`).
- `usage-widget/widget.py` `_claude_profiles`/`fetch_claude` — `CLAUDE_CONFIG_DIR` 단위로 완전히 분리된 Claude Code 자격증명을 프로필별 조회. 누락/만료 프로필도 카드에서 사라지지 않고 오류 상태로 남는다.
- `usage-widget/server.py` — `127.0.0.1` 전용 JSON/정적 서버. `/api/usage`가 표시 필드만 허용하는 sanitizer를 통과하고, 15초 강제 새로고침 하한 + 자동 갱신 주기 캐시로 공급자 조회를 코얼레싱한다.
- `usage-widget/public/` + `DESIGN.md` — 5시간/주간 사용률·남은 용량·초기화 시각·로그인 오류를 제공자/프로필별로 표시하는 반응형 카드 UI. 375/768/1280px, 긴 프로필명, 미제공 값, 인증 오류 상태를 별도 showcase로 검증한다.

## 의존 개념: [[AI-구독-사용량-조회]] · [[파일기반-지속메모리]]
