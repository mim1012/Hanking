# D:\Project\_wiki\scripts\lint-scheduled.ps1
# 주간 Windows 작업 스케줄러가 호출 — lint.py를 돌려 결과를 .lint-last.txt에 기록한다.
# rot(죽은 링크/범위초과 file:줄/카운트 불일치)를 자동 감지하는 안전망.
# 주의: lint는 "줄 범위 초과"는 잡지만 "줄 내용이 바뀐 조용한 드리프트"는 못 잡는다.

$ErrorActionPreference = 'Continue'
$wiki = Split-Path -Parent $PSScriptRoot          # D:\Project\_wiki
$lint = Join-Path $PSScriptRoot 'lint.py'
$out  = Join-Path $wiki '.lint-last.txt'

$py = 'C:\Users\PC_1M\AppData\Local\Programs\Python\Python311\python.exe'
if (-not (Test-Path $py)) { $py = 'python' }

$stamp = (Get-Date).ToString('yyyy-MM-dd HH:mm')
$result = & $py -X utf8 $lint 2>&1 | Out-String
$header = "# lint 자동 실행 (주간 스케줄) — $stamp`r`n`r`n"
Set-Content -Path $out -Value ($header + $result) -Encoding utf8

# 콘솔 요약 한 줄 (수동 실행 시 확인용)
$summary = (($result -split "`n") | Where-Object { $_ -match 'DEAD LINKS|stale|README counts' } | ForEach-Object { $_.Trim() }) -join ' | '
Write-Output "[$stamp] $summary"
