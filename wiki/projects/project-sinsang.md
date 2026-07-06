# project-sinsang

> 신상마켓 상호 페이지 TOP30 상품을 자동 로그인해 수집하고 워터마크 제거해 저장. | 도메인: 수집 | 스택: Python, Selenium(webdriver-manager), aiohttp, PicWish API, PyInstaller | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/services/browser_manager.py:70` `start_browser` + `:124` CDP webdriver 감지 우회(`navigator.webdriver=undefined`, `window.chrome`). → [[봇탐지-우회-스택]]
- `src/services/browser_manager.py:36` `_is_logged_in`, `:139` `login` — 모달 로그인 + data-cy 셀렉터.
- `src/services/picwish_service.py:52` `remove_watermark` — PicWish 워터마크 제거 API(multipart, `sync=1`, 결과 URL 다운로드). API URL `:14`.
- `src/collector/image_downloader.py:24` `download_images` — aiohttp 동시 다운로드. `:62` `download_one`.
- `src/collector/top30_collector.py:43` `collect_all` — scrollIntoView TOP30 수집. `src/dom_selectors.py` 셀렉터 중앙화.

## 의존 개념: [[봇탐지-우회-스택]] · [[브라우저-세션쿠키-영속화]]
