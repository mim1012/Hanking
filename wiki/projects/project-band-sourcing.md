# project-band-sourcing

> 네이버 밴드 도매 피드를 크롤링해 상품 파싱·마진 계산 후 WooCommerce에 자동 등록. | 도메인: 수집/커머스 | 스택: Python, Playwright(sync), woocommerce API, SQLite | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `band-sourcing-1/src/band_scraper.py:174` `_save_session`/`:188` `_load_session` — 세션 쿠키 영속(sameSite 보정) (canonical). → [[브라우저-세션쿠키-영속화]]
- `band-sourcing-1/src/band_scraper.py:78-152` — Playwright execution-context 파괴 복구(`_safe_evaluate` retry, `_wait_for_stable_context`, `_safe_goto`). 장시간 크롤링 안정성 핵심.
- `band-sourcing-1/src/band_scraper.py:509` `_scroll_and_collect_posts` — 무한 스크롤 + seen_keys 중복제거 + cutoff 종료.
- `band-sourcing-1/src/content_parser.py:27` — 한국어 상품 텍스트 파서(가격코드/사이즈/브랜드/원단 정규식).
- `band-sourcing-1/src/margin_engine.py:214` — 구간별 마진 + 카테고리/성별 분류. `band-sourcing-1/src/wc_uploader.py:15` — WooCommerce wc/v3 업로드(중복검사·중앙값 가격).

## 의존 개념: [[브라우저-세션쿠키-영속화]] · [[봇탐지-우회-스택]]
