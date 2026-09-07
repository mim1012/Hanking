# project-PJT-V5.0

> KIS(한국투자증권) 기반 국내/미국 듀얼마켓 콘솔+Tk GUI 자동매매 MVP + 2026-09-07 섹터 분석 탭(조회전용 시세/보유 스냅샷 → 섹터별 도넛·트리맵 시각화) 신규. | 도메인: 트레이딩 | 스택: Python 3, httpx2(KIS REST), tkinter(GUI), pyyaml, pytest/basedpyright/ruff | 상태: 활성(실계좌 국내 왕복 검증 완료, 섹터분석은 실계좌 미검증)

## PJT 계열 관계 (중요)
- `D:\Project\PJT V5.0`은 `D:\Project\PJT v4.7 ...` 계열([[project-PJT-v4.7-KIS자동매매]])과 **다른 프로젝트**다 — v4.7은 FastAPI 모바일 대시보드 + Flutter 앱 + Dante 스크리너/LLM 필터를 포함한 별도 코드베이스이며, V5.0은 `src/pjt_trader/` 단일 패키지의 콘솔+Tk GUI MVP다. 혼동해서 자산을 교차 포팅하지 말 것.
- 패키지명 `pjt-v5-trader`(`pyproject.toml`), 국내 3 + 미국 3종목 30초 폴링·조건부 지정가가 원래 MVP 범위(`SESSION.md:4`).

## 2026-09-07 섹터 분석 기능 (신규)
사용자가 시세/보유 종목을 섹터별로 묶어 등락·거래량·시가총액을 도넛/트리맵으로 보는 조회 전용 탭. 자동매매 엔진과 브로커 연결을 공유하지만 주문 경로와는 완전히 분리했다 — 상세 설계는 [[조회전용-분석수집기-주문격리]].

- **섹터 매핑 분리 저장** — `config/sectors.yaml`을 `settings.yaml`과 별도 파일로 관리(`src/pjt_trader/config.py`) — 분석용 섹터 등록이 매매 rule을 만들거나 자동매매를 켜지 않게. 우선순위: 사용자 매핑 > `rule.sector` > "미분류"(`market_analysis.py:108-127` `resolve_sector`).
- **KIS 시가총액 필드(추가 API 호출 없이 확보)** — 이미 호출 중인 시세 엔드포인트 응답에 시가총액 필드가 포함되어 있어, 별도 조회 없이 파싱만 추가했다:
  - 국내 정규장 `/uapi/domestic-stock/v1/quotations/inquire-price`(`FHKST01010100`) 응답의 `hts_avls`(HTS 시가총액, **억원 단위**) → `src/pjt_trader/kis.py:684-691`에서 `_KOREA_MARKET_CAP_UNIT = 100_000_000`(`kis.py:73`)을 곱해 KRW 원 단위로 환산.
  - 미국 `/uapi/overseas-price/v1/quotations/price-detail`(`HHDFS76200200`) 응답의 `tomv`(시가총액, **원시 통화 단위**, 스케일 미검증) → `kis.py:705-719`에서 그대로 파싱하고, `shar`(상장주수)×`last`(현재가)와 0.5~2배 범위인지 `_us_market_cap_plausible()`(`kis.py:992-1002`)로 검증해 벗어나면 경고 로그만 남긴다(값은 버리지 않음).
  - 국내 장후/시간외단일가/NXT 세션(`inquire-overtime-price`, `FHPST02300000`)은 응답에 시가총액 필드 자체가 없어 `market_cap=None`.
  - 공통 파서 `_quote_from_body()`(`kis.py:954-976`)에 `market_cap_key`/`market_cap_scale` 파라미터 추가, `"0"`/빈 문자열/누락은 `_optional_market_cap()`(`kis.py:979-989`)이 `None`으로 취급(면적 0 오해 방지). `Quote.market_cap: float | None = None`은 `src/pjt_trader/domain.py:74-78`에 옵션 필드로 추가(기존 dataclass 하위호환).
  - 조사 산출물: `.omx/research/market-cap-fields.md`(KIS 필드 조사, 국내/미국 엔드포인트·TR_ID·필드 의미·확신도 표, 검증 못한 항목 명시). **미검증**: 실계좌에서 `tomv` 스케일·`hts_avls` 실채움 여부(SESSION.md:9).
- **시각화** — 섹터별 등락률 단순합(pp, 가중치 없음)·도넛/트리맵. → [[Tk-Canvas-도넛-트리맵-시각화]]
- **설정 파일 안전성 리뷰(CRITICAL 수정)** — 기존 `write_config`가 settings.yaml을 비원자적으로 전체 재생성하고 손상 파일을 defaults로 침묵 리셋하던 문제를 수정. → [[원자적-설정파일-쓰기-fail-closed]]
- **GUI 테스트 안정화** — flaky한 Tk 테스트(암묵적 master 의존)와 하네스에서 안 보이는 Tk 창의 시각 검증 방법. → [[Tk-테스트-안정화-및-창캡처]]
- **검증 상태(2026-09-07)** — `uv run python -m pytest -q` 403 passed / basedpyright 0 / ruff 기존 3건(무관 파일)만. 서브에이전트 병렬 구현 + opus 리뷰 2회(핵심 CRITICAL 2·HIGH 4, GUI CRITICAL 1·HIGH 6) 전건 수정 후 위 수치. 가짜 브로커 GUI 스모크(`.omx/research/smoke_gui.py`)로 분석 조회 시 submit/cancel/open_orders/fills 호출 0회 확인.

## 기존 MVP 핵심 자산 (2026-07-24 실주문 검증 완료분)
- KIS 스로틀 + 재시도 — `src/pjt_trader/kis.py` `_request`(EGW00201/5xx 재시도), 토큰 24h 캐시(`kis_token_cache.py`).
- 호가단위 보정 — `src/pjt_trader/tick_size.py`(KRX 2023 개편 테이블), 매수 내림/매도 올림, `strategy.py`에서 적용.
- 미체결/체결 안전망 — `fill_tracker.py`(체결 감지), 타임아웃 자동취소(`orders.timeout_minutes` 기본 5분), 보유 중 재매수 금지.
- 실주문 왕복 검증 — 삼성전자 1주 매수 253,500원 체결 → 매도 254,500원 체결(2026-07-24), `logs/trade_log.xlsx` 기록.

## 의존 개념: [[조회전용-분석수집기-주문격리]] · [[Tk-Canvas-도넛-트리맵-시각화]] · [[원자적-설정파일-쓰기-fail-closed]] · [[Tk-테스트-안정화-및-창캡처]] · [[KIS-API-계좌포맷]]
