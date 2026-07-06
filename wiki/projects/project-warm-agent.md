# project-warm-agent

> Telegram + DeepSeek 호환 API로 로컬 Windows에서 파일 분석·코드 실행하는 코딩 에이전트 봇. | 도메인: AI 에이전트 인프라 | 스택: Python, python-telegram, curl_cffi, Docker/Railway | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `insane_fetch.py:26,66,94,114,173` — WAF/TLS 임퍼소네이션 페처 (canonical). → [[TLS-임퍼소네이션-WAF우회]]
- `project_store.py:31` — 세션/워크스페이스 격리 스토어 (canonical). → [[파일기반-지속메모리]]
- `run_store.py:97,144,158` — 재개 가능한 실행 체크포인트 (canonical). → [[파일기반-지속메모리]]

## 의존 개념: [[TLS-임퍼소네이션-WAF우회]] · [[파일기반-지속메모리]]
