# Tk 테스트 안정화 & 창 캡처

> pytest에서 tkinter GUI를 flaky 없이 테스트하기 위한 명시적 master 바인딩 + 모듈 스코프 root/함수 스코프 Toplevel 픽스처 + 하네스가 화면에 못 띄우는 창을 PrintWindow로 캡처하는 3종 세트.

## 무엇 / 언제 쓰나
tkinter 기반 데스크톱 앱을 pytest로 자동화 테스트하고, 에이전트 하네스에서 실행되는(화면에 실제로 보이지 않을 수 있는) Tk 창을 사람이 시각적으로 검증해야 할 때.

## 정본 구현 (canonical)
- **모든 Var에 명시적 master** — `PJT V5.0/src/pjt_trader/gui.py:138` 예: `tk.StringVar(master=self.root, value=...)`. `master`를 생략하면 `tk.StringVar`가 암묵적으로 "기본 root"(`_default_root`)를 찾는데, 여러 테스트가 여러 `Toplevel`/`Tk` 인스턴스를 오가면 어느 인터프리터에 바인딩됐는지 불명확해져 teardown 순서에 따라 flaky하게 실패한다(SESSION.md 2026-09-07 기록: "Tk 테스트 flaky(간헐 skip) 원인 = `_default_root` 의존 StringVar").
- **`autostart_refresh` 생성자 스위치** — `PJT V5.0/src/pjt_trader/gui.py:122-135` `TraderGui.__init__`: `autostart_refresh: bool = True`(기본값은 실제 앱 동작 유지) + `broker_factory: Callable[[BrokerModel], Broker] = KisClient.connect`(기본은 실 KIS 연결, 테스트에서 페이크로 교체). `:1076`에서 `self.symbols and self._autostart_refresh`로 분기해, 테스트/스모크 스크립트는 `autostart_refresh=False`로 생성 시점의 자동 조회 예약(700ms `after`)을 끄고 명시적으로 `refresh_analysis()`를 호출해 타이밍을 제어한다.
- **모듈 스코프 root + 함수 스코프 Toplevel** — `PJT V5.0/tests/conftest.py:22-43` `tk_root`(module-scope, `tk.Tk()` 생성 실패 시 `pytest.skip` — 디스플레이 없는 CI를 fail 대신 skip) + `:45-61` `gui_root`(function-scope `tk.Toplevel(tk_root)`) — 매 테스트가 독립된 위젯 트리를 갖되 Tcl 인터프리터/디스플레이 연결은 모듈당 1개만 연다. `_cancel_pending_after_jobs()`(`:10-19`)가 teardown 시 `widget.tk.call("after", "info")`로 남은 `after` 예약 작업을 전부 취소해, 느린/누락된 콜백이 다음 테스트 중 발화하는 것을 방지.
- **인프로세스 PrintWindow 캡처** — `PJT V5.0/.omx/research/smoke_gui.py:171-210` `_capture_window()`: `user32.PrintWindow(hwnd, mem, 2)`(`PW_RENDERFULLCONTENT`)로 GDI 컨텍스트에 강제 렌더링 후 BMP로 저장. 에이전트 하네스가 띄운 Tk 창은 실제 화면에 그려지지 않거나 일반 `EnumWindows` 기반 스크린캡처 도구로 안 잡히는 경우가 있어, PowerShell 스크린샷 대신 프로세스 내부에서 직접 캡처한다. 같은 파일의 `FakeBroker`(`:87-123`)와 `TraderGui(..., autostart_refresh=False, broker_factory=lambda _model: broker)`(`:135-141`) 조합으로 실 API 호출 없이 GUI를 띄워 스크린샷을 찍는다.

## 다른 구현 / 변형
- (없음 — 이 프로젝트 내 캡처/픽스처 유일 구현.)

## 주의점 / 함정
- **암묵적 master로 인한 flaky skip/실패** — `tk.StringVar()`처럼 `master`를 생략하면 여러 root/Toplevel이 공존하는 테스트 스위트에서 위젯이 이미 죽은 인터프리터에 바인딩되어 간헐적으로 `TclError`가 난다. 모든 `tk.*Var(...)` 생성에 `master=self.root`를 명시하는 것으로 고정.
- **하네스 창이 "안 보인다"는 착각** — `EnumWindows`/일반 스크린샷 도구는 에이전트 하네스가 띄운 Tk 창을 못 잡는 경우가 있는데, 이를 "창이 안 뜬 것"으로 오판하면 안 된다. `PrintWindow(..., PW_RENDERFULLCONTENT)`는 화면에 실제로 그려지지 않은 창도 강제 렌더링해 캡처할 수 있다.

## 배제된 접근 (dead-end)
- **PowerShell 외부 스크린샷 도구로 하네스 Tk 창 캡처** — 일반 화면 캡처(외부 프로세스에서 `EnumWindows`류로 창을 찾아 캡처)는 하네스가 띄운 Tk 창을 인식하지 못해 빈 화면/창 없음으로 나왔다. 하네스 세션의 창이 일반 데스크톱 열거 대상에서 벗어나 있는 것으로 추정 — 인프로세스 `PrintWindow` 캡처(`smoke_gui.py`)로 전환해 해결(산출물: `.omx/research/smoke_gui.png`, SESSION.md 2026-09-07 "검증" 절에 기록).

## 관련: [[Tk-Canvas-도넛-트리맵-시각화]] · [[조회전용-분석수집기-주문격리]]
