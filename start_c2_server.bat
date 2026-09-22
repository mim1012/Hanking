@echo off
title C2 Server (Standalone) - 상시 기동
cd /d "%~dp0"
echo [%date% %time%] C2 Server starting...
echo.

:: c2-server.exe 가 같은 폴더에 있어야 함
if not exist "%~dp0bin\c2-server.exe" (
    echo ERROR: bin\c2-server.exe not found!
    echo 현재 위치: %~dp0
    pause
    exit /b 1
)

:: 환경변수 설정 (선택 — 생략시 기본값으로 작동)
set C2_LISTEN_ADDR=0.0.0.0:8443
set DATA_DIR=%~dp0data
set TLS_CERT_PATH=%~dp0certs\server.crt
set TLS_KEY_PATH=%~dp0certs\server.key
set C2_LOG_FILE=%~dp0data\log.txt
set C2_AUTO_TLS=1
set AGENT_TOKEN=lab-only-change-me
set ADMIN_TOKEN=lab-only-change-me
set PERSIST_STATE=1

:: 실행
echo Starting c2-server.exe on port 8443 (TLS + auto-cert)...
echo Log: %C2_LOG_FILE%
echo.

start "" /B "%~dp0bin\c2-server.exe"

:: 잠시 대기 후 상태 확인
timeout /t 3 /nobreak >nul

:: /health 확인 (curl or PowerShell)
echo.
echo === Health Check ===
powershell -Command "try { $r = Invoke-WebRequest -Uri 'https://127.0.0.1:8443/health' -UseBasicParsing -SkipCertificateCheck; Write-Host ('OK: ' + $r.Content) } catch { Write-Host ('FAIL: ' + $_ ) }"

echo.
echo === Server PID ==
tasklist /FI "IMAGENAME eq c2-server.exe" 2>nul | findstr /I "c2-server"
echo.
echo C2 server running on https://0.0.0.0:8443
echo Admin API: https://127.0.0.1:8443/admin?token=lab-only-change-me
echo Health: https://127.0.0.1:8443/health
echo.
echo Close this window to keep server running (or use taskkill).
echo.
pause