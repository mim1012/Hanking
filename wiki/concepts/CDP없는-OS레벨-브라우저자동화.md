# CDP 없는 OS 레벨 브라우저 자동화 (Win32 + UIA)

> CDP(Chrome DevTools Protocol) 부착 자체가 탐지 지점일 때, Playwright/Patchright를 버리고
> 시스템 Chrome을 그냥 띄운 뒤 UI Automation 트리 조회 + Win32 마우스 입력으로 조작하는 패턴.

## 무엇 / 언제 쓰나

봇 탐지를 뚫으려고 헤더·지문·프록시·동시성을 다 바꿔봤는데 차단이 그대로일 때. 그때 남은
변수는 **CDP를 붙였다는 사실 자체**다. Playwright/Patchright/Selenium은 전부 CDP(또는 그
파생 프로토콜)로 브라우저를 몰기 때문에, 페이지 안에서 무엇을 위장해도 부착 자체는 남는다.

이 패턴은 DOM 조회를 **Windows UI Automation 트리**로, 클릭을 **`SetCursorPos` + 실제 마우스
이벤트**로 대체한다. 브라우저 입장에선 사람이 창을 보고 클릭한 것과 구분되지 않는다.
대가는 headed 전제(실제 창이 떠 있어야 함)와 느린 속도(시도당 60초 내외)다.

## 정본 구현 (canonical)

(2026-08-20 시점 `turafic_update` 브랜치 `zen-nocdp-fleet` / `investigate-strategy-a-pc-migration`.
main 머지 전까지 lint가 file missing으로 잡는다.)

- **요소 탐색 — DOM querySelectorAll 대체** — `turafic_update/engines/pc-strategy-a-nocdp/nocdp-click-engine.ps1:75`
  `Find-LinkByMid`. UIA 트리에서 `ControlType.Hyperlink`를 `TreeScope.Descendants`로 전부 긁어
  각 요소의 `ValuePattern` URL(실패 시 `Name`)에 목표 식별자 문자열이 있는지 검사(`:79-87`).
  화면 밖 요소도 트리에는 있으므로 `querySelectorAll`과 동등한 커버리지가 나온다.
- **스크롤 — el.scrollIntoView() 대체** — `:137` `Move-ElementIntoView`. `ScrollItemPattern.ScrollIntoView()`
  를 먼저 시도(`:142`)하고, 애니메이션 완료를 300ms×4회 재확인(`:152-166`). 미지원이면
  좌표 비교 휠 스크롤 최대 40회로 폴백(`:185-206`).
- **클릭 — 좌표는 요소에서 역산** — `BoundingRectangle` 중심 `X+Width/2, Y+Height/2`(`:162-163`)가
  뷰포트 안(위 100px·아래 80px 여백 제외)이면 `[Win32Input]::Click`(`:30` `SetCursorPos`+이벤트).
- **좌표 클릭이 불가능한 경우** — 가로 캐러셀에 갇혀 세로만 뷰포트 안일 때는 좌표를 포기하고
  `InvokePattern.Invoke()`로 좌표 없이 호출(`:172`, `:211` `Try-InvokeClick`). 페이지네이션
  버튼도 같은 이유로 InvokePattern 우선(`:115` `Click-NextButton`).
- **호출부** — `turafic_update/scripts/strategy-a-nocdp-runner.ts`. Node에 UIA 바인딩이 없어
  PowerShell 엔진을 자식 프로세스로 띄우고 마지막 stdout 줄로 결과를 받는다. DB 계약은
  CDP 러너와 동일(`:278` `claim_strategy_slot`, `:355` `report_strategy_slot`)이라 러너만
  갈아끼울 수 있다.

## 다른 구현 / 변형

- CDP를 유지한 채 지문만 위장하는 계열 → [[봇탐지-우회-스택]]. Cloudflare·쿠팡처럼 **부착
  자체를 안 보는** 대상에는 여전히 그쪽이 싸고 빠르다. 이 페이지의 패턴은 부착을 보는
  대상(2026-08 기준 네이버 쇼핑) 전용이다.
- 하이브리드("탐색은 CDP로, 최종 클릭만 OS 레벨로")는 검토했으나 CDP 세션이 계속 붙어 있어
  의미가 없다고 판단해 채택하지 않았다.

## 주의점 / 함정

- **"CDP 부착이 탐지 지점"** — 이 패턴이 회피하는 실패 패턴. 탐지가 페이지 내용이 아니라
  부착 여부에 걸리면, 페이지 쪽 변수(UA/Client Hints/navigator 지문/스크롤 방식/프록시/
  콜드 식별자)를 **무엇을 바꿔도 결과가 동일**하다. "다 바꿔봤는데 전부 똑같이 막힌다"가
  이 패턴의 진단 신호다 — 변수가 부족한 게 아니라 변수 축이 틀린 것이다.
  실측(`turafic_update/docs/experiment/2026-08-19-strategy-a-429-root-cause.md` §5-A):
  noCDP 호스트 287건 중 RATE_LIMITED 0건 vs 같은 시간·같은 작업 풀을 CDP로 돈 호스트
  3,000~4,500건.
- **규모에서 재확인 (2026-08-23 함대 승격 번아웃)** — cdp 러너(matched 헤더 포함)를 프로덕션
  40노드로 승격하니 차단 1%→33%→73%→**100%(15분)**, 성공률 0%로 붕괴 → 즉시 롤백. 같은 함대
  규모의 nocdp는 RATE_LIMITED 0%. 근거 주석: `turafic_update/scripts/strategy-a-nocdp-runner.ts:9-13`.
  교훈: **matched 헤더는 저부하·단시간에 증상만 가린다** — CDP 서명은 남아 규모에서 재발. 소수(2노드
  5%)를 함대 대표로 착각해 확장하면 안 됨(단계적 승격 필수). 상세:
  `turafic_update/docs/experiment/2026-08-23-session-retro-cdp-promotion-burn-rollback.md`.
- **headed 전제 + 창 가림** — 실제 창이 렌더돼야 UIA 트리에 요소가 올라온다. 콘솔 창이
  Chrome을 가리거나 세션이 잠기면 트리가 비어 "요소 없음"으로 조용히 실패한다. Chrome의
  occlusion 최적화 비활성화 플래그 + `SetForegroundWindow`가 필수. `--headless`는 성립하지 않는다.
- **"요소 없음"과 "렌더 안 됨"이 같은 증상** — 둘 다 탐색 시간을 꽉 채우고 not-found로
  끝난다. 엔진이 스크롤 시도마다 트리의 총 링크 수(`totalLinks`)를 로그에 남기게 해야
  구분된다(`:486`). 0이면 렌더/가림, 0이 아닌데 못 찾으면 매칭 문자열 문제.
- **탐색 범위가 곧 성공 상한** — `MAX_SCROLL`/캐러셀 페이지 수(`:476-477`) 밖의 요소는
  트리에 아예 안 뜬다. 대상이 상위 노출된다는 전제가 깨지면 이 패턴은 못 찾는다.
- PowerShell 엔진은 한글 리터럴을 바이트 배열로 들고 있다가 조립한다(`:48-55`) — 콘솔
  코드페이지에 따라 문자열 비교가 깨지는 걸 피하기 위한 것이므로 그대로 둘 것.

## 배제된 접근 (dead-end)

- **CDP-Patches 등으로 CDP를 감추기** — 막다른 길. 부착 흔적을 지우는 방향으로는 안 풀렸다.
- **`addInitScript`로 navigator 지문 교정** — Patchright(1.57.0)가 `Runtime.enable` 탐지 회피를
  위해 주입 스크립트를 격리 컨텍스트에서 돌려 **애초에 실행조차 안 됐다**. CDP 저수준
  (`Network.setUserAgentOverride` + `Emulation.setDeviceMetricsOverride`)으로 실제 교정에
  성공했으나 차단 결과는 무변화.
- **동시성(롤아웃%) 하향** — 25%→100% 전환 시점에 성공률이 무너진 상관관계가 있어 원인으로
  지목했으나, 25%로 되돌려도 회복되지 않았다. 상관은 실재했지만 인과가 아니었다(동시성이
  오르면 CDP 세션 노출량도 같이 는다).
- **헤더/UA/Client Hints 버전 롤백** — 4개 커밋 시점을 각각 격리 브랜치로 복원해 실행,
  전부 동일하게 차단. 헤더와 성공률 사이에 상관관계 없음.
- **matched 헤더 cdp를 프로덕션 함대로 승격** — 막다른 길(2026-08-23). 정합 헤더(`--header-mode=matched`)로
  저부하 차단을 0.7~5%까지 눌렀지만, 40노드 규모에선 CDP 서명이 다시 잡혀 15분 만에 100% 차단.
  "성공률 높음"에 혹해 CDP로 회귀하지 말 것 — 규모 안전 베이스는 nocdp. (단, 오늘 테스트는 CDP서명과
  상세물량 7배가 동시라 완전 분리 못 함 — 결정적 실험=nocdp를 동일 상세물량까지 올려도 429=0인지.)

## 관련: [[봇탐지-우회-스택]] · [[사람행동-시뮬레이션]] · [[유입수-집계되는-진입]] · [[project-turafic_update]]
