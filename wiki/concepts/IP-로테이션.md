# IP 로테이션 (ADB 모바일 데이터 토글)

> 모바일 데이터를 껐다 켜서 통신사 IP를 교체하는 기법. 4개 프로젝트에 중복 — 통합 대상 1순위.

## 무엇 / 언제 쓰나
트래픽/수집 봇에서 IP 차단 회피를 위해 IP를 주기적으로 교체할 때.

## 정본 구현 (canonical)
- **TS 완성형** — `electrone_navershopping/ipRotation.ts` — ADB `svc data disable/enable`(`:181-198`), Windows 테더링 어댑터 감지/재시작(`getTetheringAdapter:297`, PowerShell Get-NetAdapter), auto fallback(`rotateIP:537`), 5초 주기 자동복구 데몬(`startRecoveryDaemon:89`).

## 다른 구현 / 변형
- `sellermate_naver_place_all/place-check` — 위 로직 재사용.
- **Kotlin 실기기** — `naver-traffic-android-samsung/AGENTS.md:149-156` — boss만 root `su -c "svc data disable/enable"`로 그룹 공유 IP 회전, soldier는 `PAUSE_FOR_ROTATION` 감지 후 대기. 디바이스명이 역할 인코딩(`z1`=boss, `z1-1`=soldier).
- `turafic/ADB_MONITORING_GUIDE.md` — 모니터링 가이드.

## 주의점 / 함정
- env `IP_ROTATION_METHOD` 규약이 프로젝트 간 공통 — 통합 시 유지.
- 실기기는 root(`su`) 필요, PC 테더링은 어댑터 재시작 방식.

## 관련: [[봇탐지-우회-스택]] · [[project-electrone_navershopping]] · [[project-naver-traffic-android-samsung]]
