# project-Decepticon

> LLM 서브에이전트에게 킬체인 단계를 위임하는 자율 침투테스트/레드팀 오케스트레이터(모노레포). | 도메인: 보안 자동화 | 스택: Python, LangGraph, MCP, Docker, Go 런처 | 상태: 참조

## 핵심 자산 (다른 곳에서 주워올 것)
- `packages/decepticon/decepticon/agents/prompts/standard/decepticon.md:1` — 오케스트레이터 시스템 프롬프트 (canonical). "직접 실행 금지(No shell), 모든 공격은 `task()` 서브에이전트 위임", recon→exploit 강제 핸드오프, "I'll just check one thing" 안티패턴 방지. → [[멀티에이전트-역할오케스트레이션]]
- `agents/prompts/standard/`, `prompts/plugins/` — 역할별 전문 에이전트 프롬프트(analyst/exploit/recon/forensicator/scanner/verifier).
- `packages/decepticon/decepticon/agents/build.py:1` — 플러그인/슬롯 기반 에이전트 빌드(PluginBundle override 병합, `safety_critical` allowlist + `SafetyOverrideViolation` 게이트).
- OPPLAN 파일기반 상태 + `OPPLANMiddleware`(단계 전이 검증). → [[파일기반-지속메모리]]

## 의존 개념: [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]]
