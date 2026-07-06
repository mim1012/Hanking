# project-claude-forge

> "oh-my-zsh for Claude Code" — 에이전트/커맨드/스킬/훅/룰을 한 번에 심는 Claude Code 확장 번들. | 도메인: 에이전트 인프라 | 스택: Claude Code 플러그인, Bash 훅, Markdown 스킬 | 상태: 참조

## 핵심 자산 (다른 곳에서 주워올 것)
- `agents/*.md` — 11개 서브에이전트(architect/planner/code-reviewer/security-reviewer 등). → [[멀티에이전트-역할오케스트레이션]]
- `skills/team-orchestrator/SKILL.md:1` — Agent Teams(Lead 1 + Teammate 3) 오케스트레이션.
- `skills/continuous-learning-v2/SKILL.md:1` — 본능(instinct) 기반 지속학습(PreToolUse/PostToolUse 훅 관찰 → 신뢰도 가중 → skill/agent 진화). → [[파일기반-지속메모리]]
- `hooks/*.sh` — 15개 자동화 훅(output-secret-filter, remote-command-guard, db-guard, rate-limiter, security-auto-trigger 등).
- `rules/*.md`, `knowledge/` — 룰/지식 베이스.

## 의존 개념: [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]]
