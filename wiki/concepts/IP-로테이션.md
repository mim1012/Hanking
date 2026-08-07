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
- **회전 대상은 IP 값이 아니라 "IP당 밀도"다(2026-08-07 naver_turafic_samsung 실측).** boss 1대의
  셀룰러 핫스팟을 병사 N대가 공유하는 구조에서, 성공률은 개별 IP의 "평판"이 아니라 **그 IP에
  동시에 몰리는 시간당 요청량**과 반비례했다: z90(병사 2대, IP당 67건/hr)=86.6%,
  z2(6대, 148건/hr)=16.2%, z1/z3/z4(7대, 172~186건/hr)=11.6~12.9%. 기기당 요청 페이스는 그룹
  무관하게 22~27건/hr로 균일했다 — 즉 각 기기는 똑같이 "정직하게" 도는데, 한 IP에 몇 대를 몰아
  넣었는지만 성공률을 갈랐다. `android_devices.current_ip`를 하트비트에 실어(전에는 컬럼만 있고
  앱이 한 번도 채운 적 없어 항상 null이었음) 그룹별 실측 IP를 얻어야 이 상관관계가 보인다.

## 배제된 접근 (dead-end)
- **IP 하나만 로테이션 — 효과 없음.** 관리자 API로 z1의 IP를 강제로 새 IP로 교체했지만
  (`device_group.state=DRAINING→ROTATING`, `rotation-start` 엔드포인트) 성공률이 16.7%→18.8%로
  사실상 무변화(표본 78→16건, 유의미하지 않은 등락). "특정 IP가 블랙리스트됐다"가 아니라 **같은
  풀(같은 통신사 셀룰러 NAT) 안에서 아무 IP로 바꿔도 밀도 문제는 그대로 재현되기 때문**으로 추정.
  로테이션은 밀도를 낮추지 않는 한 해법이 아니다.
- **"와이파이 vs 셀룰러 회선"이 원인이라는 가설 — 반증됨.** 성공률 좋은 소그룹(z90)도 병사가
  boss의 셀룰러 핫스팟에 물린 구조였다(사용자 확인, "z90(boss)이 셀룰러 핫스팟을 켜고 z90-1이
  거기 물렸다"). 회선 종류는 z1~z4와 완전히 동일 — 상수였지 변수가 아니었다. 이 축을 조사에
  넣었다가 시간을 버린 것 자체가 named failure: "제품이 다르니 원인도 다를 것"이라는 성급한
  가정으로 실측(ISP/ASN까지 대조: 전부 `AS4766 Korea Telecom`) 전에 결론부터 냈었다.

## 관련: [[봇탐지-우회-스택]] · [[project-electrone_navershopping]] · [[project-naver-traffic-android-samsung]] · [[project-naver_turafic_samsung]]
