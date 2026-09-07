# Tk Canvas 도넛/트리맵 시각화

> tkinter 없이 테스트 가능한 순수 기하 레이어(정사각화 트리맵 + 도넛) + CanvasLike Protocol로 실제 Tk Canvas에 그리는 2단 분리 패턴.

## 무엇 / 언제 쓰나
섹터별 비중을 도넛/트리맵으로 보여주는 데스크톱(Tk) 대시보드를 stdlib만으로 만들 때(외부 차트 라이브러리 없이). 레이아웃 계산(순수 함수, 좌표만 리턴)과 실제 그리기(Canvas API 호출)를 분리하면, Tk 디스플레이 없는 CI 환경에서도 레이아웃 로직을 전부 단위테스트할 수 있다.

## 정본 구현 (canonical)
- **CanvasLike Protocol** — `PJT V5.0/src/pjt_trader/market_visuals.py:47-101`: `create_arc`/`create_oval`/`create_rectangle`/`create_text`/`delete`/`winfo_width`/`winfo_height`만 선언한 구조적 서브셋. 실제 `tkinter.Canvas`와 레코딩 페이크 둘 다 이 Protocol을 구조적으로 만족.
- **정사각화 트리맵(Bruls-Huizing-van Wijk)** — `squarify()`(`market_visuals.py:367-428`, 종횡비 최소화 재귀 `_squarify_recursive`/`_worst_ratio`/`_layout`/`_leftover`, `:286-364`)와 `nested_treemap()`(`:452-507`, 섹터→심볼 2단 중첩, 안쪽 박스가 padding/header를 뺀 뒤 0 이하로 붕괴하면 자식 없이 섹터 사각형만 남김). 면적은 `area_weights()`(`market_analysis.py:798-814`)로 정규화 — `None`/비양수 값은 제외하고 "제외 n종목"으로 카운트, 전량 결측이면 `([], len(items))`을 반환해 절대 면적 0으로 렌더링하지 않음.
- **도넛 각도 변환** — `market_visuals.py:9-21`(모듈 docstring) + `_to_tk_angles()`(`:195-197`): 12시 기준 시계방향(`start_deg`/`extent_deg`)을 Tk의 3시 기준 반시계 변환으로 매핑 — `tk_start = 90 - start_deg`, `tk_extent = -extent_deg`.
- **단일 세그먼트 100% oval 픽스** — `draw_donut()`(`:200-233`): 세그먼트가 1개(비중 100%)면 `create_arc(..., extent=360)` 대신 `create_oval`로 그린다 — Tk의 `create_arc`가 정확히 360도 extent에서 원을 안정적으로 렌더링하지 않는 문제 회피(주석에 "M7"으로 명시).
- **시장별 색상 규칙** — `market_analysis.py:670-674, 789-795` `change_color()`: 국장(KOREA)은 상승 빨강(`KOREA_UP=#d32f2f`)/하락 파랑(`KOREA_DOWN=#1e5bd6`), 미장(US)은 상승 연두(`US_UP=#7cb342`)/하락 빨강(`US_DOWN=#d32f2f`) — 두 시장의 상승색이 다르다(한국 관행 vs 미국 관행)는 점이 함정이 되기 쉬움. `None`/`0`은 `NEUTRAL`.
- **"제외 n종목" 처리** — `area_weights()`(`market_analysis.py:798-814`)와 `area_metric_missing_count()`(`:857-867`)가 시각화 이전 단계에서 결측치를 분리해, 트리맵/도넛이 "값 없음"을 면적 0인 타일로 그리는 대신 사용자에게 별도 카운트로 보고.

## 다른 구현 / 변형
- 레코딩 페이크 캔버스 — `PJT V5.0/tests/test_market_visuals.py:27` `RecordingCanvas`(`create_arc`/`create_oval` 등을 실제로 렌더링하지 않고 호출 인자만 기록) — `CanvasLike`가 구조적 타이핑이라 실제 `tkinter` import 없이 전체 draw 로직을 테스트.

## 주의점 / 함정
- **각도 컨벤션 불일치** — 도메인이 "12시·시계방향"으로 각도를 다루는데 Tk `create_arc`는 "3시·반시계"라서, 변환식 없이 그대로 넘기면 파이가 반대 방향으로 돌거나 시작점이 어긋난다. 반드시 `_to_tk_angles`를 통과시킬 것.
- **360도 단일 아크 렌더링 실패** — `create_arc(extent=360)`은 일부 Tk 빌드/DPI 조합에서 원이 보이지 않거나 깨져 그려진다. 세그먼트가 1개뿐이면 무조건 `create_oval`로 분기.
- **KRW/USD 면적 합산 금지** — `market_analysis.py:823-839` `filter_summaries_by_market()`가 시장이 섞인 summaries에서 `market=None`으로 호출되면 `ValueError`를 던진다(H2) — 국장 시가총액(원)과 미장 시가총액(달러)을 같은 트리맵 면적 축척으로 섞으면 안 되기 때문. [[조회전용-분석수집기-주문격리]]가 다루는 `market_cap` 스케일 문제(억원 vs 원시 USD)와 같은 계열의 실수.

## 배제된 접근 (dead-end)
- **`create_arc(extent=360)`로 단일 세그먼트 원 그리기** — 처음 구현은 세그먼트 수와 무관하게 `create_arc`만 썼으나, 100% 비중(세그먼트 1개) 케이스에서 일부 환경 렌더 결과가 빈 원/깨진 원으로 나와 폐기. `len(segments) == 1` 분기로 `create_oval`을 쓰도록 수정(검증: `PJT V5.0/tests/test_market_visuals.py:314` `test_draw_donut_single_segment_uses_full_oval_not_360_degree_arc` 통과, 2026-09-07 172개 관련 테스트 전체 통과 확인).

## 관련: [[조회전용-분석수집기-주문격리]] · [[Tk-테스트-안정화-및-창캡처]]
