# LabTether r28 installed-candidate QA — 2026-07-22

## Disposition

**PASS for the exercised exact-r28 installed Hub, browser, API, enrollment,
Docker, restart, and cleanup scope.** This does not constitute an official
release: signed/notarized native artifacts, public package visibility,
coordinated release publication, the remaining credential rotations, three
unproven recovery units, and a fresh continuous soak remain separate gates.

The installed source is GitHub-verified merge commit
`d7b84b7895d7e68eaca2ae5290fc38cb6e2f960d` from PR #106. Its tree
`9b86dcd89cf7a4f64351c17712a31c6d3993a1d1` exactly matches the reviewed PR
head. All 13 PR checks passed after the final trust-boundary repair; all 12
post-merge `main` checks also passed, including a clean 124/124 HTTPS browser
run and the full Go race suite.

## Exact image and deployment

- Clean source archive:
  `/Users/michael/Development/LabTether/.qa-builds/hub-d7b84b7895d7`
- Image: `labtether-installed-qa:20260722-r28`
- Image ID: `sha256:2b8f4e6d47abe82863c3c69e42c2982689ca1664973b65bf0d0a99d27b74de5a`
- OCI revision: `d7b84b7895d7e68eaca2ae5290fc38cb6e2f960d`
- OCI version and live `/version`: `20260722-r28`
- Trivy: zero HIGH/CRITICAL vulnerabilities and zero image secrets. The
  private JSON result is
  `/Users/michael/Development/LabTether/.qa-evidence/trivy-labtether-installed-qa-20260722-r28.json`.
- Pre-replacement PostgreSQL custom-format dump:
  `/Users/michael/Development/LabTether/.qa-evidence/labtetherqar17-pre-r28-d7b84b7895d7.dump`,
  3,885,640 bytes,
  SHA-256 `748c7e9bea4560af3a7c90600fcee890ba5aea1d823073aad4312de345c2c091`;
  `pg_restore --list` passed.

Only `labtether`, `web-console`, and `console-ingress` were recreated.
PostgreSQL retained container ID
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`,
start time `2026-07-22T09:25:33.948207088Z`, its data and volumes, restart
count zero, healthy state, and `OOMKilled=false`. All three application
containers use the exact r28 image and are healthy with restart count zero and
`OOMKilled=false`.

The image intentionally contains only `agent-dist/.keep`. Coordinated v1.5.1
agent-manifest generation still requires signed macOS and Windows release
artifacts; the QA image does not substitute unsigned or metadata-only bytes.

## Installed regressions closed

### `LTQA-273` — Home Assistant status consistency

The same Home Assistant controller
`homeassistant-hub-disposable-ha-qa-r17` rendered **Offline** in the installed
Devices list and **Offline** in its detail identity bar. A recent update time no
longer overrides an explicit unhealthy state. The fresh browser tab reported
zero console errors.

### `LTQA-274` — authoritative one-use event ticket

Against the real installed `/ws/events` endpoint with a still-valid owner
cookie:

- first use of a newly issued ticket upgraded with HTTP 101;
- exact replay returned HTTP 401;
- a syntactically invalid supplied ticket returned HTTP 401;
- an explicitly empty supplied ticket returned HTTP 401; and
- the no-ticket authenticated-cookie path still upgraded with HTTP 101.

This proves supplied ticket presence is authoritative and replay cannot fall
back to ambient browser authentication.

### `LTQA-275` — discovery and installer origin/trust

- Direct discovery returned `https://127.0.0.1:28443`,
  `wss://127.0.0.1:28443/ws/agent`, built-in-CA trust, and a fingerprint-bound
  bootstrap URL.
- Discovery through the installed console returned
  `http://127.0.0.1:23000`, `ws://127.0.0.1:23000/ws/agent`, plain-HTTP trust,
  and the normal installer strategy without an internal-CA bootstrap URL.
- Both generated `install.sh` payloads retained their exact request origin.
- The direct bootstrap CA fingerprint matched the retained CA certificate and
  emitted the correct CA-download and `--tls-ca-file` flow.

The final source also fails closed when the trusted-forwarding resolver returns
valid-looking values with `trusted=false`; that callback contract has a
dedicated regression test.

## Browser and API proof

A fresh authenticated route sweep returned HTTP 200, the correct LabTether
title, and zero console/page errors for all 19 main routes: Dashboard, Devices,
Topology, Services, Containers, Terminal, Files, Remote View, Logs, Alerts,
Actions, Webhooks, Schedules, Groups, Health, Users, Security, Audit Log, and
Settings. The owner session survived both application replacement and the
deliberate Hub restart. The Users page exposes an actual **Add User** button and
an **Add User** dialog with labeled username, password, confirmation, role,
cancel, and create controls; the earlier unnamed-dialog observation was a
snapshot-tool artifact, not a product defect.

## Released v1.5.1 agent and isolated Docker proof

The exact public Linux ARM64 v1.5.1 release was independently verified against
the GitHub asset digest/size, checksum, signed metadata, detached Ed25519
signature, and trusted project key:

- SHA-256 `fd61d44a19df09a9c7bd14d104ef3bf7c086f26f9475b70d526c5af7e99fe1f1`
- size 12,779,682 bytes
- metadata `v1.5.1`, `linux`, `arm64`

It ran as UID/GID 501 with a read-only root, all capabilities dropped,
`no-new-privileges`, strict private-CA TLS, a loopback-only local API, disabled
auto-update/WebRTC/log streaming/network scans, and no host Docker socket. The
current Mac LAN address no longer matched the retained server certificate's old
LAN SAN, so the disposable container used the certificate-covered `labtether`
DNS name mapped only to Docker's host gateway; verification was never skipped.

The one-use enrollment token had `max_uses=1`, reached `use_count=1`, was
removed from the state volume after success, and was explicitly revoked. The
durable 44-byte bearer was owned by 501:501 with mode 0600; enrollment state and
device identity files were present with bounded permissions. PostgreSQL held
exactly one active credential tied to the enrollment-token ID.

Docker-in-Docker 27.5.1 ARM64 used only three named disposable volumes and a
Unix socket shared with the agent. It had no host bind and no published TCP
port. Through LabTether APIs only, QA:

1. discovered the exact Docker host;
2. pulled immutable Alpine ARM64 digest
   `sha256:2c9d26f410d032d5b1525aa8a873e238b05b90c4ae8618743d4311f0cc827e37`;
3. created and started `ltqa-r28-inner` running a loopback ping;
4. read exact inventory, stats, and ping logs;
5. stopped, started, and restarted it, with Hub state and direct DinD readback
   agreeing after every transition; and
6. proved the exact inner name was absent from host Docker.

Restarting the disposable agent preserved its bearer-file hash and active
credential ID, did not recreate an enrollment secret, reconnected as v1.5.1,
and did not interrupt DinD or the inner workload.

## Deliberate Hub restart and stability watch

The exact Hub container restarted at `2026-07-22T11:00:36Z` while the released
agent and inner ping workload were active. Ordered shutdown logged complete
HTTP and runtime drains. Strict `/healthz` returned in one second; the agent and
Docker inventory returned in two seconds. The Hub retained its container ID,
PostgreSQL retained its ID/start/restart state, and the agent and DinD containers
did not restart. The active agent credential ID stayed unchanged and a fresh
post-reconnect Docker log request returned current ping output.

Eleven samples from `2026-07-22T11:02:49Z` through
`2026-07-22T11:07:51Z` all passed strict health, exact container identity,
restart-zero/OOM-free state, authenticated agent presence, Hub-to-DinD
roundtrip, host-socket isolation, and zero drain/lease/pool/runtime-ownership/
panic/fatal signatures. Evidence is in
`/Users/michael/Development/LabTether/.qa-evidence/r28-installed/stability-watch.jsonl`.

## Cleanup

The inner container was stopped and removed through LabTether. The agent was
then stopped, observed Offline, and decommissioned through exact
`DELETE /api/v2/assets/ltqa-v151-dind-r28`. PostgreSQL now contains zero active
credentials for that asset and one revoked audit credential. Both disposable
outer containers, all three named volumes, both networks, run-specific browser
state, cookie/ticket material, enrollment responses, and temporary action files
were removed. The original owner-auth state remains mode 0600 for continued
inspection. Exact-name reconciliation found no active/runtime residue. The
retained QA Hub stack remains healthy, and the PostgreSQL container and original
data volume remain preserved; only the intended run-scoped credential/asset
records changed during revoke and decommission. Expected consumed-token and
revoked-credential audit rows remain.

## Recovery and release boundary

Canonical recovery proof now passes Zeta VM 100, Zeta VM 101's boot/config
scope, and Theta Home Assistant VM 100. Delta VM 100, Delta VM 101, and Gamma
VM 100 remain `never_proven` with null `last_proven_recoverable`; destination
verification alone does not advance them. No additional restore was started
near the scheduled backup window.

The bounded config-only change to backup job `backup-5cbf16f7-7a25` retains
`all=1`, `enabled=1`, schedule `21:00`, storage `PBS-Storage`, mode `snapshot`,
and `exclude=102`. Rollback is
`pvesh set /cluster/backup/backup-5cbf16f7-7a25 --delete exclude`. The excluded
target was never queried, enumerated, or operated on.

Official release remains blocked on the six macOS signing/notarization secrets,
two Windows code-signing secrets, owner-controlled GHCR package visibility,
13 coordinated credential rotations, three recovery units, and a fresh soak
that starts only from the final published signed candidate. Home Assistant PR
#13 remains a green draft until the exact Hub v1.5.1 image exists.
