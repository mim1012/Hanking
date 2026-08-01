# AI 구독/API 사용량 조회

> Claude Code·Codex 구독의 남은 rate limit 창과 DeepSeek/Kimi/GLM API 잔액·쿼터를 프로그램으로 조회하는 기법 (비공식 엔드포인트 + OAuth 토큰 자동 갱신).

## 무엇 / 언제 쓰나
사용량 대시보드·위젯·알림 등 "지금 얼마나 남았나"를 코드로 알아야 할 때.

## 정본 구현 (canonical)
- **Claude OAuth usage 조회** — `usage-widget/widget.py:107` — `~/.claude/.credentials.json`의 accessToken으로 `GET api.anthropic.com/api/oauth/usage` (`anthropic-beta: oauth-2025-04-20`). 응답: `five_hour/seven_day{utilization,resets_at}`.
- **Claude 토큰 refresh + 파일 회수** — `usage-widget/widget.py:88` — `POST console.anthropic.com/v1/oauth/token` (client_id `9d1c250a-...`), 새 access/refresh 토큰을 `.credentials.json`에 다시 기록해 Claude Code와 공유.
- **Codex rate limit 조회** — `usage-widget/widget.py:212` — `~/.codex/auth.json` 토큰으로 `GET chatgpt.com/backend-api/wham/usage`, `used_percent` 필드 재귀 탐색 파싱(`:160`). refresh는 `auth.openai.com/oauth/token` client_id `app_EMoamEEZ73f0CkXaXp7hrann`(`:189`). 오프라인 폴백: 세션 jsonl의 `rate_limits` 스냅샷(`:269`).
- **OpenRouter 크레딧 잔액** — `usage-widget/widget.py:377` — `GET openrouter.ai/api/v1/credits` (Bearer 키). 응답 `data{total_credits,total_usage}` → 남음 = total_credits - total_usage.
- **Fireworks 월 사용액** — `usage-widget/widget.py:413` — 잔액 공개 API 없음. `GET api.fireworks.ai/v1/accounts`로 계정 ID 자동 발견 후 `GET /v1/accounts/{id}/billingUsage?startTime=&endTime=`(ISO8601)로 `serverlessCosts[]{costNanoUsd,promptTokens,completionTokens}` 합산.
- **DeepSeek/Kimi 잔액** — `usage-widget/widget.py:296` / `:323` — `GET api.deepseek.com/user/balance`, `GET api.moonshot.ai(.cn)/v1/users/me/balance` (Bearer 키).
- **GLM 코딩플랜 쿼터** — `usage-widget/widget.py:351` — `GET api.z.ai/api/monitor/usage/quota/limit`, Authorization에 **Bearer 접두사 없이** 키만.

## 주의점 / 함정
- **"refresh 토큰 회수 기록"**: 갱신 후 새 refresh_token을 원본 인증 파일에 되써주지 않으면 CLI 쪽 로그인이 깨진다 (rotating refresh token).
- Codex refresh_token이 `refresh_token_invalidated`(401)이면 프로그램으로 복구 불가 — `codex login` 재로그인만 답.
- 전부 비공식 엔드포인트라 응답 스키마 변경 대비 방어적 파싱 필요 (`used_percent` 재귀 탐색 방식).

## 배제된 접근 (dead-end)
- 만료 시 "CLI 한번 실행하세요" 안내만 하는 소극 대응 — Claude Code가 떠 있어도 `.credentials.json`을 안 갱신하는 경우가 있어(만료 3주 방치 확인) 위젯이 직접 refresh해야 실용적.

## 관련: [[project-usage-widget]] · [[실시간-토큰리프레시-락-먼저-비교]]
