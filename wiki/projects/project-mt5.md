# project-mt5

> MetaTrader5 OHLCV 수집 → 볼린저밴드 전략 백테스트 → 성과지표/리포트(실주문 아님). | 도메인: 백테스트 | 스택: Python, MetaTrader5, pandas/numpy | 상태: 참조

## 핵심 자산 (다른 곳에서 주워올 것)
- `data/fetcher.py:8-40` — MT5 시세 수집(타임프레임 매핑, `copy_rates_range`).
- `risk/manager.py:10-48` — 랏 사이징 리스크 매니저. → [[리스크-관리-상태기계]]
- `indicators/bollinger.py:5` — 볼린저밴드 순수함수. `strategies/base.py` — 전략 추상화. → [[포트-어댑터-거래소추상화]]
- `backtest/metrics.py`, `report.py` — 백테스트 성과지표/리포트.

## 의존 개념: [[리스크-관리-상태기계]] · [[포트-어댑터-거래소추상화]]
