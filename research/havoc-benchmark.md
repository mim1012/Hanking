# Havoc C2 벤치마크 조사 — 자체 c2-project 대비

> **작성일**: 2026-07-31  
> **최종 갱신**: 2026-07-31 (v1.4 B-H7/B-H8 반영)  
> **목적**: 오픈소스 Havoc Framework를 조사해 자체 랩 C2(`outputs/c2-project`)에 가져올 설계 포인트만 정리  
> **범위**: 아키텍처·프로토콜·에이전트·리스너·영속/로더 — **코드 이식/악용 가이드 아님**  
> **출처 (교차 검증)**:  
> - [HavocFramework/Havoc](https://github.com/HavocFramework/Havoc) (★~8507, main)  
> - README / profiles (`havoc.yaotl`, `http_smb.yaotl`, `webhook_example.yaotl`)  
> - [havocframework.com/docs](https://havocframework.com/docs) — listeners / agent / profiles / teamserver  
> - Demon README (`payloads/Demon/README.md`), teamserver `pkg/` 레이아웃  
> - DeepWiki architecture 요약 (2차 참고)

---

## 0. 한 줄 결론 (2026-07-31 / v1.3)

| | |
|--|--|
| **Havoc** | Go Teamserver + Qt Client + **C/ASM Demon**. HTTP(S) malleable beacon 중심, 잡 큐·SQLite·멀티 오퍼레이터·풍부한 포스트익스. |
| **c2-project** | 가벼운 Go 듀얼 트랜스포트(WSS+HTTP beacon) + **DocViewer 위장 배포 + WMI 상주**. |
| **이미 흡수 (✅)** | B-H1~H6: job queue · stable ID · kill date · profile 분리 · agent/admin 토큰 분리 · HTTP beacon · 파일 청크 up/down · state.json |
| **다음 벤치 흡수 (⬜)** | **B-H7** URI/UA/Header malleable · **B-H8** 앱 레벨 암호 · (중기) loader/shellcode · sleep/syscall | 부분→**완료(v1.4 URI/UA/Hdr)** | lab malleable (not full transform) | MSI/PDF 위장 체인 + WMI Event 영속 — Havoc 표준 아님, **교체하지 않음** |
| **전략** | Havoc 바이너리 병행/교체 ❌ → **패턴만 흡수 (옵션 A)** |

---

## 1. Havoc이 뭔가

| 항목 | 내용 | 출처 |
|------|------|------|
| 이름 | **Havoc** — modern malleable post-exploitation C2 | README |
| 제작 | Paul Ungur (**@C5pider**) | README |
| 라이선스 | 공개 GitHub | repo |
| 스타 | ~8507 (2026-07-31 관측) | GitHub API |
| 포지션 | Cobalt Strike 계열 대안의 **오픈소스 풀스택** | docs/커뮤니티 |
| 철학 | **탐지 회피 완제품 아님**. malleable·modular. README: *do not open detection issues* | README Note |

관련 org 확장:

| 저장소 | 역할 |
|--------|------|
| **Talon** | 커스텀/서비스 에이전트 |
| **havoc-py** | Python API |
| **Modules** | 모듈 확장 |

---

## 2. 3단 아키텍처 (공식)

```
┌─────────────┐   WebSocket          ┌────────────────┐
│  Client     │ ◄──────────────────► │  Teamserver    │
│  C++ / Qt   │   기본 :40056         │  Go + SQLite   │
│  Dracula UI │   Operators auth      │  listeners /   │
└─────────────┘                      │  jobs / build  │
                                     └───────┬────────┘
                                             │ HTTP(S) POST
                                             │ SMB named pipe
                                             │ External C2
                                             ▼
                                     ┌────────────────┐
                                     │  Demon Agent   │
                                     │  C + ASM       │
                                     └────────────────┘
```

| 구성요소 | 언어 | 역할 | 출처 |
|----------|------|------|------|
| **Teamserver** | Go | 리스너, 에이전트/잡, 오퍼레이터 명령, 페이로드 생성(MinGW), External C2, webhook | README, docs/teamserver, `teamserver/pkg/*` |
| **Client** | C++/Qt | 멀티유저 GUI, 리스너·페이로드·세션 콘솔 | README |
| **Demon** | C + ASM | 플래그십 임플란트 — beacon, sleep obfuscation, inject, token, pivot… | README, docs/agent |

### Teamserver 패키지 레이아웃 (관측)

```
teamserver/pkg/
  agent/  db/  events/  handlers/  (http.go, smb.go, external.go)
  profile/  service/  socks/  webhook/  packager/  win32/ …
```

DB 개념 테이블: `agents`, `links`, `listeners` (`pkg/db/`).

### Demon 소스 레이아웃 (관측)

```
payloads/Demon/src/
  asm/     — return address / stack spoofing
  core/    — connect, dynamic Win32/API·syscall
  crypt/   — encrypt/decrypt
  inject/  — injection utils
  main/    — MainExe.c / MainDll.c / MainSvc.c
```

사전 산출물: `payloads/Shellcode.x64.bin`, `DllLdr.x64.bin` 등.

### 우리 프로젝트와 구조 대비 (v1.3)

| | Havoc | c2-project **v1.3** |
|--|-------|---------------------|
| 오퍼레이터 UI | Qt 멀티유저 Client | 없음 — `/admin/*` JSON API |
| 컨트롤 플레인 | Teamserver WS :40056 | 서버 하나 (agent+admin 동일 TLS 포트, **토큰 분리**) |
| 데이터 플레인 | HTTP(S) / SMB / External | **WSS `/agent` + HTTP POST `/beacon`** (B-H6 ✅) |
| 임플란트 | Demon (C/ASM) | Go `agent` (linux + PE) |
| 상태 저장 | SQLite | **`DATA_DIR/state.json`** (agents+jobs) ✅ |
| 잡 큐 | 폴링 시까지 적재 | **agent별 queue + flush** ✅ |
| 페이로드 생성 | Teamserver MinGW 즉석 빌드 | `build.sh` + MSI/WXS 스테이지 |
| 프로파일 | `*.yaotl` (HCL fork) | `profiles/*.profile.env` + `config.lab.env` ✅ |
| 파일 전송 | download/upload 명령 | **청크 up/down + admin file API** ✅ |

---

## 3. 리스너 · 통신 모델

### 3.1 Havoc 리스너 타입 (docs/listeners)

| 타입 | Demon | Service | 통신 |
|------|:-----:|:-------:|------|
| **HTTP/HTTPS** | ✅ | ✅ | **POST** check-in / task / result |
| **SMB** | ❌ (피벗 경로) | ✅ | Named pipe P2P |
| **External C2** | ✅ | ✅ | 3rd-party → teamserver HTTP 엔드포인트 |

### 3.2 HTTP 프로파일 키 (docs/profiles + `http_smb.yaotl` 실측)

| 키 | 의미 | 예시 (http_smb.yaotl) |
|----|------|------------------------|
| `Name` | 리스너 이름 | `teams profile - http` |
| `Hosts[]` | 콜백 호스트 | `5pider.net` |
| `HostBind` / `PortBind` / `PortConn` | 바인드·접속 포트 | `0.0.0.0:443` |
| `HostRotation` | `random` \| `round-robin` | `round-robin` |
| `Secure` | TLS | 예시는 false(디버그), 운영 true 권장 |
| `KillDate` | 종료 시각 UTC | `2024-01-02 12:00:00` |
| `UserAgent` | UA 위장 | Chrome 96 Windows |
| `Uris[]` | 콜백 경로 | `/Collector/2.0/settings/` (Teams-like) |
| `Headers[]` | 요청 헤더 | `Referer: teams.microsoft.com`, `x-ms-*` |
| `Response.Headers[]` | 응답 헤더 위장 | `Microsoft-HTTPAPI/2.0`, CORS teams |
| `Cert` / `Proxy` | 인증서·프록시 | (블록) |
| `WorkingHours` | 활성 시간대 | docs에 명시 |
| SMB `PipeName` | 파이프 | `demon_pipe` |

### 3.3 Demon 프로파일 키

| 키 | 의미 | 기본 예 (havoc.yaotl) |
|----|------|----------------------|
| `Sleep` | 콜백 간격(초) | `2` |
| `Jitter` | % | `15` |
| `TrustXForwardedFor` | CDN/프록시 IP | `false` |
| `Injection.Spawn64/32` | 주입 스폰 대상 | `notepad.exe` / `Werfault.exe` |
| `Binary.ReplaceStrings-*` | 바이너리 문자열 치환 | docs 예시 |

### 3.4 Teamserver / Operators / 부가

```yaotl
Teamserver { Host = "0.0.0.0"; Port = 40056; Build { Compiler64=… Nasm=… } }
Operators  { user "…" { Password = "…" } }
Service    { Endpoint = "…"; Password = "…" }   # External/service agent
WebHook    { Discord { Url=… AvatarUrl=… User=… } }
```

기동: `./havoc server --profile profiles/havoc.yaotl -v`

### 3.5 통신 패러다임 대비 (v1.3)

| | Havoc Demon | c2-project v1.3 |
|--|-------------|-----------------|
| 모델 | **Beacon 폴링** (sleep → HTTP POST) | **WSS 상시** + **HTTP beacon 옵션** ✅ |
| 경로/헤더 | Uris/UA/Headers/Response **프로파일** | 고정 `/agent`, `/beacon` — **B-H7 미흡** |
| 암호화 | `crypt/` 모듈 (앱 레벨) | **TLS만** + 평문 JSON — **B-H8 미흡** |
| 인증 | Operators(유저/비번) vs agent 바인딩 | `AGENT_TOKEN` ≠ `ADMIN_TOKEN` ✅ |
| Host rotation | round-robin / random | 단일 `C2_SERVER` |
| Kill date | 리스너 `KillDate` + WorkingHours | `KILL_DATE` / `MAX_AGENT_AGE` / `die` ✅ |
| Jitter | Demon.Sleep + Jitter | `BEACON_INTERVAL` + `JITTER_PERCENT` ✅ |
| 끊김 특성 | sleep 중 소켓 없음 → 프록시 친화 | WSS는 idle timeout 민감 / beacon은 완화 ✅ |

---

## 4. Demon 기능 맵

### 4.1 README·docs 명시 스택

| 영역 | Havoc | c2-project v1.3 | 벤치 |
|------|-------|-----------------|------|
| Sleep obfuscation | Ekko / Zilean / FOLIAGE / WFSOEx | `time.Sleep` | P2 |
| Stack spoof | x64 return addr + sleep stack dup | 없음 | P2 |
| Indirect syscalls | Nt* | 없음 (T-E1) | P2 |
| AMSI/ETW | HW breakpoint patch | 없음 | P3 (의도적 낮음) |
| Proxy library load | ✅ | Loader 미완 | P1 (체인) |
| 출력물 | exe / dll / shellcode / service | exe + linux + 미완 dll/MSI | P1 |
| SMB pivot | named pipe | 없음 | P3 |
| Token vault | ✅ | 없음 | P3 |
| shell / file | ✅ | **shell/exec/upload/download ✅** | — |
| BOF / dotnet / socks / rportfwd | ✅ | 없음 | P2–3 |
| screenshot 등 | ✅ | stub | P3 |
| Kill date | ✅ | ✅ | — |
| 영속 WMI | **비표준** | **1급 설계** | **차별 유지** |

### 4.2 Demon 명령 (docs/agent 발췌)

`help sleep checkin job task proc transfer dir download upload cd cp remove mkdir pwd cat screenshot shell powershell inline-execute shellcode dll exit token dotnet net config pivot rportfwd socks`

### 4.3 영속성 — 중요 정정 (재확인)

Havoc 본체는 **WMI/레지스트리 영속을 표준 체인으로 강제하지 않음**.  
Demon = 임플란트; 영속은 오퍼레이터·로더·별도 모듈 영역.  
→ 벤치 시 “Havoc처럼 WMI”가 아니라 **임플란트 품질·malleability·잡/프로파일**을 가져오고, **WMI + DocViewer는 우리 차별점 유지**.

---

## 5. Teamserver 패턴 → 우리 적용

| 패턴 | Havoc | v1.3 상태 |
|------|-------|-----------|
| Job queue | 폴링 전까지 적재 | ✅ B-H1 |
| Stable agent meta | DB agents | ✅ B-H2 `agent-<fnv64>` |
| Kill date / working hours | KillDate, WorkingHours | ✅ kill · ⬜ working hours |
| Profile as code | yaotl | ✅ env profiles (B-H4) — 문법 풍부도는 낮음 |
| Operator ≠ agent auth | Operators vs agent | ✅ B-H5 토큰 2종 (유저 RBAC는 없음) |
| HTTP beacon | 기본 | ✅ B-H6 |
| Malleable URI/UA/Header | Uris/Headers/Response | ⬜ **B-H7** |
| App-level crypt | Demon `crypt/` | ⬜ **B-H8** |
| Payload gen on server | MinGW | 호스트 `build.sh`로 충분 |
| External C2 / Service endpoint | Service{} | ⬜ B-H14 (CDN 연동 시) |
| Webhook | Discord 등 | ⬜ B-H13 |
| TrustXForwardedFor | Demon 플래그 | ⬜ (CDN D5) |
| Multiplayer GUI | Qt | ⬜ B-H11 웹 콘솔 |
| SQLite | ✅ | state.json으로 대체 (충분하면 유지) |

---

## 6. 프로토콜 갭 (v1.3 기준)

### Havoc (개념)

```
[Demon] --HTTP POST (malleable URI/UA/headers)--> [Listener]
         body: encrypted metadata + results
         response: encrypted jobs / noop
[Client] --WS :40056--> [Teamserver]
```

### c2-project v1.3

```
[Agent] --WSS /agent  or  POST /beacon--> [c2server]
         JSON register/heartbeat/command/result
         X-Auth-Token: AGENT_TOKEN
[Admin] --HTTPS /admin/* --> [c2server]
         X-Auth-Token: ADMIN_TOKEN
```

| 갭 | 설명 | ID | 상태 |
|----|------|-----|------|
| G1 | 앱 레벨 암호 없음 (TLS only) | B-H8 | ⬜ |
| G2 | 고정 경로·UA·헤더 | B-H7 | ⬜ |
| G3 | 명령 큐 | B-H1 | ✅ |
| G4 | stable ID | B-H2 | ✅ |
| G5 | kill date | B-H3 | ✅ |
| G6 | 파일 청크 | (B-H12) | ✅ v1.3 |
| G7 | host rotation / multi-callback | B-H7 확장 | ⬜ |
| G8 | working hours | — | ⬜ 선택 |
| G9 | 응답 헤더 위장 (server Response) | B-H7 | ⬜ |

---

## 7. 배포 체인 대비

| 단계 | Havoc 전형 | c2-project |
|------|------------|------------|
| 전달 | 생성 exe/dll/sc를 별도 초기침투에 실음 | **미끼 PDF + 위장 MSI** 제품화 |
| 실행 | 스태거/로더/인젝트 | MSI → Loader → Agent |
| 상주 | 주입 대상·서비스 엔트리 등 | **WMI** DocViewer* |
| C2 | HTTP(S) beacon | WSS 및/또는 HTTP beacon |

**우리 강점**: 배포 위장 + WMI 시나리오 특화, 랩 스모크 단순.  
**Havoc 강점**: Demon 생성물·인젝트·슬립·syscall, 오퍼레이터 UX, malleable HTTP, pivot, BOF 생태계.

---

## 8. 기능 매트릭스 (v1.3 갱신)

| 기능 | Havoc | c2-project v1.3 | 비고 |
|------|:-----:|:---------------:|------|
| Teamserver Go | ✅ | ✅ (단순) | |
| GUI Client | ✅ | ❌ | B-H11 |
| Multi-operator RBAC | ✅ | ❌ (토큰 2종만) | |
| 영속 상태 | SQLite | state.json ✅ | |
| HTTP(S) beacon | ✅ | ✅ B-H6 | |
| WebSocket agent | Client 채널 | ✅ agent | 우리 역전 강점 |
| SMB pivot | ✅ | ❌ | |
| External C2 | ✅ | ❌ | B-H14 |
| Malleable URI/UA/Header | ✅ | ❌ | **B-H7** |
| App crypt | ✅ | ❌ | **B-H8** | 부분→**완료(v1.4 PSK AEAD)** | no FS / no per-session KX | ✅ | ✅ | |
| Sleep obfuscation | ✅ | ❌ | |
| Indirect syscall | ✅ | ⬜ | |
| Shellcode out | ✅ | ❌ | |
| DLL/EXE out | ✅ | 부분 | |
| Job queue | ✅ | ✅ | |
| Stable agent ID | ✅ | ✅ | |
| Kill date | ✅ | ✅ | |
| shell/exec | ✅ | ✅ | |
| upload/download chunk | ✅ | ✅ | |
| WMI persistence | 비표준 | ✅ 설계 | **차별** |
| 위장 MSI+PDF | ❌ | ✅ | **차별** |
| BOF/dotnet/socks | ✅ | ❌ | |
| Webhook | ✅ | ❌ | B-H13 |
| 프로파일 | yaotl | env | |
| 랩 단순성 | 무거움 | ✅ | **차별** |

---

## 9. 벤치 백로그 상태

### P0 — 완료 ✅

| ID | 항목 | 상태 |
|----|------|------|
| B-H1 | Job queue + flush + `/admin/jobs` | ✅ |
| B-H2 | Stable agent ID | ✅ |
| B-H3 | Kill date / max age / `die` | ✅ |
| B-H4 | Profile 분리 (`profiles/*.env`) | ✅ |
| B-H5 | AGENT_TOKEN ≠ ADMIN_TOKEN | ✅ |

### P1 — 통신·임플란트

| ID | 항목 | 상태 | 비고 |
|----|------|------|------|
| B-H6 | HTTP beacon | ✅ v1.3 | |
| **B-H7** | **URI/UA/Header (+ Response headers, host list)** | ⬜ | **다음 추천** |
| **B-H8** | **앱 레벨 세션 암호 (최소 HMAC 또는 AEAD)** | ⬜ | **다음 추천** |
| B-H9 | Loader shellcode/dll 엔트리 | ⬜ | DocViewer 체인 |
| B-H10 | Indirect syscall / sleep 고도화 (Win) | ⬜ | Win VM 이후 |
| (파일) | chunk up/down | ✅ v1.3 | 구 B-H12 상당 |

### P2 — UX·확장

| ID | 항목 | 상태 |
|----|------|------|
| B-H11 | 최소 웹 콘솔 | ⬜ |
| B-H13 | Webhook (새 에이전트 알림) | ⬜ |
| B-H14 | External C2 훅 / CDN 워커 | ⬜ |
| B-H15 | Havoc 바이너리 병행 운영 | ❌ 비권장 기본 |

### 권장 흡수 순서 (조사 결론)

1. **B-H7 malleable** — `http_smb.yaotl` 패턴을 env/profile로:  
   `BEACON_URIS`, `BEACON_UA`, `BEACON_HEADERS`, `RESPONSE_HEADERS`, (선택) `C2_HOSTS`+rotation  
2. **B-H8 app crypto** — register 후 세션키 합의, body AEAD; TLS 종료 지점(CDN) 대비  
3. Win VM에서 `agent.exe` 실기 → Loader/MSI/WMI  
4. B-H11 웹 콘솔 / B-H13 webhook은 운영 편의

---

## 10. “Havoc을 쓸까, 벤치만 할까”

| 옵션 | 설명 | 추천 |
|------|------|------|
| **A. 벤치만** | 패턴 흡수, 코드베이스 c2-project 유지 | ✅ **기본** |
| **B. 병행 랩** | 공식 스택 올려 Demon 동작만 관찰 | 선택 (Qt/MinGW/Py3.10 비용) |
| **C. 교체** | c2-project 폐기 | ❌ DocViewer+WMI 재작성, 방향 불일치 |

---

## 11. B-H7 / B-H8 설계 스케치 (구현 전 조사 산출)

### B-H7 Malleable (Havoc Uris/Headers/Response 대응)

| env (안) | 대응 Havoc | 적용 위치 |
|----------|------------|-----------|
| `BEACON_PATH` / `BEACON_URIS` (csv) | `Uris[]` | server mux + agent POST path 선택 |
| `BEACON_UA` | `UserAgent` | agent HTTP client |
| `BEACON_HEADERS` (`k:v;k:v`) | `Headers[]` | agent request |
| `RESPONSE_HEADERS` | `Response.Headers` | server beacon/WSS 응답 |
| `C2_HOSTS` + `HOST_ROTATION` | `Hosts`+`HostRotation` | agent dial 타깃 |
| `AGENT_WS_PATH` | (WS는 우리 고유) | 이미 존재 |

최소 구현: 단일 path/UA/header 오버라이드 → 이후 URI 로테이션.

### B-H8 App crypto (Demon crypt/ 대응)

| 단계 | 내용 |
|------|------|
| 1 | `register`는 TLS+토큰 유지 (부트스트랩) |
| 2 | 서버가 `registered`에 세션 키 재료 (또는 ECDH) |
| 3 | 이후 `heartbeat`/`command`/`result` body = AEAD(ciphertext) |
| 4 | admin API는 별도 (서버 내부 plain) |
| 주의 | 키 하드코딩 금지; 랩에선 PSK(`C2_PSK`)로 시작 가능 |

---

## 12. 참고 링크

| 자원 | URL |
|------|-----|
| GitHub | https://github.com/HavocFramework/Havoc |
| Docs index | https://havocframework.com/docs |
| Listeners | https://havocframework.com/docs/listeners |
| Agent | https://havocframework.com/docs/agent |
| Profiles | https://havocframework.com/docs/profiles |
| Teamserver | https://havocframework.com/docs/teamserver |
| Profile 예 | `profiles/havoc.yaotl`, `http_smb.yaotl`, `webhook_example.yaotl` |
| Demon README | `payloads/Demon/README.md` |
| Architecture | https://deepwiki.com/HavocFramework/Havoc/2-architecture |

---

## 13. 변경 이력

| 날짜 | 내용 |
|------|------|
| 2026-07-31 | 최초 조사 (GitHub/docs/profile/Demon + c2 대조) |
| 2026-07-31 | P0 구현 기록 (B-H1–H5) |
| 2026-07-31 | **재검증**: 공식 README·yaotl·docs·pkg 레이아웃 재확인; **v1.3 갭 매트릭스 갱신** (H6·파일전송 ✅, H7/H8 다음); B-H7/H8 설계 스케치 추가 |

---

## 14. P0 구현 기록 (요약)

| ID | 위치 | 스모크 |
|----|------|--------|
| B-H1–H5 | server/agent v1.1+ | 랩 통과 |
| B-H6 + files | v1.3.0-lab-bundle | Linux 통과; agent.exe PE only |


### v1.4 update
- B-H7/B-H8 implemented in c2-project (see protocol.md, SMOKE_v1.4.md).
