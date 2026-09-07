# 원자적 설정파일 쓰기 & Fail-Closed 파싱

> 설정 YAML을 mkstemp+fsync+os.replace로 원자적으로 쓰고, 손상된 기존 파일을 만나면 기본값으로 조용히 재생성하는 대신 예외를 던져 데이터 유실을 막는 패턴.

## 무엇 / 언제 쓰나
GUI 토글 하나로 전체 설정 파일(수십~수백 줄, 여러 섹션)을 다시 쓰는 앱에서. 원자적 쓰기가 없으면 프로세스가 쓰기 도중 죽었을 때 파일이 잘려 남고, fail-closed 파싱이 없으면 "파일이 이상하니 기본값으로 새로 만들자"는 방어적 코드가 실제로는 사용자의 축적된 설정(주문 타임아웃, 100개 종목 규칙 등)을 통째로 날린다.

## 정본 구현 (canonical)
- **공용 원자적 쓰기 헬퍼** — `PJT V5.0/src/pjt_trader/atomic_write.py:18-37` `atomic_write_text()`: `tempfile.mkstemp(dir=path.parent)`로 같은 디렉터리에 임시파일 생성 → `fh.flush()` + `os.fsync(fh.fileno())` → `tmp_path.replace(path)`. `os.replace`가 POSIX/Windows 모두에서 원자적이라, 읽는 쪽은 항상 "완전한 옛 파일" 또는 "완전한 새 파일"만 본다. 예외 발생 시 `tmp_path.unlink(missing_ok=True)`로 임시파일을 청소한 뒤 재raise — `path`는 절대 건드리지 않음.
- **두 소비자** — `PJT V5.0/src/pjt_trader/setup_wizard.py:97-115` `write_config()`(settings.yaml, ~100개 심볼 규칙)와 `PJT V5.0/src/pjt_trader/config.py:283-299` `save_sector_map()`(sectors.yaml)이 동일 헬퍼를 재사용.
- **Fail-closed 원본 YAML 읽기** — `PJT V5.0/src/pjt_trader/setup_wizard.py:131-150` `_read_raw_yaml()`: 파일이 없으면 `None`(정당한 "설정 없음", 기본값 적용). 파일이 있지만 파싱 실패(`yaml.YAMLError`) 또는 매핑이 아닌 값(빈 문서, 잘린 쓰기로 인한 스칼라 등)으로 파싱되면 `ConfigError`를 던진다(`:147`, `:149`) — "파일 이상하니 기본값으로 재생성"을 절대 하지 않는다.
- **non-`symbols` 섹션 보존** — `PJT V5.0/src/pjt_trader/setup_wizard.py:70-94` `build_config_yaml()`: `existing` dict가 주어지면 `quote_polling`/`trading_window`/`broker`/`logging`/`orders` 등 모든 최상위 섹션을 원본 그대로 유지하고 `symbols`만 교체(`:93`). `polling` 인자가 없으면 기존 `quote_polling`도 그대로 둔다(`:89-92`).

## 다른 구현 / 변형
- (없음 — 이 프로젝트 내 유일한 원자적 쓰기 구현. 재사용처는 `write_config`/`save_sector_map` 2곳.)

## 주의점 / 함정
- **"전체 재생성" 함정(리뷰에서 지적)** — GUI가 심볼 토글 하나 바꿀 때마다 설정 파일 전체를 기본값 템플릿으로 재작성하면, `buy_interval_seconds`/`sell_interval_seconds`/`orders.timeout_minutes`처럼 GUI가 편집 대상으로 알지 못하는 섹션이 조용히 사라진다. `write_config`가 `_read_raw_yaml`로 기존 파일을 먼저 읽어 `existing`으로 넘기지 않으면 이 문제가 재발한다.
- **손상 파일을 "파일 없음"과 동일시하는 함정** — 잘린 쓰기(mid-write 크래시로 파일이 스칼라/빈 문서가 됨)를 `None`(파일 없음)과 같게 처리하면, 다음 저장 시 기본값 위에 새 심볼만 얹어 기존 100개 규칙이 소리 없이 사라진다. `_read_raw_yaml`은 이 두 상태(없음 vs 손상)를 명확히 구분해 후자만 예외로 승격한다.

## 배제된 접근 (dead-end)
- **손상/파싱실패 시 기본값으로 폴백** — 리뷰 지적(CRITICAL): 기존 방식은 GUI가 매 토글마다 settings.yaml 전체를 비원자적으로 재생성하고, 손상 파일을 만나면 defaults로 조용히 리셋했다 — `buy_interval_seconds`/`orders.timeout_minutes` 등 GUI가 모르는 섹션이 사라지는 실사고 경로. `_read_raw_yaml`을 fail-closed(`ConfigError`)로 바꾸고 `write_config`가 그 예외를 그대로 전파해 "저장 실패를 사용자에게 보여주고 원본 파일은 그대로 둔다"로 교체(검증: `tests/test_setup_wizard.py`의 `test_write_config_preserves_us_exchange`, `test_write_config_preserves_buy_sell_interval_and_orders_when_polling_not_given` 통과, 2026-09-07 172개 관련 테스트 전체 통과 확인).

## 관련: [[조회전용-분석수집기-주문격리]]
