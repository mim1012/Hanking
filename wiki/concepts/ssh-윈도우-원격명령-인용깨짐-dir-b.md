# ssh-윈도우-원격명령-인용깨짐-dir-b

> PowerShell→OpenSSH→Windows 원격에서 `ssh host cmd /c dir X /b`류 명령이 `매개 변수 형식이 틀립니다 - "b""`처럼 인용이 깨져 실패한다 — 인자에 따옴표·특수문자가 없는 명령만 직접 쓰고, 복잡한 작업은 스크립트 파일을 scp한 뒤 `powershell -File 경로 -Param 값`(공백 없는 토큰)으로 실행한다.

## 무엇 / 언제 쓰나

PowerShell에서 `ssh remotehost <command>` 형태로 Windows 원격 명령을 보낼 때, 그 명령 문자열이 SSH 클라이언트→sshd→cmd.exe/PowerShell을 거치며 인용 규칙이 계층마다 다르게 해석되어 스위치나 중첩 따옴표가 깨지는 상황에서 쓴다. 여러 대(예: XEN-12~16)에 같은 예약작업을 원격 등록할 때 특히 재현된다.

## 정본 구현 (canonical)

- `D:\Project\zero-landing\tools\setup-collector-task.ps1:1-32` — 원격 실행 전용 등록 스크립트. 인자를 공백 없는 토큰(`-WorkerId XEN-12`)만 쓰도록 설계했고, 재등록 순서는 `Stop-ScheduledTask` → `Unregister-ScheduledTask` → `Register-ScheduledTask` → (`-Run`이면) `Start-ScheduledTask`. Principal은 `admin`/`S4U`/`Highest`, 트리거는 `AtStartup`.
- `D:\Project\zero-landing\tools\run-forever.bat:1-8` — 크래시 시 10초 후 재기동하는 배치 래퍼. `cd /d "%~dp0\.."`로 스크립트 위치 기준 레포 루트로 이동한 뒤 인자로 받은 명령을 `:loop`에서 반복 실행한다.

검증된 성공: 2026-09-16 XEN-12~16 5대에 `setup-collector-task.ps1`을 scp한 뒤 `ssh host powershell -NoProfile -ExecutionPolicy Bypass -File C:\keyword-pipeline\tools\setup-collector-task.ps1 -WorkerId XEN-12 -Run` 형태(공백 없는 토큰만)로 실행해 5대 전부 `state=Running`을 확인했고, 이후 `keyword_pipeline_events` 하트비트에서 cycle 진행을 확인했다. 배포 확인은 `ssh host dir C:\path`(스위치 없이) + 로컬 `Select-String`, 프로세스 확인은 `ssh host tasklist` + 로컬 필터, 종료는 `taskkill /f /im node.exe`, 예약작업 조회는 `schtasks /query /fo csv /v` + 로컬 파싱으로 전부 스위치·따옴표 처리를 로컬 쪽으로 옮겼다.

## 다른 구현 / 변형

- `D:\Project\zero-landing\tools\setup-node.ps1:56-66` — `kw-collector` 예약작업의 원본 정의. `setup-collector-task.ps1`이 여기서 등록 로직을 그대로 추출했지만, 이쪽은 `$Role -eq 'master'`일 때만 실행되는 노드 최초 셋업(RDP 관리자 PowerShell 전제)의 일부라서 master 역할 노드에만 등록되고 worker 노드에는 없다. `setup-collector-task.ps1`은 이 제약을 풀어 아무 노드나 SSH만으로 단독 등록할 수 있게 만든 변형이다.

## 주의점 / 함정

- **"`/b` 스위치 인용 변형"** — `dir /b`처럼 슬래시로 시작하는 스위치가 `ssh` 경유 시 `/b"`처럼 따옴표가 붙어 도착해 `매개 변수 형식이 틀립니다` 에러가 난다.
- **"골든이미지 hostname 동일"** — 여러 대가 같은 골든 이미지에서 복제되어 hostname이 전부 같으면(`DESKTOP-M57RQ61` 등) `$env:COMPUTERNAME` 기본값을 워커 식별자로 쓸 수 없다. worker-id는 반드시 스크립트 인자로 명시해야 한다.
- **"Unregister만 하면 고아 프로세스 → 이중 기동"** — 재등록 전에 `Stop-ScheduledTask`를 먼저 하지 않으면 기존 프로세스가 고아로 남아, 재등록된 작업과 함께 두 개가 동시에 돈다.
- **"원격 콘솔 출력 cp949 mojibake"** — 원격 콘솔 출력의 한글이 깨져 오면 로컬에서 `-replace '[^\x20-\x7E]'`처럼 비-ASCII를 걸러내고 읽는다.

## 배제된 접근 (dead-end)

- `ssh host powershell -Command "Get-CimInstance … -Filter \"name='node.exe'\""`처럼 중첩 따옴표를 쓰는 명령 — 계층(SSH 클라이언트/서버, cmd, PowerShell)마다 따옴표가 다르게 벗겨져 실패한다.
- `ssh host cmd /c "a & b"`처럼 `&`로 명령을 체이닝 — 체이닝 자체에 따옴표가 필요해 같은 문제가 재현된다. 대신 명령을 별도의 `ssh` 호출 여러 번으로 나눈다.

## 관련

[[Windows예약작업-IT트리거-로그오프함정]] · [[SSH-공개키인증-Win32버그-우회]] · [[RDP없이-원격-headed-브라우저-관찰]]
