# KIS API 계좌 포맷 & 인증

> 한국투자증권 Open API의 계좌번호 8+2 분리, OAuth 토큰 캐시, 환경별 TR_ID 분기. 4개 트레이딩 프로젝트 공통.

## 무엇 / 언제 쓰나
KIS REST/WebSocket으로 국내/해외 주식 자동매매·조회를 붙일 때. 재구현 금지.

## 정본 구현 (canonical)
- **계좌 접미사 파싱** — `SOLX/src/kis_rest_adapter.py:150-166` — `"12345678-01"`/`"1234567801"` 양쪽 입력을 하이픈·공백 제거 후 `[:8]`=CANO, `[8:10]`=ACNT_PRDT_CD, 최소 10자리 검증 + 로그 마스킹. **가장 견고한 구현.**
- **`my_prod`(뒤 2자리) 의미 매핑** — `open-trading-api/kis_devlp.yaml:20-25` — `01`=종합, `03`=국내선물옵션, `08`=해외선물옵션, `22`=개인연금, `29`=퇴직연금.
- **도메인** — 실전 `openapi...:9443`, 모의 `openapivts...:29443`, WS `ops...:21000/31000` (`kis_devlp.yaml:27-31`).

## 토큰 & 인증 (canonical)
- **토큰 파일 캐시 + 빈도제한 방어** — `StockDante/poc/kis_auth.py:19-84` — 만료 10분 마진, 403/429/EGW00133 시 캐시 재사용. 근거: KIS 토큰 24h 유효 + 재발급 빈도제한.
- **스레드세이프 캐시(.NET)** — `AutoTradeX/src/AutoTrader.Core/Services/Auth/KisAuthService.cs:18-70` — SemaphoreSlim 락, 만료 5분 전 자동갱신.
- **Hashkey 주문 위변조 방지** — `StockDante/poc/kis_order.py:18-74`, `SOLX/src/kis_rest_adapter.py:628-629`.
- **실제 만료시각 기준 캐시(PJT V5.0, 2026-09-09)** — `PJT V5.0/src/pjt_trader/kis_token_cache.py` / `PJT V5.0/src/pjt_trader/kis.py` — KIS가 토큰 재요청 때 새 토큰이 아니라 이미 나이든 기존 토큰을 반환할 수 있다. `issued_at + 24h`로 만료를 계산하면 끝자락에 EGW00123(만료 token)을 반복 전송한다. 재사용 구현은 응답 `access_token_token_expired` 또는 JWT `exp`를 단일 기준으로 저장하고, 만료 응답을 받은 조회 GET만 1회 재시도해야 한다. 주문 POST는 접수 여부가 불명확하므로 자동 재전송 금지.

## TR_ID 분기 (canonical)
- **국내 paper/real** — `StockDante/poc/config.py:16-28` — `VTTC0802U`(모의)/`TTTC0802U`(실전) 등, 시세 TR 공통.
- **해외 상수표** — `SOLX/src/kis_rest_adapter.py:90-97` — 현재가 `HHDFS00000300`, 매수 `TTTT1002U`(모의 `VTTT1002U`), 매도 `TTTT1006U`, 잔고 `TTTS3012R`.

## 주의점 / 함정 (⚠️ 안전)
- **모의투자 플래그는 실주문을 막지 못한다** — `AutoTradeX/src/AutoTrader.Core/Configuration/KisSettings.cs:36-39` — `IsPaperTrading`는 잔고조회 TR_ID만 바꿈. KIS는 실전/모의를 **계좌번호(도메인)로만** 구분. 재사용 시 반드시 인지.
- **취소는 조회가 아니라 주문 전송** — PJT V5.0 장기운영 정정. 자동취소도 KIS 취소 TR을 전송하므로 실주문 OFF/정지 게이트와 같은 mutation gate로 막아야 한다.

## 관련: [[project-SOLX]] · [[project-StockDante]] · [[포트-어댑터-거래소추상화]] · [[리스크-관리-상태기계]]
