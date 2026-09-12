# 러너-인스턴스락-PID재사용

> PID를 적어 두는 인스턴스 락 파일은 종료 시 반드시 지우고, 남은 락의 PID가 살아 있어도 "그 프로세스가 정말 우리 러너인지" 커맨드라인까지 확인해야 한다. Windows는 PID를 수 분 안에 재사용한다.

## 무엇 / 언제 쓰나
- 단일 인스턴스 데몬(호스트당 러너 1개)을 락 파일+PID로 보장하는 모든 곳. 특히 감독 프로세스(FleetAgent)가 자식을 재기동하는 구조에서 치명적 — 기동 거부가 "조용한 노드 정지"로 이어진다.
- 2026-09-12 로컬 재현: 큐모드 러너가 정상 종료(`process.exit(0)`)한 뒤 락 파일이 남고, 3분 뒤 `conhost.exe`가 같은 PID를 받아 다음 기동이 "동일 호스트 러너 이미 실행 중"으로 종료됨. 수정 fa312e1c는 XEN-20 개통 빌드에 포함돼 실기동 확인.

## 정본 구현 (canonical)
- `turafic_update/scripts/strategy-a-nocdp-runner.ts:197` `releaseInstanceLock` — fd 닫기 + **락 파일 unlink**(우리가 만든 락이므로 삭제).
- `…/strategy-a-nocdp-runner.ts:208` `isRunnerProcess` — `Get-CimInstance Win32_Process -Filter ProcessId=<pid>` 커맨드라인에 러너 스크립트명이 있는지 확인. 조회 실패 시 보수적으로 "살아있음"(중복 기동보다 기동 거부가 안전).
- `…/strategy-a-nocdp-runner.ts:281` `acquireInstanceLock` — `wx`로 생성 시도 → EEXIST면 PID 읽기 → `isProcessRunning && isRunnerProcess`일 때만 거부, 아니면 rename→unlink로 stale 회수(최대 4회).

## 다른 구현 / 변형
- 강제 종료(taskkill /T /F)로 죽으면 exit 핸들러가 안 돌아 락이 남는다 → 다음 기동의 커맨드라인 검사가 유일한 방어. 운영 스크립트가 러너를 강제 종료했다면 락 파일(`C:\temp\strategy-a-nocdp-<host>.lock`)을 수동 삭제.
- 고아 정리(`killOrphanedSiblingRunners`)는 별도 층: 자식 마커(`--dispatch-child=<parent pid>`)로만 매칭해 운영자가 손으로 띄운 같은 스크립트를 죽이지 않는다(`:259`).

## 주의점 / 함정
- **"PID 재사용 오판"** — `process.kill(pid, 0)`은 "어떤 프로세스든 존재"만 답한다. 존재 검사만으로 락 유효성을 판단하면 재사용된 PID에 걸린다.
- 감독 프로세스가 5초 뒤 재기동하는 정상 경로에서는 재사용 확률이 낮아 오래 숨어 있다가, 6시간 주기 재시작·크래시·수동 kill 뒤에만 튀어나온다.

## 배제된 접근 (dead-end)
- **종료 시 fd만 닫고 파일은 남기기(기존 구현)** — 락 파일이 영구 잔존, PID 재사용 시 기동 거부.
- **재시도 루프/대기로 회피** — 재사용된 PID의 프로세스(conhost 등)가 살아 있는 한 영원히 거부되므로 대기는 무의미.

## 관련: [[전략인지-큐-노드-capability-게이트]] · [[앱-자동업데이트-채널]] · [[project-turafic_update]]
