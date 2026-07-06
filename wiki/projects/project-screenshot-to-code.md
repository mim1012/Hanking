# project-screenshot-to-code

> 스크린샷·Figma·화면녹화를 HTML/React/Vue/Tailwind 코드로 변환하는 AI 도구. | 도메인: AI 도구 | 스택: FastAPI + React/Vite, 멀티모델(OpenAI/Anthropic/Gemini/Replicate), WebSocket 스트리밍 | 상태: 참조

## 핵심 자산 (다른 곳에서 주워올 것)
- `backend/prompts/system_prompt.py:1` `SYSTEM_PROMPT` — 에이전트형 코드생성 시스템 프롬프트(파일 툴 강제, 스택별 지시, 스크린샷 자기검증 루프).
- `backend/prompts/pipeline.py:11` `build_prompt_messages` + `prompts/plan.py` `derive_prompt_construction_plan` — 전략 분기(create vs update_from_history vs update_from_file_snapshot) 프롬프트 오케스트레이션.
- `asset_extraction.py`(스크린샷에서 로고/이미지 재사용), `image_generation/`, `video/`(녹화→프로토타입), `evals/`(평가 하네스).

## 의존 개념: [[멀티에이전트-역할오케스트레이션]]
