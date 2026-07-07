# project-PJT-v4.7-KIS자동매매

> 한국투자증권 OpenAPI 기반 KR/US 듀얼 마켓 24시간 무인 자동매매 시스템(v4.7) + FastAPI 모바일 대시보드 + Flutter 앱. | 도메인: 트레이딩 | 스택: Python3, mojito2(KIS 래퍼), schedule, pandas, yfinance(US 폴백), FastAPI/uvicorn, Flutter/Dart | 상태: 활성

## PJT 폴더 관계 (중요)
- `PJT v4.6 (…원본`, `PJT v4.6 (09.23 수정본 반영` — **둘 다 빈 껍데기**(`nul` 파일만, 내용 소실). 재사용 자산 없음. (재확인 2026-07-07)
- `PJT v4.7 1023 new (2)/` — 컨테이너(문서/zip/배포본). **실제 소스는 중첩 폴더 `PJT v4.7 1023 new/`** (dev), 배포는 `AutoTrading_Release/` (CLAUDE.md:15-22).
- `PJT v4.7 1023 new/`에 **두 번째 CLAUDE.md**가 있다(6월 12 갱신, 14KB) — FastAPI 대시보드(`api_server.py`)·Flutter 앱(`mobile_app/`)·Dante 스크리너 파이프라인(`common/screener*.py`, `scripts/eod_scan.py`)·TradingAgents LLM 필터(`common/ta_filter.py`) 등 최상위 CLAUDE.md에 없는 최근 서브시스템을 문서화. 최상위 CLAUDE.md의 `파일:줄` 인용은 이 갱신과 무관하게 여전히 유효(같은 라인 유지).

## 핵심 자산 (다른 곳에서 주워올 것)
- `common/base_api.py`, `base_strategy.py`, `base_token_manager.py` — ABC 상속으로 US/KR 파생(멀티마켓 확장 템플릿, CLAUDE.md:100-119).
- KIS 토큰 생명주기 — prefix US/KR 분리(`us_*`/`kr_*`), 24h 만료 + 5h 잔여시 자동갱신 + 긴급 재발급(24h 우회) (CLAUDE.md:142-149). → [[KIS-API-계좌포맷]]
- **KIS API 필드 폴백 체인** — 실전/모의 필드명 차이 흡수(`ord_psbl_qty→ord_psbl_qty1→ovrs_cblc_qty`) (CLAUDE.md:159-167). KIS 연동 필수 노하우.
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/auto_market_scheduler.py:438` — 10초 폴링 KR/US/idle 자동 전환(프로덕션 진입점).
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/stop_loss_tracker.py:16` — `StopLossTracker`: JSON+CSV 하이브리드 블랙리스트, `.bak` 원자적 쓰기(`:41` 백업파일 경로, `:54` 손상 시 `.bak` 복구).
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/common/stock_config_store.py:43` — 종목 config effective-path 해석(:43)·읽기 fail-loud(:70)·원자적 쓰기 os.replace 재시도(:142)·핀 보호 watch_list 주입(:149). 다중 소비자 desync로 인한 "핀 심볼 증발" 실패 패턴 방지.
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/scripts/watchlist_ingest.py:146` — `ingest_us_candidates()`: EOD 발굴 후보 → watch_list 자동 주입(상한 10·TTL 스캔카운터 5·손절 블랙리스트 제외·핀 보호·잔고조회 실패 시 회차 스킵). LLM 시그널을 주문 경로와 분리하는 설계의 P1 구현. 설계서: `RESEARCH/us_daily_signal_pipeline.md` (배제된 dead-end: 별도 메타파일 방식 — split-brain 위험으로 단일 config `watch_list_meta` 서브키 채택).
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/common/credential_store.py:24` — `_mask()`(:24) + `patch_credentials()`(:63): 모바일 API가 KIS 자격증명을 갱신해도 **평문 시크릿을 절대 응답에 포함하지 않고** 마스킹된 메타데이터만 반환. `.env`는 임시파일 쓰기 + `os.replace`로 원자적 재작성(:107-109). 시크릿을 다루는 CRUD API에 이식 가능.
- `PJT v4.7 1023 new (2)/PJT v4.7 1023 new/common/settings_store.py:142` — `_apply_to_config_modules()`: `settings.json` 패치 시 이미 import된 `config`/`kr.config`/`us.config`를 `sys.modules` 순회로 즉시 hot-patch — 프로세스 재시작 없이 실행 중인 트레이딩 루프에 반영. `config.py`(기본값) → `.env`(시크릿) → `settings.json`(런타임 오버라이드) 3단 레이어링, 나중 값이 승리.

## 주의점
- `secret.json`, `*_api_token.json`이 소스에 평문 존재(민감정보).

## 의존 개념: [[KIS-API-계좌포맷]] · [[포트-어댑터-거래소추상화]] · [[리스크-관리-상태기계]]
