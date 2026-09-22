# Hangame §3-1 process name probe — run on Windows lab VM
# Captures actual image names for baduki/poker/launchers and diffs against allowlist.
#
# Usage (PowerShell, admin optional):
#   .\scripts\hangame_proc_probe.ps1
#   .\scripts\hangame_proc_probe.ps1 -OutDir C:\temp\hangame_probe
#
# After starting 바둑이/포커 client, re-run and attach data/hangame_proc_probe_last.md
# back into research/hangame-process-detect.md §3 "실측 확인".

param(
    [string]$OutDir = ""
)

$ErrorActionPreference = "Continue"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Split-Path -Parent $ScriptDir
if (-not $OutDir) {
    $OutDir = Join-Path $Root "data"
}
if (-not (Test-Path $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
}

$ts = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
$allowlistPath = Join-Path $Root "profiles\hangame_games.json"

# Expected §3-1 images (fallback if JSON missing)
$expected = @(
    "baduki.exe", "poker7.exe", "laspoker.exe",
    "duelpoker.exe", "dualpocker.exe", "highlow2.exe"
)
$launchers = @(
    "hangamestartersetup.exe", "hanstarter.exe", "hanstarter2.exe",
    "HanStarter.exe", "HanStarter2.exe"
)

if (Test-Path $allowlistPath) {
    try {
        $prof = Get-Content $allowlistPath -Raw | ConvertFrom-Json
        $expected = @()
        foreach ($g in $prof.games) {
            foreach ($img in $g.images) { $expected += $img.ToLower() }
        }
        if ($prof.launchers_ignore) {
            $launchers = @($prof.launchers_ignore)
        }
    } catch {
        Write-Host "[!] failed to parse allowlist: $_" -ForegroundColor Yellow
    }
}

$pattern = 'baduk|poker|duel|laspoker|highlow|hoola|gostop|duelgo|hangame|starter|hanstarter|ssduel|gmahjong|bzbaduk|hanjanggi|yutnori|sudda|hangang|omok|solcard'

Write-Host "[*] Hangame process probe @ $ts" -ForegroundColor Cyan
Write-Host "[*] Allowlist: $($expected -join ', ')"

$all = @()
try {
    $all = Get-Process | Select-Object Name, Id, Path, Company, Description
} catch {
    Write-Host "[-] Get-Process failed: $_" -ForegroundColor Red
}

$matched = $all | Where-Object {
    $_.Name -match $pattern -or
    ($_.Path -and $_.Path -match $pattern) -or
    ($_.Description -and $_.Description -match '한게임|Hangame|HanStarter')
} | Sort-Object Name

# Also tasklist CSV for image-name parity with agent scanner
$tasklistRows = @()
try {
    $csv = & tasklist /FO CSV /NH 2>$null
    foreach ($line in $csv) {
        if ($line -match $pattern) {
            $tasklistRows += $line
        }
    }
} catch {}

$foundNames = @{}
foreach ($p in $matched) {
    $n = ($p.Name + ".exe").ToLower()
    if ($p.Name -match '\.exe$') { $n = $p.Name.ToLower() }
    else { $n = ($p.Name + ".exe").ToLower() }
    $foundNames[$n] = $p
}

$hit = @()
$miss = @()
foreach ($e in $expected) {
    $k = $e.ToLower()
    # Get-Process Name is without .exe
    $stem = $k -replace '\.exe$', ''
    $proc = $all | Where-Object { $_.Name -ieq $stem }
    if ($proc) { $hit += $k } else { $miss += $k }
}

$launcherHits = @()
foreach ($L in $launchers) {
    $stem = ($L.ToLower() -replace '\.exe$', '')
    if ($all | Where-Object { $_.Name -ieq $stem }) { $launcherHits += $L }
}

$md = @()
$md += "# Hangame process probe — live capture"
$md += ""
$md += "- **When**: $ts"
$md += "- **Host**: $env:COMPUTERNAME"
$md += "- **User**: $env:USERNAME"
$md += "- **Allowlist file**: ``$allowlistPath``"
$md += ""
$md += "## Matched processes (pattern filter)"
$md += ""
$md += "| Name | PID | Path | Description |"
$md += "|------|-----|------|-------------|"
if ($matched) {
    foreach ($p in $matched) {
        $path = if ($p.Path) { $p.Path } else { "" }
        $desc = if ($p.Description) { $p.Description } else { "" }
        $md += "| $($p.Name) | $($p.Id) | $path | $desc |"
    }
} else {
    $md += "| _(none)_ | | | |"
}
$md += ""
$md += "## Allowlist cross-check (§3-1)"
$md += ""
$md += "| Image | Present |"
$md += "|-------|---------|"
foreach ($e in $expected) {
    $ok = if ($hit -contains $e.ToLower()) { "YES" } else { "no" }
    $md += "| $e | $ok |"
}
$md += ""
$md += "- **Hits**: $($hit -join ', ')"
$md += "- **Missing (not running now)**: $($miss -join ', ')"
$md += "- **Launcher processes seen**: $(if ($launcherHits) { $launcherHits -join ', ' } else { '(none)' })"
$md += ""
$md += "## tasklist lines (pattern)"
$md += ""
$md += '```'
if ($tasklistRows) { $md += ($tasklistRows -join "`n") } else { $md += "(none)" }
$md += '```'
$md += ""
$md += "## Operator notes"
$md += ""
$md += "1. Start 바둑이/7포커 etc., re-run this script."
$md += "2. Confirm ``Name`` column matches allowlist (case-insensitive)."
$md += "3. Add any new launcher image to ``launchers_ignore`` in hangame_games.json."
$md += "4. Paste this file into research/hangame-process-detect.md as 실측 확인."
$md += ""

$mdPath = Join-Path $OutDir "hangame_proc_probe_last.md"
$jsonPath = Join-Path $OutDir "hangame_proc_probe_last.json"
$md -join "`n" | Set-Content -Path $mdPath -Encoding UTF8

$obj = [ordered]@{
    when          = $ts
    host          = $env:COMPUTERNAME
    expected      = $expected
    hits          = $hit
    missing       = $miss
    launcher_hits = $launcherHits
    matched       = @($matched | ForEach-Object {
        [ordered]@{ name = $_.Name; pid = $_.Id; path = $_.Path; description = $_.Description }
    })
}
$obj | ConvertTo-Json -Depth 6 | Set-Content -Path $jsonPath -Encoding UTF8

Write-Host "[+] wrote $mdPath" -ForegroundColor Green
Write-Host "[+] wrote $jsonPath" -ForegroundColor Green
Write-Host "[*] allowlist hits: $($hit.Count) / $($expected.Count)"
if ($matched) {
    $matched | Format-Table Name, Id, Path -AutoSize
} else {
    Write-Host "[*] no hangame-like processes running (baseline OK — start a client and re-run)" -ForegroundColor Yellow
}

exit 0
