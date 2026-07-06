# project-leadgen-lab

> 한국 커뮤니티 신호로 리드 니치를 리서치·스코어링하고 동의 우선 랜딩을 만드는 로컬 리드젠 워크벤치. | 도메인: 리드젠/리서치 | 스택: Python(urllib), Naver Search/Datalab OpenAPI, Playwright CDP, Vercel | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/research_collect.py:124` `naver_get`, `:137` `naver_post`, `:151` `collect_search_results` — Naver 검색(blog/cafearticle)+Datalab 트렌드 API 클라이언트. `:116` `get_credentials`(env). → [[네이버-공식API-서명]]
- `src/research_analyze.py:115` `score_record` — 리드 스코어링(pain×4, intent×2, community, promo 감점).
- `src/cdp_capture_current_page.py:145` `connect_over_cdp` — 실행 중 Chrome 탭 CDP 캡처(9222 attach, full-page + accessibility 스냅샷). `:24` `frame_snapshot`.
- `src/marketing_automation.py` — 마케팅 캘린더 생성기.

## 의존 개념: [[네이버-공식API-서명]] · [[브라우저-세션쿠키-영속화]]
