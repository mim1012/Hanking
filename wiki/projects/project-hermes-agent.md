# project-hermes-agent

> 학습 루프(스킬 자기생성·자기개선) + 멀티플랫폼 게이트웨이를 갖춘 자기개선형 AI 에이전트. | 도메인: AI 에이전트 인프라 | 스택: Python, TUI, MCP, 멀티 LLM 어댑터 | 상태: 참조 (Nous Research, MIT)

## 핵심 자산 (다른 곳에서 주워올 것)
- `agent/` 프로바이더 어댑터 — anthropic/bedrock/gemini/codex/moonshot, 단일 인터페이스 LLM 스왑. → [[멀티에이전트-역할오케스트레이션]]
- `agent/context_compressor.py`, `prompt_caching.py`, `memory_manager.py` — 컨텍스트/메모리 스위트. → [[파일기반-지속메모리]]
- `agent/credential_pool.py`, `rate_limit_tracker.py` — 다계정 키 로테이션·쿼터.
- `gateway/` — 단일 프로세스로 Telegram/Discord/Slack/WhatsApp/Signal.
- `cron/scheduler.py` — 자연어 예약작업.

## 의존 개념: [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]]
