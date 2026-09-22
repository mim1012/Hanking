# C2 Server — Standalone Deploy Guide (v1.5.0)

> **Created**: 2026-08-01
> **Version**: 1.5.0-lab-standalone
> **Purpose**: 단일 `c2-server.exe` 더블클릭만으로 상시 기동되는 Windows용 C2 서버

---

## 산출물

| 파일 | 위치 | 크기 | 설명 |
|------|------|------|------|
| `c2-server.exe` | `bin/c2-server.exe` | 5.6 MB | Windows GUI exe (콘솔 없이 백그라운드 실행) |
| `start_c2_server.bat` | `start_c2_server.bat` | 1.4 KB | 실행+상태 확인 배치 파일 |
| `c2server` | `bin/c2server` | 8.0 MB | Linux용 바이너리 (동일 기능) |
| `main.go` | `server/main.go` | 46 KB | 소스코드 (v1.5.0) |

---

## 핵심 기능

### 자체서명 TLS 인증서 자동생성 (`C2_AUTO_TLS=1`)
- **설정 파일 없이** 첫 실행 시 `certs/server.crt` + `certs/server.key` 자동 생성
- RSA 2048비트, 10년 유효, Subject: `DocView Soft / localhost`
- 이미 인증서가 있으면 생성 스킵 → 재시작해도 문제없음

### 콘솔 숨김 실행 (`-H=windowsgui`)
- 더블클릭만으로 실행, CMD 창 안 뜸
- 모든 로그는 `data/log.txt`에 기록

### 환경변수 기본값 내장
- `AGENT_TOKEN` / `ADMIN_TOKEN`: 기본값 `lab-only-change-me`
- `LISTEN`: `0.0.0.0:8443`
- `DATA_DIR`: `./data` (exe 기준 상대경로)
- `PERSIST_STATE`: 1 (에이전트/잡 상태 디스크 저장)
- `C2_CRYPTO`: PSK 설정 안 하면 off

---

## 사용법

### Windows (더블클릭)

```
1. c2-server.exe 를 원하는 폴더에 복사
2. 더블클릭
   → 자동: certs/server.crt + certs/server.key 생성
   → 자동: data/ + data/files/ + data/state.json 생성
   → https://0.0.0.0:8443 에서 리스닝 시작

또는 start_c2_server.bat 사용:
   → bat를 같은 폴더에 두고 실행
   → 3초 후 /health 자동 확인
   → 상태 출력 후 대기
```

### Linux (nohup)

```bash
cd /path/to/deploy
./bin/c2server &
# 또는 nohup으로:
nohup ./bin/c2server > /dev/null 2>&1 &
```

### 환경변수 오버라이드 (선택)

```bash
set C2_LISTEN_ADDR=0.0.0.0:9443     # 포트 변경
set ADMIN_TOKEN=my-secret-admin      # 관리자 토큰 변경
set AGENT_TOKEN=my-secret-agent      # 에이전트 토큰 변경
set C2_LOG_FILE=D:\logs\c2.txt      # 로그 위치 변경
set C2_AUTO_TLS=0                    # 기존 인증서만 사용
```

---

## /health 검증

```bash
# Powershell (Windows)
curl -sk https://127.0.0.1:8443/health

# 응답 예시
{"status":"ok","server_ver":"1.5.0-lab-standalone","agents":0,"online":0,
 "jobs":0,"persist":true,"crypto":false,"beacon_path":"/beacon",
 "transports":["wss","http"],"malleable":true}
```

### Python / curl

```bash
curl -sk https://127.0.0.1:8443/health
```

---

## 종료 방법

```bash
# Windows
taskkill /F /IM c2-server.exe

# Linux
pkill -f c2server
```

---

## 상시 기동 테스트 시나리오

### 1. 서버 기동 확인

| 단계 | 확인 항목 | 기대 결과 |
|------|-----------|-----------|
| 1a | 더블클릭 실행 | 콘솔 창 없음 |
| 1b | 프로세스 목록 (tasklist) | `c2-server.exe` 실행 중 |
| 1c | /health | `status: ok` |
| 1d | data/log.txt | "c2server 1.5.0-lab-standalone listen=..." |

### 2. 중단 없는 기동 (24h+)

| 항목 | 설명 |
|------|------|
| 리스너 | TLS 1.2, 포트 8443 |
| 재접속 | 에이전트가 끊겨도 서버는 계속 리스닝 |
| 상태 지속성 | `data/state.json`에 에이전트/잡 저장 |
| 메모리 누수 | Go GC가 자동 관리 (프로덕션 전 장시간 모니터링 권장) |

### 3. agent 등록 확인 (WSS)

```bash
# 에이전트 토큰으로 WebSocket 연결
wscat -c wss://127.0.0.1:8443/agent?token=lab-only-change-me -n
{"type":"register","agent_id":"test-001","hostname":"WINVM",...}
# 응답: {"type":"registered","agent_id":"test-001",...}
```

---

## 코드 변경 요약 (v1.4 → v1.5.0)

| 변경 | 설명 |
|------|------|
| `ensureTLSCerts()` | 자체서명 RSA 2048 인증서 생성 (x509.CreateCertificate) |
| `pemEncodeCert()` / `pemEncodeKey()` | PEM 인코더 (encoding/pem 없이 직접 구현) |
| `AutoTLS` Config 필드 | `C2_AUTO_TLS` 환경변수 (기본 1) |
| `LogFile` Config 필드 | `C2_LOG_FILE` (기본 `log.txt`, `data/` 아래) |
| `logPrintf()` | 파일+stdout 동시 로깅 |
| `-ldflags="-H=windowsgui"` | Windows GUI 모드 (콘솔 창 없음) |
| `-s -w` | Strip debug/symbols (5.6MB로 축소) |

---

## 보안 참고

- **자체서명 인증서**는 테스트/내부망 용도입니다. 익스터널 노출 시 Let's Encrypt 등 유효 인증서 사용 권장
- **기본 토큰(`lab-only-change-me`)** 은 반드시 변경
- **C2_PSK** 설정 시 AES-256-GCM으로 모든 통신 암호화 (B-H8)

---

## 파일 목록 (전체)

```
outputs/c2-project/
├── bin/
│   ├── c2-server.exe          # Windows standalone (5.6 MB)
│   └── c2server                # Linux standalone (8.0 MB)
├── server/
│   └── main.go                 # Source (v1.5.0)
├── start_c2_server.bat         # Windows 실행 배치파일
└── STANDALONE_DEPLOY.md        # 본 문서
```