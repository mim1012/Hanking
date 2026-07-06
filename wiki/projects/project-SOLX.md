# project-SOLX

> KIS REST 기반 미국주식(SOXL 등) 그리드 자동매매. Excel 설정 브리지 + 텔레그램 + EXE. | 도메인: 해외주식 트레이딩 | 스택: Python, requests/websockets, PyInstaller | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/kis_rest_adapter.py:150-166` — 계좌 접미사(`-01`) 파싱 (canonical, 가장 견고). → [[KIS-API-계좌포맷]]
- `src/kis_rest_adapter.py:90-97` — 해외주식 TR_ID 상수표 (canonical). → [[KIS-API-계좌포맷]]
- `src/kis_rest_adapter.py:172-208` — 토큰 캐시(5분 마진) + Approval key + Hashkey.
- `src/kis_rest_adapter.py:590-659` — 해외주문 실행(거래소 자동감지 SOXL→AMEX).
- `.claude/skills/kis-health`, `excel-check` — KIS 연결·Excel 검증 스킬.

## 의존 개념: [[KIS-API-계좌포맷]] · [[포트-어댑터-거래소추상화]]
