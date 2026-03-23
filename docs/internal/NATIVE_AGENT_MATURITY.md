# Native Agent Maturity Roadmap

This document tracks the remaining work needed to bring the macOS and Windows native agents to production-grade maturity. It covers code signing, installers, credential storage, crash reporting, and the agent update mechanism.

Related audit issues: #8, #9, #20, #21, #34, #35.

---

## 1. macOS Agent -- Code Signing & Notarization (issue #8)

### Prerequisites

- **Apple Developer Program membership** ($99/year) -- required for Developer ID certificates and notarization.
- A **Developer ID Application** certificate (not a Mac App Store certificate -- the agent is distributed outside the App Store).
- An **app-specific password** for `notarytool` (generated at appleid.apple.com under "Sign-In and Security > App-Specific Passwords").

### Current State

The release workflow (`mac-agent/.github/workflows/release.yml`) builds a universal (arm64 + x86_64) binary and uploads it to GitHub Releases. There is no code signing or notarization step. Users downloading the binary will see the macOS Gatekeeper warning: _"cannot be opened because the developer cannot be verified"_.

### Implementation Plan

Add the following steps to `mac-agent/.github/workflows/release.yml` between the "Package universal binary" and "Upload to GitHub Release" steps:

#### 1a. Import signing certificate

Store the `.p12` certificate as a base64-encoded GitHub Actions secret (`APPLE_CERTIFICATE_P12`) along with its password (`APPLE_CERTIFICATE_PASSWORD`).

```yaml
- name: Import signing certificate
  env:
    CERTIFICATE_P12: ${{ secrets.APPLE_CERTIFICATE_P12 }}
    CERTIFICATE_PASSWORD: ${{ secrets.APPLE_CERTIFICATE_PASSWORD }}
  run: |
    KEYCHAIN_PATH="${RUNNER_TEMP}/signing.keychain-db"
    KEYCHAIN_PASSWORD="$(openssl rand -base64 24)"

    security create-keychain -p "${KEYCHAIN_PASSWORD}" "${KEYCHAIN_PATH}"
    security set-keychain-settings -lut 21600 "${KEYCHAIN_PATH}"
    security unlock-keychain -p "${KEYCHAIN_PASSWORD}" "${KEYCHAIN_PATH}"

    echo "${CERTIFICATE_P12}" | base64 --decode > "${RUNNER_TEMP}/cert.p12"
    security import "${RUNNER_TEMP}/cert.p12" \
      -P "${CERTIFICATE_PASSWORD}" \
      -A -t cert -f pkcs12 \
      -k "${KEYCHAIN_PATH}"
    security list-keychain -d user -s "${KEYCHAIN_PATH}"
```

#### 1b. Sign the binary

```yaml
- name: Code sign binary
  env:
    SIGNING_IDENTITY: ${{ secrets.APPLE_SIGNING_IDENTITY }}
  run: |
    STAGE_DIR="${RUNNER_TEMP}/release-stage"
    codesign --deep --force --options runtime \
      --sign "${SIGNING_IDENTITY}" \
      "${STAGE_DIR}/labtether-agent-macos"
```

The `--options runtime` flag enables the hardened runtime, which is required for notarization.

#### 1c. Notarize

```yaml
- name: Notarize binary
  env:
    APPLE_ID: ${{ secrets.APPLE_ID }}
    APPLE_APP_PASSWORD: ${{ secrets.APPLE_APP_PASSWORD }}
    APPLE_TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
  run: |
    STAGE_DIR="${RUNNER_TEMP}/release-stage"
    # notarytool requires a zip or dmg
    ditto -c -k "${STAGE_DIR}/labtether-agent-macos" "${STAGE_DIR}/notarize.zip"
    xcrun notarytool submit "${STAGE_DIR}/notarize.zip" \
      --apple-id "${APPLE_ID}" \
      --password "${APPLE_APP_PASSWORD}" \
      --team-id "${APPLE_TEAM_ID}" \
      --wait --timeout 10m
    xcrun stapler staple "${STAGE_DIR}/labtether-agent-macos"
```

#### Required GitHub Secrets

| Secret                     | Description                                    |
|----------------------------|------------------------------------------------|
| `APPLE_CERTIFICATE_P12`   | Base64-encoded Developer ID Application `.p12` |
| `APPLE_CERTIFICATE_PASSWORD` | Password for the `.p12` file                |
| `APPLE_SIGNING_IDENTITY`  | e.g. `"Developer ID Application: LabTether (TEAMID)"` |
| `APPLE_ID`                | Apple ID email for notarytool                  |
| `APPLE_APP_PASSWORD`      | App-specific password for notarytool           |
| `APPLE_TEAM_ID`           | 10-character Apple Developer Team ID           |

### Verification

After implementation, download a release artifact on a fresh Mac and confirm:
- `codesign -dv --verbose=4 labtether-agent-macos` shows a valid signature.
- `spctl --assess --type execute labtether-agent-macos` returns "accepted".
- Double-clicking the binary does not trigger a Gatekeeper warning.

---

## 2. Agent Update Mechanism (issue #21)

### Current State

The Go agent binary (used on Linux/FreeBSD, cross-compiled from `hub/`) already has a self-update mechanism:

- The install script accepts `--auto-update true|false` (default: true) and `--force-update` flags.
- On startup (when auto-update is enabled), the agent calls `update self` which checks the hub for a newer binary, downloads it, replaces itself, and restarts.
- The hub API (`internal/hubapi/agents/agent_settings_http_handlers.go`) can trigger remote self-update and reports status back as an event log entry.

### What the Native Agents Need

The macOS (Swift) and Windows (C#) agents are **wrappers** around the Go agent binary. They provide the native UI (menu bar / system tray) and manage the Go agent process lifecycle. The update mechanism needs to cover both layers:

#### Layer 1: Go Agent Binary Update (already works)

The Go binary's built-in `update self` command handles its own updates. The native wrappers already launch and manage this binary, so Go-level updates work as-is.

#### Layer 2: Native Wrapper Update (not yet implemented)

The native wrapper (Swift app / C# WinUI app) itself needs an update path:

**macOS approach:**
- Use Sparkle framework (https://sparkle-project.org/) -- the de facto standard for macOS app auto-updates.
- Host an `appcast.xml` alongside GitHub Releases.
- The agent checks the appcast on a configurable interval (e.g. daily), downloads the new `.tar.gz`, verifies the signature, replaces itself, and relaunches.
- Alternative: implement a simpler custom updater that checks the GitHub Releases API for a newer tag, downloads, and swaps the binary (similar to the Go agent's approach).

**Windows approach:**
- If using MSIX (see section 3): the App Installer protocol handles updates automatically via `.appinstaller` files.
- If using a raw binary distribution: implement a similar check-and-replace mechanism, downloading from GitHub Releases.

### Implementation Priority

1. The Go agent self-update already covers the core functionality.
2. Native wrapper updates are lower priority since the wrapper changes less frequently than the agent binary.
3. Start with the simpler GitHub Releases API check approach for both platforms before adding Sparkle/App Installer.

---

## 3. Windows Installer (issue #9)

### Current State

The Windows release workflow (`win-agent/.github/workflows/release.yml`) publishes a `.zip` containing the self-contained .NET 8 executable. Users must manually extract and run the binary. There is no installer, Start Menu shortcut, or auto-start configuration.

### Options

| Approach | Pros | Cons |
|----------|------|------|
| **MSIX** | Modern, sandboxed, supports App Installer for auto-update, Microsoft Store ready | Requires Windows SDK for packaging, code signing cert needed |
| **MSI (WiX)** | Classic, Group Policy deployable, enterprise-friendly | More complex build tooling, no built-in auto-update |
| **Inno Setup** | Simple, well-documented, single `.exe` installer | Not enterprise-standard, no GPO deployment |

### Recommendation

Start with **MSIX** for simplicity and built-in update support:

1. Add MSIX packaging to the `.csproj`:
   ```xml
   <PropertyGroup>
     <WindowsPackageType>MSIX</WindowsPackageType>
     <EnableMsixTooling>true</EnableMsixTooling>
   </PropertyGroup>
   ```

2. Create a `Package.appxmanifest` defining capabilities, visual assets, and the startup task (for auto-start on login).

3. Sign the MSIX with a code signing certificate (can use a self-signed cert for testing, need a trusted cert for production).

4. Optionally create a `.appinstaller` file hosted alongside releases for automatic updates.

### CI Steps to Add

```yaml
- name: Build MSIX package
  run: >-
    msbuild src/LabTetherAgent/LabTetherAgent.csproj
    -t:Publish
    -p:Configuration=Release
    -p:RuntimeIdentifier=win-x64
    -p:Platform=x64
    -p:SelfContained=true
    -p:WindowsPackageType=MSIX
    -p:EnableMsixTooling=true
    -p:GenerateAppxPackageOnBuild=true
    -p:AppxPackageSigningEnabled=true
    -p:PackageCertificateThumbprint=${{ secrets.WIN_CERT_THUMBPRINT }}

- name: Sign MSIX
  run: >-
    signtool sign /fd SHA256 /a /f cert.pfx /p ${{ secrets.WIN_CERT_PASSWORD }}
    publish/*.msix
```

### Required Secrets

| Secret                  | Description                              |
|-------------------------|------------------------------------------|
| `WIN_CERT_PFX`         | Base64-encoded code signing certificate  |
| `WIN_CERT_PASSWORD`    | Password for the `.pfx` file             |
| `WIN_CERT_THUMBPRINT`  | Certificate thumbprint for MSBuild       |

---

## 4. Windows Credential Storage (issue #34)

### Current State

`win-agent/src/LabTetherAgent/Settings/CredentialStore.cs` has a working credential store interface with four resource names (`ApiToken`, `EnrollmentToken`, `LocalApiAuth`, `WebRtcTurnPass`). However, the actual storage uses an **in-memory dictionary backed by a plaintext file** (`$SETTINGS_DIR/.credentials`). Three methods (`Store`, `Retrieve`, `Remove`) contain `// TODO: Replace with PasswordVault when compiling on Windows`.

### Target Implementation

Use `Windows.Security.Credentials.PasswordVault` for secure OS-level credential storage:

```csharp
using Windows.Security.Credentials;

private readonly PasswordVault _vault = new();

public void Store(string resourceName, string value)
{
    Remove(resourceName); // PasswordVault throws if duplicate
    _vault.Add(new PasswordCredential(resourceName, UserName, value));
}

public string? Retrieve(string resourceName)
{
    try
    {
        var cred = _vault.Retrieve(resourceName, UserName);
        cred.RetrievePassword();
        return cred.Password;
    }
    catch (Exception)
    {
        return null;
    }
}

public void Remove(string resourceName)
{
    try
    {
        var cred = _vault.Retrieve(resourceName, UserName);
        _vault.Remove(cred);
    }
    catch (Exception) { }
}
```

### Requirements

- The project must target `net8.0-windows10.0.19041.0` or later (WinRT interop).
- Add `<TargetPlatformMinVersion>10.0.17763.0</TargetPlatformMinVersion>` to the `.csproj`.
- Keep the file-based fallback for development/testing on non-Windows platforms.
- Migrate existing plaintext credentials to PasswordVault on first launch after upgrade, then delete the plaintext file.

### Security Note

The current plaintext `.credentials` file stores secrets (API tokens, enrollment tokens) in cleartext on disk. This is the highest-priority item in this section. Until PasswordVault is implemented, the file should at minimum be created with restrictive ACLs (owner-only read/write).

---

## 5. Crash Reporting (issue #20)

### Options

| Service | Free Tier | Platforms | Notes |
|---------|-----------|-----------|-------|
| **Sentry** | 5,000 events/month | All (Go, Swift, C#) | Most mature, cross-platform SDKs |
| **Apple CrashReporter** | Unlimited | macOS/iOS only | Requires App Store or TestFlight distribution |
| **Windows Error Reporting** | Unlimited | Windows only | Built-in but limited for custom apps |
| **Crashlytics (Firebase)** | Unlimited | iOS/Android/Unity | No desktop support |

### Recommendation: Sentry

Sentry provides the best cross-platform coverage:

- **Go agent**: Use `sentry-go` SDK. Initialize at agent startup, capture panics and fatal errors.
- **macOS agent (Swift)**: Use `sentry-cocoa` SDK via SPM. Captures crashes, unhandled exceptions, and breadcrumbs.
- **Windows agent (C#)**: Use `Sentry.Dotnet` NuGet package. Integrates with .NET unhandled exception handlers.

### Integration Plan

1. Create a Sentry project under a LabTether organization.
2. Configure a DSN per platform (or use a single project with environment tags).
3. Pass the DSN via environment variable (`LABTETHER_SENTRY_DSN`) or compile-time constant.
4. Initialize Sentry early in each agent's startup:
   - Set release version tag from build metadata.
   - Set environment tag (`production` / `development`).
   - Attach asset ID and hub URL as context (but not tokens or other secrets).
5. Disable Sentry when no DSN is configured (self-hosted users may not want telemetry).

### Privacy Considerations

- Crash reports should never include API tokens, enrollment tokens, or user credentials.
- PII scrubbing should be enabled in Sentry project settings.
- Consider making crash reporting opt-in for self-hosted deployments (off by default, enable via config flag).

---

## 6. TEMP Placeholder Labels (issue #35) -- RESOLVED

Fixed in `labtether/labtether-mac` commit `142f3be`:

- Renamed all `"TEMP"` labels to `"THERM"` across `MetricsView.swift` and `PopOutSystemSection.swift`.
- The label was ambiguous (could read as "temporary"). `THERM` clearly indicates thermal/temperature data and follows the existing short-caps label convention (`CPU`, `MEM`, `DISK`, `RX`, `TX`).

---

## Summary & Priority Order

| Priority | Issue | Item | Effort | Blocked On |
|----------|-------|------|--------|------------|
| 1 | #35 | TEMP labels | Done | -- |
| 2 | #34 | Windows PasswordVault | Small | Windows build environment |
| 3 | #8 | macOS code signing | Medium | Apple Developer Program membership |
| 4 | #9 | Windows installer (MSIX) | Medium | Windows SDK, code signing cert |
| 5 | #21 | Native wrapper auto-update | Medium | Code signing (must be signed first) |
| 6 | #20 | Crash reporting (Sentry) | Medium | Sentry account setup |
