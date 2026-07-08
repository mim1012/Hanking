# project-Crypto

> Binance + Bybit **암호화폐 선물 자동매매 봇** — 조건 기반 진입/청산, 실시간 시장 감시. | 도메인: 트레이딩(암호화폐 선물) | 스택: Python3(PyQt5 GUI / headless VPS / Flask 웹) 정본 + C#/.NET8 WPF(개발중), 파일기반 config.json(DB 없음), TradingView screener, Telegram | 상태: 활성/성숙(2025-11~2026-02 실거래 로그, 대형 테스트 스위트, SOLID Phase4 리팩터 진행). origin `github.com/mim1012/Crypto`.

> 등록된 트레이딩 프로젝트(SOLX/StockDante/mt5/PJT-v4.7-KIS)는 전부 한국 주식(KIS)/MT5 FX — Crypto는 **암호화폐 선물(Binance/Bybit)**, 다른 자산군·다른 서명체계라 중복 아님. 재사용 패턴(포트-어댑터·리스크 상태기계)만 공유.

## 핵심 자산 (다른 곳에서 주워올 것)
- **포트-어댑터 거래소 추상화** — `api/base_api.py:174` `BaseExchangeAPI(ABC)` — `place_order`/`get_positions` 등 추상 포트 + 표준 dataclass(`APICredentials`/`OrderRequest`(reduce_only·close_position·time_in_force 포함)/`PositionInfo`, `:48`~). thread-local 세션(`:190`). 구현체 `api/binance/futures_client.py`·`api/bybit/futures_client.py`. → [[포트-어댑터-거래소추상화]]
- **거래소별 HMAC 서명 2종(추상화의 정당성)** — Binance는 raw `query_string` 서명 `api/binance/futures_client.py:136`; Bybit V5는 `timestamp+api_key+recv_window+params` 헤더 서명 `api/bybit/futures_client.py:115`. Binance 헤지모드 `dualSidePosition→positionSide`(`futures_client.py:208`). → [[KIS-API-계좌포맷]](암호화폐 대응 형제)
- **리스크 진입 게이트(검증 상태기계)** — `risk/risk_manager.py:96` `validate_entry_signal()` — 일일거래수→최대포지션→일일손실한도→포지션크기 4게이트 순차, 하나라도 False면 진입 차단. 일일손실 `daily_loss_pct <= -limit`(`:142`), 날짜롤오버 카운터 리셋(`:130`). → [[리스크-관리-상태기계]]
- **자동 재연결 WebSocket 매니저(지수 백오프)** — `core/websocket_manager.py:17` — 하트비트 ping + 백오프 재연결 `delay=min(base*2^attempts, max)`(1s→60s 캡, 최대 10회, `:162`), `ping_interval=20/ping_timeout=10`. → [[실시간-스트리밍-소켓아키텍처]]
- **레지스트리 기반 조건 팩토리(전략 플러그인)** — `conditions/condition_factory.py:397` `CONDITION_CLASSES` 딕셔너리 매핑 + `create_condition()`(`:408`)가 GUI params 병합·인스턴스화. 진입/청산 타입 혼용 방지 위해 `ExitConditionFactory` 분리. 새 전략 추가 지점.
- **이중 거래소 병렬 백엔드(차익/미러링)** — `core/dual_exchange_backend.py:72` `DualExchangeBackend` — Binance+Bybit 가격/오더북 스트림을 `asyncio.Task`로 동시 실행(`price_streams: Dict[str, asyncio.Task]` `:87`), `ArbitrageOpportunity` dataclass(`:60`).

## 교훈 — Offset-1 현재봉 포함 버그 (2026-01-30 수정, 재사용 가능한 트레이딩 지표 교훈)
`conditions/entry/price_channel.py:294` — Donchian 채널이 **아직 형성 중인 현재봉을 포함**하면 신호가 조기 발화. 처방: `high_prices[-(period+1):-1]`로 현재봉 제외 + `len < period+1` 데이터 충분성 체크. 같은 클래스 버그가 PCT 트레일링 청산(`conditions/exit/pct_condition.py`)·PC-main 청산에도 있었고 전부 수정. 규칙 "모든 지표 계산은 현재 미완성 봉 제외" + 전용 회귀테스트 3종(`test_donchian_fix.py` 등) PASS. 임의 트레이딩 지표(MA/채널/트레일링)에 재사용되는 이름 붙인 실패 패턴.

## 의존 개념: [[포트-어댑터-거래소추상화]] · [[리스크-관리-상태기계]] · [[실시간-스트리밍-소켓아키텍처]] · [[KIS-API-계좌포맷]]
