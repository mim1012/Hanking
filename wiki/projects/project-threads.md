# project-threads

> 공식 Threads Graph API 기반 계정별 예약 게시/답글/리포스트/댓글 자동응답 MVP. | 도메인: 소셜 자동화(공식 API) | 스택: Python, FastAPI, httpx, SQLite, uv | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/threads_automation/threads_api.py:124` `build_authorization_url`, `:128` `exchange_code_for_account`, `:137` `refresh_access_token` — OAuth 인증/토큰 교환/갱신 (canonical 공식 소셜 API 커넥터). `:30` `redact_access_token_url`(토큰 마스킹).
- `src/threads_automation/service.py:89` `schedule_text_post`, `:105` `publish_reply`, `:126` `repost`, `:214` `process_keyword_replies`(댓글 번호 감지→자동답글).
- `http_client.py`(async 팩토리), `models.py`(Pydantic), `storage.py`(state store).

## 대비
- **공식 API 방식** (이 프로젝트) vs **비공식 크롤링** ([[project-band-sourcing]], 쓰레드 리포스트 크롤링). insta도 Graph API 지향.

## 의존 개념: [[브라우저-세션쿠키-영속화]]
