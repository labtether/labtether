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

# Remove the Hub SSH key. The agent service runs as SYSTEM and adds the key,
# with the comment labtether-hub, to administrators_authorized_keys. Only lines
# with that comment are removed; a failure is reported and uninstall carries on.
$authorizedKeys = Join-Path $env:ProgramData "ssh\administrators_authorized_keys"
if (Test-Path -LiteralPath $authorizedKeys) {
    try {
        $lines = @(Get-Content -LiteralPath $authorizedKeys)
        $kept = @($lines | Where-Object { $_ -notmatch '^\S+\s+\S+\s+labtether-hub\s*$' })
        if ($kept.Count -ne $lines.Count) {
            # Rewrite in place so the file keeps its ACL; UTF-8 without a BOM.
            [System.IO.File]::WriteAllLines($authorizedKeys, [string[]]$kept)
            Write-Host "Removed the Hub SSH key from $authorizedKeys"
        }
    } catch {
        Write-Warning "Could not update ${authorizedKeys}: remove the lines ending in labtether-hub by hand."
    }
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
