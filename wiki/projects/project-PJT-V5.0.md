# project-PJT-V5.0

> KIS(한국투자증권) 기반 국내/미국 듀얼마켓 콘솔+Tk GUI 자동매매 MVP + 2026-09-07 섹터 분석 탭(조회전용 시세/보유 스냅샷 → 섹터별 도넛·트리맵 시각화) + 2026-09-09 장기운영 안정화. | 도메인: 트레이딩 | 스택: Python 3, httpx2(KIS REST), tkinter(GUI), SQLite, pyyaml, pytest/basedpyright/ruff | 상태: 활성(U1~U18 코드 검증 완료, U19 8일 가상 테스트 통과, 실제 192시간 fake-broker GUI soak 대기)

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

## 2026-09-09 장기운영 안정화 상태
일주일 이상 GUI 자동매매를 목표로 주문 중복·토큰 만료·원장/엑셀 장애·시간창/GUI 바인딩 문제를 정리했다. 현재 판정은 **코드 검증 완료(U1~U18) + 오프라인 장기 시나리오 통과(U19 일부)**이며, 실제 192시간 fake-broker GUI soak가 끝나기 전까지 “1주일 무인 운영 보장 완료”로 표현하지 않는다.

- **토큰 수명** — KIS가 토큰 재요청 때 이미 나이든 기존 토큰을 반환할 수 있어 `issued_at + 24h` 추정은 금지. `access_token_token_expired`/JWT `exp` 실제 만료시각을 기준으로 캐시·갱신하고, 만료 응답을 받은 조회 GET만 1회 갱신 재시도한다. 주문 POST는 접수 여부가 불명확하므로 자동 재전송하지 않는다.
- **주문 원장** — SQLite `logs/program_ledger.sqlite3`가 주문 의도·접수/거부/불명·체결·프로그램 보유수량의 단일 기준이다. 기존 `logs/program_positions.json`은 한 번만 이전하고 원본으로 보존한다. 새 엑셀 보고서는 원장 기준 `logs/trade_log_v2.xlsx`이며, 엑셀 잠금은 경고/재시도 대상이고 원장 실패는 주문 차단 대상이다.
- **OFF/정지 게이트** — 자동취소는 조회가 아니라 KIS 취소 주문 전송이다. 실주문 OFF와 정지 상태는 신규 주문과 자동취소를 모두 막고, 이미 전송된 요청의 결과 기록은 끝까지 수행한다.
- **거래시간/주기** — GUI 설정은 국내·미국 시간 제한을 따로 저장하고 한국시간 기준 자정 넘김(`22:00~06:00`)을 지원한다. 실행 중 주기 변경은 러너 대기를 깨워 다음 사이클부터 적용하며, 저장된 설정과 실행 적용 설정을 구분 표시한다.
- **GUI 바인딩/분석** — 선택 종목 편집 입력값은 YAML → `Settings` → 주문 판단까지 이어진다. 분석 조회는 자동매매 중에도 읽기 전용 차단 브로커로 실행하며, 거래량·시가총액 도넛/트리맵을 같은 탭에 동시에 표시한다.
- **검증** — 2026-09-09 기준 `uv run python -X utf8 -m pytest -q` 585 passed, `uv run basedpyright` 0 errors, `uv run ruff check src tests` all checks passed. U19는 가상 8일 장애 시나리오 통과, 실제 192시간 soak 미완료.

## 기존 MVP 핵심 자산 (2026-07-24 실주문 검증 완료분)
- KIS 스로틀 + 재시도 — `src/pjt_trader/kis.py` `_request`(EGW00201/5xx 재시도), 응답/JWT 실제 만료시각 기반 토큰 캐시(`kis_token_cache.py`).
- 호가단위 보정 — `src/pjt_trader/tick_size.py`(KRX 2023 개편 테이블), 매수 내림/매도 올림, `strategy.py`에서 적용.
- 미체결/체결 안전망 — `fill_tracker.py`(체결 감지), 타임아웃 자동취소(`orders.timeout_minutes` 기본 5분, 주문 전송으로 취급), SQLite 원장 기준 프로그램 보유분 재매수 방지.
- 과거 실주문 왕복 검증 — 삼성전자 1주 매수 253,500원 체결 → 매도 254,500원 체결(2026-07-24), legacy `logs/trade_log.xlsx` 기록. 2026-09-09 이후 보고서는 `logs/trade_log_v2.xlsx`.

## 의존 개념: [[조회전용-분석수집기-주문격리]] · [[Tk-Canvas-도넛-트리맵-시각화]] · [[원자적-설정파일-쓰기-fail-closed]] · [[Tk-테스트-안정화-및-창캡처]] · [[KIS-API-계좌포맷]]

