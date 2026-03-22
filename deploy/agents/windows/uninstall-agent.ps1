param(
    [string]$ServiceName = "LabTetherAgent",
    [string]$InstallDir  = "C:\Program Files\LabTether",
    [string]$ConfigDir   = "C:\ProgramData\LabTether"
)

$ErrorActionPreference = "Stop"

# Stop and remove service
$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($null -ne $svc) {
    if ($svc.Status -eq "Running") {
        Write-Host "Stopping $ServiceName..."
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }
    Write-Host "Removing service..."
    sc.exe delete $ServiceName | Out-Null
} else {
    Write-Host "Service '$ServiceName' not found — skipping."
}

# Remove binaries
if (Test-Path $InstallDir) {
    Remove-Item -Path $InstallDir -Recurse -Force
    Write-Host "Removed: $InstallDir"
} else {
    Write-Host "Install directory '$InstallDir' not found — skipping."
}

Write-Host ""
Write-Host "Uninstall complete."
Write-Host "Configuration preserved at: $ConfigDir"
Write-Host "(Delete manually if no longer needed)"
