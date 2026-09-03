# LabTether security architecture and improvement plan

Last reviewed: 2026-09-03

This document defines the security rules that every LabTether repository must
keep, records the current audit baseline, and gives the order for closing the
remaining gaps. It complements the public vulnerability-reporting policy in
the repository root.

## Scope

LabTether is a privileged control plane. It can connect to hosts, run commands,
move files, collect telemetry, and install or update agents. A Hub compromise
can therefore become a fleet compromise.

The review covers these repositories:

- `labtether/labtether` (Hub and console)
- `labtether/labtether-agent`
- `labtether/labtether-cli`
- `labtether/labtether-homeassistant`
- `labtether/labtether-ios`
- `labtether/labtether-linux`
- `labtether/labtether-mac`
- `labtether/labtether-website`
- `labtether/labtether-win`
- `labtether/protocol`

The private `labtether/demo-repository` was visible in the organization list
but was not available in the local workspace. Its source still needs the same
review before the organization-wide audit can be called complete.

Production systems are not part of a source review. In particular, Proxmox VM
102 is excluded and must not be queried or used as test evidence.

## Security rules

These rules are release blockers:

1. Every remote request is authenticated before it reaches a privileged action.
2. Authorization is checked against the exact asset and action, not only the
   route or screen.
3. Agent identity is unique, revocable, and cannot be silently reused after
   decommissioning.
4. Hub-to-agent and browser-to-Hub traffic verifies the expected peer. Any
   insecure transport mode is explicit, narrow, visible, and off by default.
5. User-controlled addresses cannot reach forbidden local, link-local, metadata,
   or control-plane targets unless a policy explicitly allows that destination.
6. Commands use typed arguments and allowlists. User input is never joined into
   a shell command.
7. File paths stay inside their allowed roots, including after symlink and archive
   extraction.
8. Secrets are never committed, logged, returned in errors, or included in build
   artifacts. Stored secrets are encrypted and can be rotated.
9. Updates and installers verify version, platform, digest, and signature before
   execution.
10. Backups are encrypted, restorable, access-controlled, and exclude forbidden
    assets.
11. Security controls fail closed. A timeout, parser panic, or unavailable policy
    service must not silently grant access.
12. Logs record who did what to which asset, but never record the secret used.

## Current baseline

The 2026-09-03 pass used clean worktrees at each repository's current default
branch. It included GitHub advisory review, CodeQL and secret-scanning status,
workflow linting, dependency audits, Go vulnerability analysis, redacted secret
history scanning, and a broad static-analysis pass.

Important limits:

- The managed deep-security worker could not start because the desktop task did
  not provide its required read-only filesystem profile. Manual scans reduce the
  gap but do not replace that independent pass.
- Some GitHub security APIs are unavailable for private repositories.
- Static-analysis hits are leads, not confirmed bugs. Each needs a source-to-sink
  review before it is filed or fixed.
- This was source review only. It did not change production or inspect VM 102.

The first dependency snapshot contained 42 open Dependabot alerts: 10 in Hub,
12 in the website, 15 in the Windows agent, 3 in Home Assistant, and 2 in the
Linux agent. The first repair batch addresses the vulnerable versions that were
installed or selected by release builds.

Confirmed findings in that batch:

- Hub SSH clients reached vulnerable `x/crypto/ssh` parsing code when connecting
  to a malicious or compromised peer.
- Hub SMB directory reads used an unmaintained library whose response parser
  could panic the Hub on a crafted server reply.
- Hub and website JavaScript lockfiles selected vulnerable transitive packages.
- The Windows agent could publish with a vulnerable .NET 8 runtime pack, and an
  MSBuild-only path could bypass the repository SDK selection.
- Home Assistant and Linux development/runtime dependencies needed patched
  releases.
- The Go agent imported a vulnerable crypto module, although its current call
  graph did not reach the reported vulnerable functions.

Other observed gaps:

- Default-branch rules did not require approving reviews, strict status checks,
  or signed commits in the inspected public repositories.
- Secret scanning or push protection was disabled for the macOS repository and
  push protection was disabled for the protocol repository.
- A redacted history scan produced candidates in Hub and website history. Most
  look like fixtures, examples, generated files, or secret-name references, but
  every candidate needs classification before it is dismissed.
- Several production files exceed 1,000 lines. Large security-sensitive files
  make review harder and allow unrelated behavior to become coupled.

## Remediation program

### P0: restore a clean dependency and CI baseline

Target: now, before other feature work is merged.

- Land the patched Hub SSH and SMB dependencies.
- Land the Hub and website JavaScript dependency updates and require a zero-exit
  dependency audit in CI.
- Require .NET runtime 8.0.30 or newer in every Windows release path and inspect
  the published artifact's runtime metadata.
- Land the Home Assistant, Linux, and Go-agent dependency updates.
- Re-run native package audits, Go vulnerability checks, CodeQL, build, unit,
  race, and release-smoke jobs on the review branches.
- Reconcile each of the 42 alerts after GitHub refreshes them. Close only alerts
  whose vulnerable version is no longer present in the relevant build graph.

Exit gate: all required CI is green; no reachable known high/critical issue is
open; every remaining alert has a written disposition and owner.

### P1: protect the control plane's trust boundaries

Target: next two engineering cycles.

Run focused reviews, one boundary at a time:

1. **Enrollment and identity**: first enrollment, re-enrollment, key rotation,
   duplicate identity, revoked agent reconnects, decommission, and stale sessions.
2. **Authentication and authorization**: owner bootstrap, OIDC, cookies, API
   tokens, WebSocket upgrades, tenant/asset ownership, admin-only actions, and
   audit-log coverage.
3. **Remote execution**: terminal, saved actions, jobs, PowerShell, shell, SSH,
   cancellation, output limits, environment variables, and command allowlists.
4. **Outbound connections**: HTTP probes, web services, SSH/SFTP, SMB, FTP,
   proxies, redirects, DNS rebinding, IPv4/IPv6 private ranges, and cloud metadata.
5. **File operations**: traversal, symlinks, archive extraction, recursive delete,
   overwrite rules, permission preservation, quotas, and partial transfers.
6. **Agent transport**: TLS/WSS verification, certificate pinning or trust roots,
   replay resistance, message size limits, sequencing, reconnect behavior, and
   malformed protocol messages.
7. **Update and install paths**: manifest provenance, checksums, signatures,
   downgrade prevention, platform matching, rollback, and privileged service setup.
8. **Backup and restore**: encryption, integrity, restore authorization, retention,
   secret handling, and explicit VM 102 exclusion for any separately authorized
   production configuration work.

Each review must produce a small threat model, reproducible evidence, regression
coverage, and either a fix or a documented accepted risk. High-impact parser and
network findings should also receive a bounded hostile-peer test.

Exit gate: every boundary has an owner, tests for its security rules, and no
untracked high-risk finding.

### P1: secrets and repository governance

Target: in parallel with the trust-boundary work.

- Classify every redacted history-scan candidate without printing its value.
- Revoke and rotate any confirmed credential before rewriting history.
- Keep only clearly fake fixtures; mark them with narrow scanner allowlist rules.
- Enable secret scanning and push protection for every repository that supports
  them, including macOS and protocol.
- Protect each default branch with at least one approval, required current CI,
  dismissal of stale approvals, conversation resolution, and no force pushes.
- Add `CODEOWNERS` for authentication, authorization, protocol, update, release,
  and deployment paths.
- Pin third-party GitHub Actions to reviewed commit SHAs and schedule dependency
  update review.
- Enable private vulnerability reporting where supported.

Exit gate: a test push of a safe fake secret is blocked, default branches reject
an unreviewed change, and repository settings are recorded as evidence.

### P2: release and supply-chain proof

Target: after P0 and P1 are stable.

- Produce an SBOM for each shipped binary, container, app, and website worker.
- Generate provenance that binds source commit, builder, dependencies, and output
  digest.
- Keep native signing secrets local and outside every repository and build context.
- Verify signatures and checksums again after upload, from the public artifact.
- Add malicious-package, wrong-platform, wrong-version, missing-asset, downgrade,
  and interrupted-release tests.
- Preserve the staged release order: publish and verify prerequisites first, then
  allow Hub release generation.
- Define an emergency revoke-and-republish procedure that does not bypass normal
  verification.

Exit gate: a release can be traced from reviewed commit to verified public bytes,
and a bad or missing prerequisite prevents Hub publication.

### P2: reliability and abuse resistance

Target: after the highest-risk boundary reviews.

- Put explicit size, time, concurrency, and output limits on parsers, uploads,
  downloads, logs, terminal streams, WebSockets, and discovery jobs.
- Fuzz protocol decoding, archive/file parsing, URL handling, SSH/SFTP, and SMB
  adapters.
- Ensure cancellations close sockets, child processes, goroutines, file handles,
  and temporary data.
- Add rate limits and backoff to login, enrollment, token creation, remote command,
  file-transfer, and notification endpoints.
- Test restart and partial-failure behavior for jobs that can change hosts.

Exit gate: hostile inputs cannot cause an unbounded allocation, process-wide
panic, stuck privileged job, or silent retry storm.

### P3: reduce code-review risk

Target: ongoing, in small behavior-preserving changes.

Start with large, security-sensitive files such as Hub database migrations,
telemetry persistence, status state, terminal/remote-view components, protocol
messages, and the large native-agent views and clients.

- Split by responsibility and trust boundary, not by arbitrary line count.
- Move parsing and validation into small pure functions with table tests.
- Replace repeated route checks and stringly typed permissions with shared policy
  functions and typed capabilities.
- Record dependency direction so UI, transport, policy, and persistence do not
  call through each other.
- Add file-size and complexity reporting as information first; make it blocking
  only after the worst files have been reduced.

Exit gate: privileged changes can be reviewed without reading unrelated UI,
transport, or persistence logic.

## Proof matrix

Every security change should run the smallest relevant checks plus these release
gates:

| Repository | Required proof |
|---|---|
| Hub | Go tests and race checks, `govulncheck`, console test/lint/build, dependency audit, CodeQL, container/release smoke |
| Go agent and Linux | Go tests and race checks, `go vet`, `govulncheck`, cross-platform builds, release-contract checks |
| Protocol and CLI | Go tests, `go vet`, `govulncheck`, backward/forward compatibility fixtures |
| Website | supported Node/npm install, dependency policy, test/lint/build, Cloudflare worker runtime smoke |
| Home Assistant | supported Python matrix, component/add-on tests, dependency audit, packaging validation |
| Windows | Windows-host build/test/package, runtime metadata inspection, signature/install/uninstall proof |
| macOS and iOS | build/test on supported Xcode, entitlement review, archive/signature inspection, update/install proof where applicable |
| All repositories | workflow lint, secret scan, dependency alerts, protected-branch checks, artifact inventory |

## Finding records and cadence

Keep one private record per confirmed vulnerability. Include affected commit,
entry point, required actor, impact, proof, fix, regression test, owner, target
date, and release/backport status. Do not put exploit details or secrets in public
issues.

- Review new high/critical alerts immediately.
- Review dependency and secret dashboards weekly.
- Run the full cross-repository audit monthly and before a coordinated release.
- Re-run the managed deep scan when its filesystem profile is available.
- Review this document after a new remote-control feature, auth design, deployment
  model, supported platform, or security incident.

The program is complete only when the proof gates pass. A scanner count of zero
by itself is not proof that LabTether is secure.
