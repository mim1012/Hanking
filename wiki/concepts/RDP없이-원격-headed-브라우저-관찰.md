# RDP없이-원격-headed-브라우저-관찰
> 원격 Windows PC에서 도는 headed 브라우저 자동화를, RDP로 세션을 뺏지 않고 SSH 스크린샷으로 관찰하는 패턴.

## 무엇 / 언제 쓰나
원격 PC 함대에서 headed 브라우저(트래픽/순위/검증 등)를 돌리면서 "지금 뭐 하는지 눈으로 보고 싶을" 때. RDP로 붙으면 콘솔 세션을 가상세션으로 뺏어가 automation이 끊기므로, 대신 **autologon 콘솔 세션을 상시 Active로 유지 + tscon으로 콘솔 복귀 + SSH로 스크린샷 왕복**한다. 원격 Windows PC를 쓰는 프로젝트(트래픽 turafic / 순위 sellermate / 파이프라인 zero-landing) 공통.

## 정본 구현 (canonical)
- `turafic_update/scripts/fleet-attach.mjs` — SSH ops 툴(정본 인벤토리 파싱 + plink/hostkey). 서브커맨드 status/console/shot/watch/run/push.
  - `:65 adminSession` — **qwinsta**로 세션 판정(query user는 세션 없으면 exit1 오판 → qwinsta는 항상 exit0).
  - `:84 ensureConsole` — 세션이 콘솔 밖(Disc/RDP)이면 `tscon <id> /dest:console`로 콘솔 복귀(automation 생존 전제).
  - `:95 deployShotScript` `:103 shot` `:146/:149 shot/watch` — /IT 스케줄태스크로 콘솔세션에서 캡처 → pscp로 로컬 왕복.
- `turafic_update/scripts/take-screenshot.ps1` — 콘솔(인터랙티브) 세션에서 CopyFromScreen(SSH 세션0에선 빈화면이라 /IT 필수).
- 상세 함정·실측: turafic 메모리 `fleet-attach-screenshot-observe`, `ssh-interactive-headed-launch`.

## 다른 구현 / 변형
- `turafic_update/scripts/nocdp-remote-launch.ps1` — headed 기동+폴링+회수 올인원(Register-ScheduledTask Interactive principal). 관찰이 아니라 기동용.
- 실시간으로 눈으로 볼 땐 **VNC/AnyDesk(콘솔 미러링)** 또는 **RDP 그림자(`mstsc /shadow`)** — 세션을 뺏지 않고 화면만 비춰 automation 안 끊김.

## 주의점 / 함정
- **「RDP 세션 뺏김 = automation 끊김」**: RDP 접속은 콘솔 세션을 가상세션으로 끌어오고, 창 닫으면 Disc(Disconnected)로 떨궈 포커스가 사라진다 → OS레벨 SendKeys(nocdp UIA)가 무효 → **100% MID_NOT_FOUND**. 반드시 tscon으로 콘솔 복귀.
- pscp **원격경로는 슬래시**(`C:/...`) — 백슬래시는 다운로드 시 먹혀 사라짐.
- 로컬 sleep은 `Atomics.wait`(cmd `timeout /t`는 stdin 리디렉션 불가로 죽음).
- node_modules robocopy **완료 전 재부팅 금지** → tsx/dist/cli.mjs 누락으로 MODULE_NOT_FOUND.
- **hostname은 노드 식별 불가**(클론 이미지가 이름 공유, 예 XEN-12/13/16/18 = DESKTOP-M57RQ61) — 텔레메트리 구분은 다른 축으로.

## 배제된 접근 (dead-end)
- **RDP로 직접 관찰**: 세션을 뺏고 창 닫으면 Disc → automation 끊김. 잠깐 봐도 나올 때 tscon 복귀 필수라 근본적으로 관찰 도구로 부적합.
- **window occlusion 플래그로 클릭 고치기**: CDP `Input.dispatchMouseEvent`는 렌더러로 직접 가서 **OS 창 가시성/포커스와 무관** → 콘솔창이 크롬을 덮어도 CDP 클릭엔 영향 없다. `--disable-features=CalculateNativeWinOcclusion`은 렌더 스로틀만 막음(클릭 무관). "가려짐"의 진짜 원인은 DOM 오버레이(turafic 메모리 `click-suggest-overlay-not-window-occlusion`).

## 관련: [[CDP없는-OS레벨-브라우저자동화]] [[Windows예약작업-IT트리거-로그오프함정]] [[크롬창-오클루전-렌더차단]] [[SSH-공개키인증-Win32버그-우회]] [[project-turafic_update]]
