# C2 Project — 실행 로드맵 (2026-08-23)

> 작성: kim hun / kali
> 기본 C2 인프라: Vultr (45.32.251.5:8443), Server v1.6.0-lab-stage
> 전략: ② LAN Worm + ④ 토렌트 크랙 유통

---

## ✅ 완료

| # | 항목 | 상태 |
|---|------|:----:|
| 0 | Vultr C2 구축 + 에이전트 확장 (list_dir/ps/net_scan/lan_recon/socks) | ✅ |
| 0 | V3 crypter pipeline (crypted_final, ALL CHECKS PASSED, suspicious=0) | ✅ |
| 0 | 서버 stage payload 업데이트 (agent_worm.exe 6.9MB, 웜+영속성 포함) | ✅ |
| 0 | 서버 env.systemd 토큰 수정 (64자 풀토큰) | ✅ |
| ④ | **토렌트/크랙 유통 패키지** | ✅ |
| ④ | crack_wrapper.exe: VC++ 위장, agent 추출, C2_PCBANG+WORM_ENABLE | ✅ |
| ④ | 토렌트: `crack_builder/vcredist_x64_2025_KB20260814.torrent` (4.2KB) | ✅ |
| ④ | 크랙 패키지: `crack_builder/packages/` (2.8MB, PDF+설치방법) | ✅ |
| ② | **LAN 전파 웜 모듈** (`agent/worm.go` 470줄) | ✅ |
| ② | PC방 감지 → ARP 스캔 → SMB 445 포트 스캔 | ✅ |
| ② | 12개 기본 admin 크레덴셜 SMB 로그인 시도 | ✅ |
| ② | ADMIN$ 마운트 → agent 복사 → WMIC/schtasks 원격 실행 | ✅ |
| ② | MS17-010 (EternalBlue) 취약점 체크 | ✅ |
| ② | 5분 간격 자동 스캔 고루틴 | ✅ |
| 0 | USB/Email/ISO delivery 업데이트 | ✅ |

---

## 🔴 1순위: SYSTEM agent 복구 (호스트PC)

| 작업 | 설명 | 예상 시간 | 상태 |
|------|------|:---------:|:----:|
| 서비스 재시작 or 재부팅 | DocViewerHelper 서비스 재실행 | 대기중 | ⏸ |
| 확인 | Vultr C2 dashboard에서 SYSTEM agent online 확인 | 5분 | ⏸ |

---

## 🟡 2순위: 토렌트 유통 시작

| 작업 | 설명 | 예상 시간 | 상태 |
|------|------|:---------:|:----:|
| 토렌트 사이트 업로드 | Korean/Chinese torrent sites에 등록 | 2시간 | ❌ |
| 시드 유지 | Vultr or kali에서 시딩 | 지속 | ❌ |

**타겟 토렌트 사이트:**
- 토렌트하자 (torrenthaja.com)
- 토렌트와 (torrentwa.com)
- 토렌트홀릭 (torrentholic.com)
- 52pojie.cn (중국)
- bt.bttt.la (중국)

---

## 🟡 3순위: PC방 게임폴더 영속성

| 작업 | 설명 | 예상 시간 | 상태 |
|------|------|:---------:|:----:|
| 게임폴더 fallback 구현 | Valorant/LoL/OW2 폴더에 자기 복제 | 2시간 | ❌ |
| 배포 테스트 | Win VM → 게토 환경 시뮬레이션 | 2시간 | ❌ |

---

## 🟡 4순위: 실시간 화면 스트리밍

| 작업 | 설명 | 예상 시간 | 상태 |
|------|------|:---------:|:----:|
| stream_start/stream_frame 구현 | BitBlt + WS push | 3일 | ❌ |
| 대시보드 UI | 스트림 뷰어 추가 | 2일 | ❌ |

---

## 🔵 5순위: DeepFreeze 부트킷

| 작업 | 설명 | 예상 시간 | 상태 |
|------|------|:---------:|:----:|
| MBR 부트킷 연구 | Stage1 + Windows chainload | 2주 | ❌ |
| UEFI ESP 부트킷 | bootmgfw.efi 패치 | 2주 | ❌ |

---

## 파일 링크

```
C2 서버:          https://45.32.251.5:8443/ (Admin Token 로그인)
서버 SSH:         ssh vultr (100.89.148.16)

빌드:
  deploy/stealth_build.sh           — V3 우회 빌드 파이프라인
  crack_builder/crack_wrapper.c     — 크랙 래퍼 소스
  crack_builder/scripts/*.sh        — 토렌트 빌드

에이전트 소스:
  agent/main.go                     — 메인 (2,750줄)
  agent/pcbang.go                   — 영속성 (263줄)
  agent/worm.go                     — LAN 웜 (470줄)
  agent/xor.go                      — 문자열 난독화

산출물:
  bin/agent_worm.exe                — 웜+PC방 영속 포함, 6.9MB
  cryptor/crypted_final.exe         — V3 우회 패키징, 2.8MB
  crack_builder/vcredist_x64_2025_setup.exe  — 크랙 래퍼
  crack_builder/vcredist_x64_2025_KB20260814.torrent  — 토렌트

배포:
  delivery/usb_vcredist_x64_2025/     — USB 패키지
  delivery/email_vcredist_x64_2025/   — 이메일 피싱 (ZIP pw: install32051)
  deploy/GameLauncher_latest.iso      — ISO (MotW-free)
```