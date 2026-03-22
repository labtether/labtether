param(
    [string]$Arch             = "amd64",
    [string]$ServiceName      = "LabTetherAgent",
    [string]$DisplayName      = "LabTether Agent",
    [string]$InstallDir       = "C:\Program Files\LabTether",
    [string]$ConfigDir        = "C:\ProgramData\LabTether",
    [string]$SourceDir        = "build",
    [string]$HubURL           = "",
    [string]$EnrollmentToken  = "",
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"

if ($Uninstall) {
    $uninstallScript = Join-Path $PSScriptRoot "uninstall-agent.ps1"
    if (Test-Path $uninstallScript) {
        & $uninstallScript -ServiceName $ServiceName -InstallDir $InstallDir -ConfigDir $ConfigDir
    } else {
        throw "Uninstall script not found: $uninstallScript"
    }
    return
}

# Validate arch
if ($Arch -notin @("amd64", "arm64")) {
    throw "Unsupported -Arch value '$Arch'. Must be 'amd64' or 'arm64'."
}

$binaryName = "labtether-agent-windows-$Arch.exe"
$sourceBinary = Join-Path $SourceDir $binaryName

if (-not (Test-Path $sourceBinary)) {
    throw "Binary not found: $sourceBinary"
}

# --- Config directory with restricted ACLs ---
if (-not (Test-Path $ConfigDir)) {
    Write-Host "Creating config directory: $ConfigDir"
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null

    $acl = Get-Acl $ConfigDir
    $acl.SetAccessRuleProtection($true, $false)   # disable inheritance, remove inherited rules

    $systemRule = New-Object System.Security.AccessControl.FileSystemAccessRule(
        "SYSTEM", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
    )
    $adminRule = New-Object System.Security.AccessControl.FileSystemAccessRule(
        "Administrators", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
    )

    $acl.AddAccessRule($systemRule)
    $acl.AddAccessRule($adminRule)
    Set-Acl -Path $ConfigDir -AclObject $acl
    Write-Host "Config directory created with restricted ACLs."
}

# --- Install directory ---
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$destBinary = Join-Path $InstallDir "labtether-agent.exe"

# --- Upgrade path: stop service before replacing binary ---
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($null -ne $existing) {
    Write-Host "Existing service found. Performing upgrade..."
    if ($existing.Status -eq "Running") {
        Write-Host "Stopping $ServiceName..."
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }
}

# Copy binary into place
Copy-Item -Path $sourceBinary -Destination $destBinary -Force
Write-Host "Binary installed to: $destBinary"

# --- Build environment block for service ---
# sc.exe does not natively set env vars on services; we use the registry.
$regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"

# --- Create or reconfigure service ---
if ($null -eq $existing) {
    Write-Host "Creating service: $ServiceName"
    New-Service -Name $ServiceName `
                -BinaryPathName "`"$destBinary`"" `
                -DisplayName $DisplayName `
                -StartupType Automatic
} else {
    Write-Host "Reconfiguring service: $ServiceName"
    sc.exe config $ServiceName binPath= "`"$destBinary`"" start= auto | Out-Null
}

# --- Service description ---
sc.exe description $ServiceName "LabTether Agent: endpoint monitoring, operations, and remote access." | Out-Null

# --- Recovery configuration: restart 3x with 60s delay, reset after 24h ---
sc.exe failure $ServiceName reset= 86400 actions= restart/60000/restart/60000/restart/60000 | Out-Null
Write-Host "Recovery policy configured (restart x3, 60s delay, 24h reset)."

# --- Environment variables via registry ---
$envVars = @{}
if ($HubURL -ne "") {
    $envVars["LABTETHER_WS_URL"] = $HubURL
}
if ($EnrollmentToken -ne "") {
    $envVars["LABTETHER_ENROLLMENT_TOKEN"] = $EnrollmentToken
}

if ($envVars.Count -gt 0) {
    # Registry key may not exist yet if service was just created; wait briefly
    Start-Sleep -Seconds 1
    $existing_env = (Get-ItemProperty -Path $regPath -Name "Environment" -ErrorAction SilentlyContinue).Environment
    $envList = if ($existing_env) { [System.Collections.Generic.List[string]]$existing_env } else { [System.Collections.Generic.List[string]]@() }

    foreach ($key in $envVars.Keys) {
        # Remove old entry for this key if present
        $envList = [System.Collections.Generic.List[string]]($envList | Where-Object { $_ -notmatch "^${key}=" })
        $envList.Add("${key}=$($envVars[$key])")
    }

    Set-ItemProperty -Path $regPath -Name "Environment" -Value ($envList.ToArray()) -Type MultiString
    Write-Host "Service environment variables set."
}

# --- Start service ---
Write-Host "Starting $ServiceName..."
Start-Service -Name $ServiceName

Write-Host ""
Write-Host "Install complete."
Write-Host "  Service : $ServiceName"
Write-Host "  Binary  : $destBinary"
Write-Host "  Config  : $ConfigDir"
