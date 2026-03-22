<#
.SYNOPSIS
    Build a LabTether Agent MSI using WiX v5.

.DESCRIPTION
    Stages the pre-built Windows agent binary, then invokes the WiX v5
    compiler to produce a signed-ready MSI for enterprise deployment via
    GPO, SCCM, or Intune.

    Prerequisites (CI / build machine):
      - WiX v5 installed as a .NET global tool:
          dotnet tool install --global wix
        or available directly on PATH as `wix`.
      - WiX WixToolset.Util.wixext extension:
          wix extension add WixToolset.Util.wixext
      - The agent binary already cross-compiled at:
          build/labtether-agent-windows-<arch>.exe

    This script is a CI concern and will not run on macOS (where the
    WiX toolchain is Windows-only). It is safe to commit and lint on any
    platform; it only executes on Windows CI runners.

.PARAMETER Arch
    Target architecture. Accepted values: amd64, arm64.
    Maps to the binary name labtether-agent-windows-<arch>.exe.

.PARAMETER Version
    Semantic version string for the MSI, e.g. "1.4.2".
    Passed to WiX as the ProductVersion property and used in the output
    filename. Defaults to "0.0.0" for local/dev builds.

.PARAMETER BuildDir
    Directory containing the pre-built binary. Defaults to "build"
    relative to the repository root (two directories above this script).

.PARAMETER OutputDir
    Directory to write the finished MSI. Defaults to "build" relative
    to the repository root.

.PARAMETER WixCommand
    Override the WiX executable. Defaults to auto-detection: tries
    "dotnet tool run wix" first, then falls back to bare "wix".

.EXAMPLE
    # amd64 release build from CI:
    .\build-msi.ps1 -Arch amd64 -Version 1.4.2

.EXAMPLE
    # arm64 build with explicit paths:
    .\build-msi.ps1 -Arch arm64 -Version 1.4.2 `
        -BuildDir D:\ci\artifacts -OutputDir D:\ci\msi
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("amd64", "arm64")]
    [string]$Arch,

    [Parameter(Mandatory = $false)]
    [string]$Version = "0.0.0",

    [Parameter(Mandatory = $false)]
    [string]$BuildDir = "",

    [Parameter(Mandatory = $false)]
    [string]$OutputDir = "",

    [Parameter(Mandatory = $false)]
    [string]$WixCommand = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# ---------------------------------------------------------------------------
# Resolve paths relative to repo root (this script lives two levels deep:
#   deploy/agents/windows/wix/build-msi.ps1
#   ↑ wix/  ↑ windows/  ↑ agents/  ↑ deploy/  ↑ repo root
# ---------------------------------------------------------------------------
$ScriptDir = $PSScriptRoot                                        # .../wix/
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir "..\..\..\..")).Path

if ($BuildDir -eq "") {
    $BuildDir = Join-Path $RepoRoot "build"
}
if ($OutputDir -eq "") {
    $OutputDir = Join-Path $RepoRoot "build"
}

# Working directories inside the wix/ subtree
$WixDir     = $ScriptDir
$StagingDir = Join-Path $WixDir "staging"

# ---------------------------------------------------------------------------
# Validate binary
# ---------------------------------------------------------------------------
$BinaryName   = "labtether-agent-windows-$Arch.exe"
$SourceBinary = Join-Path $BuildDir $BinaryName

if (-not (Test-Path $SourceBinary)) {
    Write-Error "Binary not found: $SourceBinary`nBuild the agent with: make build-agent-windows GOARCH=$Arch"
    exit 1
}

Write-Host "Binary   : $SourceBinary"
Write-Host "Version  : $Version"
Write-Host "Arch     : $Arch"
Write-Host "OutputDir: $OutputDir"

# ---------------------------------------------------------------------------
# Stage binary
# ---------------------------------------------------------------------------
if (-not (Test-Path $StagingDir)) {
    New-Item -ItemType Directory -Path $StagingDir -Force | Out-Null
}

$StagedBinary = Join-Path $StagingDir "labtether-agent.exe"
Copy-Item -Path $SourceBinary -Destination $StagedBinary -Force
Write-Host "Staged   : $StagedBinary"

# ---------------------------------------------------------------------------
# Resolve WiX executable
# Auto-detect: prefer dotnet tool (installed globally via dotnet tool install
# --global wix), fall back to bare wix on PATH.
# ---------------------------------------------------------------------------
function Invoke-Wix {
    param([string[]]$Arguments)

    if ($WixCommand -ne "") {
        # Caller-supplied override (e.g. full path or specific invocation)
        & $WixCommand @Arguments
        return $LASTEXITCODE
    }

    # Try dotnet tool run first (works whether wix is a global or local tool)
    $dotnet = Get-Command dotnet -ErrorAction SilentlyContinue
    if ($null -ne $dotnet) {
        & dotnet tool run wix @Arguments
        if ($LASTEXITCODE -ne 127) {
            # 127 = command not found within dotnet tools; any other code
            # means dotnet found the tool (success=0, failure=non-zero).
            return $LASTEXITCODE
        }
    }

    # Fall back to bare wix binary on PATH
    $wixBin = Get-Command wix -ErrorAction SilentlyContinue
    if ($null -ne $wixBin) {
        & wix @Arguments
        return $LASTEXITCODE
    }

    Write-Error "WiX v5 not found. Install with: dotnet tool install --global wix"
    exit 1
}

# ---------------------------------------------------------------------------
# Ensure required WiX extension is present
# WixToolset.Util.wixext provides util:PermissionEx used in Agent.wxs.
# This is a no-op if already installed; safe to run on every build.
# ---------------------------------------------------------------------------
Write-Host "Ensuring WiX extensions..."
$extResult = Invoke-Wix @("extension", "add", "WixToolset.Util.wixext", "--global")
if ($extResult -ne 0) {
    # Non-fatal: extension may already be installed; WiX will error at
    # compile time if it truly cannot be found.
    Write-Warning "WiX extension add exited $extResult — may already be installed."
}

# ---------------------------------------------------------------------------
# Map architecture to WiX platform string
# ---------------------------------------------------------------------------
$Platform = switch ($Arch) {
    "amd64" { "x64"   }
    "arm64" { "arm64" }
    default {
        Write-Error "Unknown arch: $Arch"
        exit 1
    }
}

# ---------------------------------------------------------------------------
# Ensure output directory exists
# ---------------------------------------------------------------------------
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

$OutputMsi = Join-Path $OutputDir "labtether-agent-windows-$Arch.msi"

# ---------------------------------------------------------------------------
# WiX build
#
# wix build [sources] [flags]
#   -arch          target platform (x64 / arm64)
#   -d             define preprocessor variable (Version)
#   -o             output MSI path
#   -ext           load extension DLL
#
# Source files: Product.wxs + Agent.wxs (both in $WixDir).
# WiX v5 resolves relative paths in source files against the source file's
# own directory, so staging\labtether-agent.exe is resolved from $WixDir.
# ---------------------------------------------------------------------------
Write-Host ""
Write-Host "Building MSI..."

$Sources = @(
    (Join-Path $WixDir "Product.wxs"),
    (Join-Path $WixDir "Agent.wxs")
)

$WixArgs = @(
    "build"
    ) + $Sources + @(
    "-arch",    $Platform,
    "-d",       "Version=$Version",
    "-ext",     "WixToolset.Util.wixext",
    "-o",       $OutputMsi
)

$BuildResult = Invoke-Wix $WixArgs

if ($BuildResult -ne 0) {
    Write-Error "WiX build failed (exit $BuildResult). Check output above for details."
    exit $BuildResult
}

# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------
Write-Host ""
Write-Host "MSI build succeeded."
Write-Host "  Output : $OutputMsi"
Write-Host "  Size   : $([Math]::Round((Get-Item $OutputMsi).Length / 1MB, 2)) MB"
Write-Host ""
Write-Host "Deploy example (silent install with properties):"
Write-Host "  msiexec /i `"$OutputMsi`" /qn HUB_URL=wss://hub.example.com ENROLLMENT_TOKEN=<token>"
Write-Host ""
Write-Host "Deploy example (silent install, no enrollment — configure later via registry):"
Write-Host "  msiexec /i `"$OutputMsi`" /qn"
