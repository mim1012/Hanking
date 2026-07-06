# project-PJT-v4.7-KIS자동매매

> 한국투자증권 OpenAPI 기반 KR/US 듀얼 마켓 24시간 무인 자동매매 시스템(v4.7). | 도메인: 트레이딩 | 스택: Python3, mojito2(KIS 래퍼), schedule, pandas, yfinance(US 폴백) | 상태: 활성

## PJT 폴더 관계 (중요)
- `PJT v4.6 (…원본`, `PJT v4.6 (09.23 수정본 반영` — **둘 다 빈 껍데기**(`nul` 파일만, 내용 소실). 재사용 자산 없음.
- `PJT v4.7 1023 new (2)/` — 컨테이너(문서/zip/배포본). **실제 소스는 중첩 폴더 `PJT v4.7 1023 new/`** (dev), 배포는 `AutoTrading_Release/` (CLAUDE.md:15-22).

## 핵심 자산 (다른 곳에서 주워올 것)
- `common/base_api.py`, `base_strategy.py`, `base_token_manager.py` — ABC 상속으로 US/KR 파생(멀티마켓 확장 템플릿, CLAUDE.md:100-119).
- KIS 토큰 생명주기 — prefix US/KR 분리(`us_*`/`kr_*`), 24h 만료 + 5h 잔여시 자동갱신 + 긴급 재발급(24h 우회) (CLAUDE.md:142-149). → [[KIS-API-계좌포맷]]
- **KIS API 필드 폴백 체인** — 실전/모의 필드명 차이 흡수(`ord_psbl_qty→ord_psbl_qty1→ovrs_cblc_qty`) (CLAUDE.md:159-167). KIS 연동 필수 노하우.
- `auto_market_scheduler.py` — 10초 폴링 KR/US/idle 자동 전환(프로덕션 진입점).
- `StopLossTracker` — JSON+CSV 하이브리드 블랙리스트, `.bak` 원자적 쓰기.
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/common/stock_config_store.py:43` — 종목 config effective-path 해석(:43)·읽기 fail-loud(:70)·원자적 쓰기 os.replace 재시도(:142)·핀 보호 watch_list 주입(:149). 다중 소비자 desync로 인한 "핀 심볼 증발" 실패 패턴 방지.
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/scripts/watchlist_ingest.py:146` — `ingest_us_candidates()`: EOD 발굴 후보 → watch_list 자동 주입(상한 10·TTL 스캔카운터 5·손절 블랙리스트 제외·핀 보호·잔고조회 실패 시 회차 스킵). LLM 시그널을 주문 경로와 분리하는 설계의 P1 구현. 설계서: `RESEARCH/us_daily_signal_pipeline.md` (배제된 dead-end: 별도 메타파일 방식 — split-brain 위험으로 단일 config `watch_list_meta` 서브키 채택).

## 주의점
- `secret.json`, `*_api_token.json`이 소스에 평문 존재(민감정보).

## 의존 개념: [[KIS-API-계좌포맷]] · [[포트-어댑터-거래소추상화]] · [[리스크-관리-상태기계]]
