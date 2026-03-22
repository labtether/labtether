# Privacy Policy

Effective date: March 9, 2026

LabTether is designed first as self-hosted software. In the default deployment model,
the hub, database, agents, and connected infrastructure run under the control of the
operator who deploys them rather than under a LabTether-hosted SaaS service.

This page explains the main categories of data processed by:

- the self-hosted LabTether hub
- the current native companion surfaces, including the iOS app and macOS menu bar agent

## 1. Who Controls Data In A LabTether Deployment

For a self-hosted LabTether deployment, the operator who runs the hub is generally
the primary controller of the data collected through that deployment.

That means:

- infrastructure inventory, telemetry, logs, alerts, incidents, recordings, and audit data are stored in systems chosen by that operator
- connector credentials and remote-access targets are configured by that operator
- retention, backup, and access-control decisions are made by that operator

If you are using LabTether on behalf of a team, household, or client environment,
you are responsible for making sure your deployment and policies are appropriate for
that environment.

## 2. Data The Hub Processes

Depending on which features you enable, a LabTether hub may process:

- account and access data such as usernames, roles, local-auth records, session metadata, and optional OIDC identity claims
- infrastructure inventory such as asset names, hostnames, IPs, service metadata, connector object IDs, and platform details
- operational telemetry such as metrics, health snapshots, alert state, incidents, maintenance history, and audit logs
- remote-access metadata such as terminal and desktop session state, previous-session records, and workflow activity
- connector and integration data pulled from systems you choose to connect, such as Proxmox VE, Proxmox Backup Server, TrueNAS, Docker, Portainer, and Home Assistant

Because the product is self-hosted, this data normally stays inside the infrastructure,
network, and storage footprint selected by the operator.

## 3. Data Stored Locally On iPhone And iPad

The iOS app stores a limited set of data locally on the device to support login,
offline resilience, and operator preferences.

This can include:

- the configured hub URL and session token, stored in the iOS Keychain
- cached API responses used for faster reloads and limited offline viewing
- locally queued offline actions and audit state when offline action replay is enabled
- notification preferences, quiet hours, digest settings, Live Activity preferences, and other app settings stored in local app storage
- push-registration metadata for the active device and hub association

The iOS app requires origin-only hub URLs and uses `https` for non-loopback hubs.
An `Allow Untrusted TLS` compatibility toggle exists for self-signed-cert environments,
but it is disabled by default.

## 4. Data Stored Locally On macOS

The macOS menu bar agent stores operator-entered secrets in the macOS Keychain and
uses local runtime files only as needed to launch the embedded agent process.

Depending on configuration, the macOS surface may locally store:

- hub URL and local agent settings
- operator-entered secrets in Keychain-backed storage
- short-lived runtime files required to start the embedded `labtether-agent`

## 5. Optional Mobile Telemetry

The iOS app includes an optional `Share Mobile Telemetry` setting that is enabled by
default. When enabled, the app sends authenticated mobile observability events to the
configured LabTether hub.

These events are intended for product reliability and operator troubleshooting and can
include:

- API latency and error outcomes
- realtime reconnect state
- app lifecycle diagnostics related to the LabTether mobile client

This telemetry is best-effort, queued, and batched. In the current implementation,
it is sent to the configured hub's `POST /telemetry/mobile/client` endpoint rather
than to a separate LabTether-operated analytics service.

You can disable this setting from the iOS app under `Settings -> Behavior -> Share Mobile Telemetry`.

## 6. Push Notifications, Live Activities, And Lock-Screen Content

If you enable notifications on iOS:

- the app requests push-notification permission from Apple
- the device registers with Apple Push Notification service (APNs)
- the app forwards the device token and notification-routing preferences to your configured LabTether hub

Those preferences can include severity threshold, quiet hours, digest timing, and
category or toggle selections needed for delivery behavior.

If you enable Live Activities, LabTether may show incident or remote-session status on
the lock screen or Dynamic Island. The current default is redacted detail mode rather
than full-detail exposure. Full detail is an explicit operator choice in app settings.

## 7. Device Permissions Used By The iOS App

The iOS app currently uses these platform permissions:

- Local Network / Bonjour: to discover LabTether hubs on the local network
- Notifications: to receive alert and incident push notifications
- Face ID / Touch ID or equivalent device-owner authentication: to approve remote terminal and desktop actions when that setting is enabled

Biometric matching is handled by the operating system. LabTether uses the result of
that local device-owner approval flow; it does not receive raw biometric templates.

## 8. Third Parties And External Services

Depending on how you deploy LabTether, data may also be processed by services you choose
to use alongside the product, including:

- Apple services such as APNs and iOS/macOS system frameworks
- your identity provider when optional OIDC sign-in is enabled
- your connected infrastructure platforms and APIs
- your own reverse proxy, certificate, DNS, VPN, or remote-access stack

For example, if you use Tailscale, your Tailscale configuration and policies are governed
by Tailscale's terms and privacy practices rather than this document.

## 9. Retention And Deletion

Retention is primarily determined by the operator's deployment and configuration choices.
LabTether includes retention controls for several hub-side data classes, but the exact
retention behavior depends on how the deployment is configured.

On iOS sign-out, the app clears:

- the session token
- hub cookies and URL cache artifacts
- the mobile offline API response cache
- persisted offline queues and related audit state for supported mobile workflows
- push registration for the active hub/device association

The hub URL is retained for faster re-login unless the operator explicitly changes or
clears the saved hub configuration.

## 10. Security And Operator Responsibilities

LabTether's security posture, transport rules, and secrets-handling model are documented
in [docs/SECURITY.md](docs/SECURITY.md). Operators are responsible for:

- choosing appropriate authentication and access controls
- protecting TLS trust and certificates
- setting sane retention and backup policies
- redacting secrets before sharing logs or screenshots
- validating connected-system permissions and connector scopes

## 11. Contact

For general support, setup help, or non-security questions, see [SUPPORT.md](SUPPORT.md).

For suspected vulnerabilities, use the private reporting guidance in [SECURITY.md](SECURITY.md).

This page may be updated as the public release contract evolves. When that happens,
the effective date at the top of this file will change.
