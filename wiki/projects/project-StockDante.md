# project-StockDante

> KIS REST 기반 국내/미국 주식 자동매매(세력봉+거래량+이평 스크리닝). 헥사고날 구조. | 도메인: 국내외 트레이딩 | 스택: Python, requests, pytest(36) | 상태: 활성 (품질 높음)

## 핵심 자산 (다른 곳에서 주워올 것)
- `poc/kis_auth.py:19-84` — 토큰 캐시 + 빈도제한(403/429/EGW00133) 방어 (canonical). → [[KIS-API-계좌포맷]]
- `poc/config.py:16-28` — 환경별 도메인/TR_ID 매핑 (canonical). → [[KIS-API-계좌포맷]]
- `poc/kis_order.py:18-74` — 주문 실행 + hashkey.
- `dante/risk.py:15-60` — 리스크 상태기계 (canonical). → [[리스크-관리-상태기계]]
- `dante/indicators.py` — 순수함수 지표. `dante/broker.py` — 포트-어댑터. → [[포트-어댑터-거래소추상화]]

## 의존 개념: [[KIS-API-계좌포맷]] · [[리스크-관리-상태기계]] · [[포트-어댑터-거래소추상화]]
