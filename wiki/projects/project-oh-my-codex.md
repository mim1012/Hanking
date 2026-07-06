# project-oh-my-codex

> OpenAI Codex CLI를 멀티에이전트 오케스트레이션 시스템으로 확장하는 애드온. | 도메인: AI 에이전트 인프라 | 스택: Node≥20, tmux, MCP | 상태: 참조 (OMX, MIT)

## 핵심 자산 (다른 곳에서 주워올 것)
- `prompts/` — 역할 프롬프트 29종(planner/architect/reviewer/verifier 등) (canonical). → [[멀티에이전트-역할오케스트레이션]]
- `README.md:38` — 팀 파이프라인(team-plan→prd→exec→verify→fix), tmux 실행.
- `.omx/state/` — 모드 라이프사이클 지속 상태. → [[파일기반-지속메모리]]

## 의존 개념: [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]]
