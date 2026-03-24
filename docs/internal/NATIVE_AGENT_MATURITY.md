# Native Agent Maturity Tracking

Status of native agent features required for GA release.

## macOS Agent (mac-agent)

| Feature | Status | Blocker |
|---------|--------|---------|
| Universal binary build | Done | — |
| Code signing (Developer ID) | Scaffolded in CI | Need Apple Developer Program enrollment ($99/yr) |
| Notarization | Scaffolded in CI | Requires code signing first |
| Sparkle auto-update | Not started | Need appcast.xml hosting |
| Crash reporting (Sentry) | Not started | Need Sentry DSN |
| Fix "TEMP" placeholder labels | Not started | MetricsView.swift, PopOutSystemSection.swift |

### Required Secrets for Code Signing
- `APPLE_CERTIFICATE_P12` — Developer ID Application certificate (base64)
- `APPLE_CERTIFICATE_PASSWORD` — Certificate password
- `APPLE_ID` — Apple ID email
- `APPLE_TEAM_ID` — Team ID from Apple Developer portal
- `APPLE_APP_PASSWORD` — App-specific password for notarytool

## Windows Agent (win-agent)

| Feature | Status | Blocker |
|---------|--------|---------|
| .NET build + zip release | Done | — |
| MSI/MSIX installer | Not started | Need WiX Toolset or MSIX packaging |
| Code signing (Authenticode) | Not started | Need EV code signing certificate |
| Windows service registration | Not started | Needs installer |
| Auto-update mechanism | Not started | — |
| Crash reporting (Sentry) | Not started | Need Sentry DSN |

### Required Secrets for Code Signing
- `WIN_CERTIFICATE_PFX` — Authenticode signing certificate (base64)
- `WIN_CERTIFICATE_PASSWORD` — Certificate password

## iOS App (ios)

| Feature | Status | Blocker |
|---------|--------|---------|
| Swift build + test CI | Done | — |
| App Store distribution | Not started | Need Apple Developer enrollment |
| Crash reporting | Not started | Need Sentry/Firebase DSN |
| Push notifications | Not started | Need APNs configuration |

## Agent Update Mechanism

### Go Agent (cross-platform)
The Go agent has a complete self-update mechanism:
- Polls hub `/api/v1/agent/releases/latest` on startup
- Hub pushes `update.request` over WebSocket when outdated
- Downloads, SHA256-verifies, atomically replaces binary
- Supports Ed25519 signature verification
- Controlled by `LABTETHER_AUTO_UPDATE` env var

### macOS Agent
Currently bundles the Go agent binary. Update mechanism:
- Go agent self-updates independently
- macOS wrapper (Swift app) has no auto-update
- **Recommended:** Integrate Sparkle framework for wrapper updates

### Windows Agent
Currently builds standalone .NET executable. Update mechanism:
- No auto-update implemented
- **Recommended:** Use ClickOnce or custom update service

## Priority Order
1. macOS code signing (unblocks macOS distribution)
2. Windows installer (unblocks Windows distribution)
3. Crash reporting (all platforms)
4. Auto-update for native wrappers
5. Fix macOS UI placeholders
