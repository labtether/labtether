# Credential rotation closeout audit — 2026-07-16

Status: **attempted and safely rolled back; 0 of 13 rotated**. No secret value
is recorded in this file.

## Scope and evidence boundary

This audit closes the ambiguity around the two credential residuals in the full
installed-product QA plan. It identifies the conservative rotation set, the
actual rotation mechanism, the rollback order, and the evidence required before
an old credential can be revoked.

The exact purged `LTQA-028` log bytes no longer exist, so it is not possible to
prove which individual child credential appeared in which file. The safe scope
is therefore every credential reachable through the four affected MCPM
profiles, not a guessed subset.

## Conservative rotation set

| Provider | Credential count | Identities / endpoints | Current local owner |
|---|---:|---|---|
| Proxmox VE | 4 | Dedicated `mcp@pve!mcp` tokens for Delta, Gamma, Theta and Zeta | Four mode-`0600` files under `~/.config/proxmox-mcp/` |
| TrueNAS | 2 | API keys for OmegaNAS and TauNAS | `~/.config/mcpm/servers.json`; both values also occur in `servers.json.bak-portainer` |
| Portainer | 5 | API keys for OmegaNAS, TauNAS, ContainerVM-ZetaServer, ContainerVM-DeltaServer and BackupNAS | `~/.config/mcpm/servers.json`; the OmegaNAS value also occurs in `servers.json.bak-portainer` |
| Home Assistant MCP | 2 | Secret-path capability URLs for Simba and McCann Home Assistant | `~/.config/mcpm/servers.json`, `servers.json.bak-portainer`, and `backups/servers.json` |

Total conservative connector rotation set: **13 credentials**.

The current MCPM configuration was last modified on 2026-06-16 and the four
Proxmox credential files on 2026-06-15, all before `LTQA-028`. No post-incident
local rotation is evidenced.

## Rotation mechanisms and blockers

### Proxmox VE

Use an upstream administrator to create a sibling API token for the same
`mcp@pve` user and compare `pveum user token permissions` before changing the
client. Proxmox documents that token permissions are a subset of the backing
user and that a separated token needs its own ACLs. The old token must remain
valid until the replacement passes the same bounded node-status read through
MCPM.

Reference: <https://pve.proxmox.com/pve-docs/pve-admin-guide.pdf>

The `2026-07-22` upstream metadata read showed all four current tokens have
`privsep=0`, not separated ACLs. A parity-only replacement must preserve that
inheritance behavior; changing to separated least-privilege ACLs is a distinct
hardening change. The local files still provide only API-token authentication,
not the user/password or administrator session required to create and retire a
sibling token. All four rotations therefore remain blocked.

### TrueNAS

Create a new named key for the same upstream account, retain the returned key
only in the protected MCPM configuration, restart the TrueNAS bridge, and pass
`system_health` for both appliances before deleting the old key by ID. The
TrueNAS API returns the raw key only on creation; `api_key.create` and
`api_key.delete` require the documented API-key administration role.

References:

- <https://api.truenas.com/v25.10.0/api_methods_api_key.create.html>
- <https://api.truenas.com/v25.10/api_methods_api_key.delete.html>

The initial blocker was resolved by read-only upstream metadata on
`2026-07-22`: OmegaNAS used unrevoked key ID `5`, TauNAS used unrevoked key ID
`6`, both were owned by `root`, and both principals had `FULL_ADMIN`. The
collection advertised creation and each key item advertised deletion. Both
were therefore execution-ready, but the attempted OmegaNAS campaign below was
safely rolled back and TauNAS was not attempted.

### Portainer

Generate a second access token for the same user, update one endpoint at a time,
pass `systemStatus`, then remove the old key by its key ID. Portainer requires
the owning user's password when generating a token and returns the raw API key
only once. The current API keys alone are not sufficient for a safe create-first
rotation.

References:

- <https://docs.portainer.io/2.27/api/access>
- <https://api-docs.portainer.io/?edition=ce&version=2.39.0>

Read-only metadata on `2026-07-22` identified the current owners and token IDs:
OmegaNAS `michael` / `5`, TauNAS `michael` / `2`, ContainerVM-ZetaServer
`michael` / `3`, ContainerVM-DeltaServer `michael` / `4`, and BackupNAS `admin`
/ `1`. Each principal was role `1` (administrator), and each current API key
could pass `systemStatus` and list its token metadata. The five owning-user
password re-authentications are still unavailable and must not be guessed or
extracted, so none can safely create its replacement.

### Home Assistant MCP secret paths

Each add-on uses a persisted 128-bit secret path as the credential. The upstream
documented regeneration procedure is to remove only
`/data/secret_path.txt`, restart that add-on, copy the newly generated URL, and
update every client. Rotate one Home Assistant instance at a time; prove the new
MCP URL first, then prove the prior path is rejected.

Reference: <https://homeassistant-ai.github.io/ha-mcp/guide-addon/>

Blocker: regeneration is a remote add-on data mutation and restart. It must be
coordinated with both affected Home Assistant administrators and all clients
that consume the URL.

### MomentBackup UI automation token

This is a process-bound QA bearer, not an account credential. The owning MCP
generates 32 random bytes on every `app_launch`, passes the token only in the
child environment, and clears its token state after `app_quit` / child exit.
The safe rotation is therefore **the owning MCP's `app_quit`, followed by a new
`app_launch`**, never killing an arbitrary running MomentBackup process.

Source boundary:

- `/Users/michael/Development/MomentBackup/docs/ui-automation-mcp.md:34`
- `/Users/michael/Development/MomentBackup/app/scripts/automation/momentbackup-ui-mcp.mjs:208`
- `/Users/michael/Development/MomentBackup/app/scripts/automation/momentbackup-ui-mcp.mjs:249`

The normal `MomentBackup` LaunchAgent has no automation-token or automation-port
environment keys. Current ownership of any separately launched automation child
was not established, so no process was stopped and production MomentBackup was
not disturbed.

## Safe execution order

For each endpoint independently:

1. Record the upstream credential ID, owner and effective permissions without
   recording the raw value.
2. Create a replacement while retaining the old credential as rollback.
3. Atomically update the mode-`0600` current configuration and every protected
   backup copy that still contains the old value.
4. Restart only that provider's MCPM bridge.
5. Pass MCP initialization, tool discovery and one provider-specific bounded
   authenticated read.
6. Revoke the old credential upstream and prove it is rejected.
7. Re-run the bounded read with the replacement, confirm all four historical
   `mcpm-*.log` paths remain absent, and confirm no raw credential appears in
   evidence output.

Do not batch-revoke credentials and do not delete the old value before the new
path is proven. If any new-path check fails, restore the protected configuration
to the still-valid old credential and restart only that bridge.

## Current cleanup evidence

- `sh.mcpm.proxmox`, `sh.mcpm.homeassistant`, `sh.mcpm.truenas` and
  `sh.mcpm.portainer` launch definitions route both stdout and stderr to
  `/dev/null`; their environment key set is limited to `HOME`, `PATH`, and
  `HOMEBREW_NO_ENV_HINTS`.
- The former `~/Library/Logs/mcpm-{proxmox,homeassistant,truenas,portainer}.log`
  paths are absent.
- The 13 current connector values occur only in the protected configuration
  owners and the named protected backup copies above across the inspected
  LabTether/homelab/local-config roots; none occurs in the LaunchAgent files.
- A separate exact-value scan of retained private history found both current
  TrueNAS keys in one private Codex QA session log. Both Home Assistant secret
  paths occur in the user's shell history and in three private Codex session
  logs. No current Proxmox or Portainer value matched the retained shell/Codex/
  `~/Library/Logs` roots. These append-only/private-log matches do not make the
  values safe; they make upstream rotation mandatory. Raw values were not
  copied into this evidence.
- The separate pre-existing `mcpm-homelab.log` does not match any of the 13
  current values or the audited bearer/API-key assignment signatures. It was
  not deleted because it is outside the exact `LTQA-028` purge set.
- At this checkpoint all four MCPM bridge ports were unreachable. Therefore the
  previous read-only connector proof is historical, not current service-health
  proof, and no credential may be revoked until its bridge is deliberately
  started and the replacement passes.

## Authorized TrueNAS execution checkpoint — 2026-07-22

Michael authorized the listed rotations. Execution began with OmegaNAS only,
under the retain-update-restart-prove-revoke-prove contract. TauNAS was not to
start unless OmegaNAS completed through old-key rejection and replacement
re-proof.

Before mutation, all four MCPM LaunchAgents were running on loopback ports
`6276` through `6279`. All 13 current credential paths passed their bounded
reads. The two TrueNAS appliances ran version `25.10.4`; both `system_health`
calls returned `OK`. The two current TrueNAS values agreed between
`servers.json` and `servers.json.bak-portainer`, and both files were mode
`0600`.

Two OmegaNAS attempts failed closed before old-key revocation:

1. The first replacement was created and passed direct authenticated reads.
   Both protected configuration copies were atomically updated, and only
   `sh.mcpm.truenas` was restarted. The immediate post-restart MCP proof failed
   before revocation. The transaction restored both old configuration copies,
   restarted only the TrueNAS bridge, and deleted the uncommitted replacement.
2. A single instrumented retry repeated the create-first sequence. Replacement
   ID `7` passed direct authentication, both protected copies retained mode
   `0600`, and canonicalized collateral hashes proved no non-target entry
   changed. After the bridge restart, OmegaNAS progressed through its readiness
   path, but the peer TauNAS `system_health` call failed during bridge warm-up.
   Shell `errexit` handling exited before the intended bounded backoff could
   retry that transient stage. The fail-closed transaction again restored both
   old copies, restarted only the TrueNAS bridge, and deleted replacement ID
   `7`.

Final restored-state proof after the second attempt:

- OmegaNAS still uses unrevoked upstream key ID `5`; the configured key passed a
  direct API read with HTTP `200`. Replacement ID `7` and every replacement
  name from the two attempts were absent upstream.
- `servers.json` and `servers.json.bak-portainer` contain the same restored
  OmegaNAS value, both remain mode `0600`, and no rotation temporary file
  remains.
- `sh.mcpm.truenas` was running as PID `39924` with exactly one listener on
  `127.0.0.1:6278` at the checkpoint. MCP initialization, tool discovery, and
  `system_health` returned `OK` for both OmegaNAS and TauNAS after rollback.
- The historical `~/Library/Logs/mcpm-truenas.log` path remained absent. No raw
  credential was written to this evidence.
- TauNAS key ID `6` was not replaced, updated, or revoked. The other 11
  credentials were not mutated. VM 102 was not queried, enumerated, or touched,
  and Docker/Colima were not started.

The attempts establish a bridge-readiness gap: the MCPM HTTP listener can be
available before every aggregated stdio provider is ready for a bounded tool
call. A future campaign needs a reviewed proof harness whose retry loop is not
short-circuited by shell `errexit`, and must retain the old key until
initialization, discovery, and both TrueNAS health reads pass within the bounded
readiness window.

During the preceding read-only readiness inspection, a faulty redaction
selector emitted both current TrueNAS values into a private Codex tool-output
record. The values are not reproduced here. They were already in retained
private Codex history under the earlier audit, so the conservative rotation set
does not change, but upstream rotation remains mandatory.

## Closeout decision

Log-retention remediation remains effective. Credential rotation is **not
complete: 0 of 13 connector credentials are rotated**. The two authorized
OmegaNAS attempts preserved the old key and returned to the proven pre-change
state; TauNAS remains execution-ready but unattempted pending a corrected
bridge-readiness harness and fresh execution decision.

The other 11 credentials retain their external-authority blockers: four
Proxmox user/admin authentications, five Portainer password
re-authentications, and two coordinated Home Assistant add-on secret
regenerations with all consuming clients accounted for. Those credentials were
not mutated. After eventual upstream revocation, the exact Home Assistant
entries in shell history can be removed in a separately approved history
rewrite; private Codex session records must be treated as retained audit
records, so rotation rather than deletion is the security boundary.
