# Proxmox Backup Server full user QA — 2026-07-15

## Outcome

LabTether's PBS integration was exercised through the installed web console, Hub API, and a protocol-faithful disposable HTTPS PBS target. The QA went beyond unit tests: onboarding, TLS trust, authentication failure, discovery, inventory, task drill-down, confirmations, cancellation, mutable operations, upstream failure, recovery, and Hub restart persistence were all driven through the browser.

The initial production PBS integration inspection was read-only: no production
backup, snapshot, namespace, job, maintenance setting, or task was changed, and
all integration-workflow mutations targeted only the disposable PBS emulator.
A later, separately authorized recovery campaign performed destination-local
verification and isolated proofs under the canonical recovery contract. Current
canonical state passes Zeta VM 100, Zeta VM 101's bounded boot/config scope, and
Theta Home Assistant VM 100; Delta VMs 100/101 and Gamma VM 100 remain
`never_proven` with null `last_proven_recoverable`. The prohibited target was
never queried, enumerated, or acted on.

Final-candidate destructive route re-certification is recorded in the r21 and r22 sections below. r21's functional PBS workflows passed, but its sustained restart gate exposed a runtime-cancellation defect. The repaired r22 image subsequently passed a fresh cache-disabled installed smoke, the same deliberate restart gate, and a sustained five-minute post-restart observation without the r21 failure signature.

The current installed candidate is exact merged r28. It preserves the proven
PBS destructive-route, failure/recovery, and restart work while closing three
separate installed trust/state defects and recertifying browser/API, released
agent, isolated Docker, restart, stability, and cleanup boundaries. That
integration result remains separate from canonical recovery proof.

## r28 installed-candidate regression — 2026-07-22

GitHub-verified merge commit
`d7b84b7895d7e68eaca2ae5290fc38cb6e2f960d` built as
`labtether-installed-qa:20260722-r28`, image ID
`sha256:2b8f4e6d47abe82863c3c69e42c2982689ca1664973b65bf0d0a99d27b74de5a`.
All 13 PR checks and all 12 post-merge main checks passed. A custom-format
pre-replacement PostgreSQL dump (3,885,640 bytes, SHA-256
`748c7e9bea4560af3a7c90600fcee890ba5aea1d823073aad4312de345c2c091`)
passed `pg_restore --list`; app-only replacement preserved PostgreSQL's exact
container identity, start time, database, volumes, healthy state, restart count
zero, and `OOMKilled=false`. Trivy reported zero HIGH/CRITICAL vulnerabilities
and zero image secrets.

- A fresh authenticated browser sweep loaded all 19 main routes with HTTP 200,
  the correct title, and zero console/page errors. One Home Assistant controller
  rendered explicit **Offline** consistently in both list and detail views.
- A real `/ws/events` matrix accepted a new one-use ticket once, rejected replay
  and invalid/empty supplied tickets with HTTP 401 despite a valid owner cookie,
  and retained cookie fallback only when no ticket was supplied.
- Direct discovery/installers retained
  `https://127.0.0.1:28443` / `wss://127.0.0.1:28443/ws/agent` plus the built-in
  CA contract. Console-proxied discovery/installers retained
  `http://127.0.0.1:23000` / `ws://127.0.0.1:23000/ws/agent` without an internal
  CA bootstrap. Malformed, ambiguous, or resolver-untrusted forwarding fails
  closed.
- The exact public Linux ARM64 agent v1.5.1 enrolled through strict private-CA
  TLS with a one-use token, retained only its bounded durable credential,
  reconnected after agent and Hub restarts, and drove an isolated pinned DinD
  container through inventory, stats, logs, and reversible lifecycle. Host
  Docker had no inner container and production Docker was untouched.
- The deliberate Hub restart completed ordered HTTP/runtime drains; strict
  health returned in one second and agent/Docker inventory in two seconds.
  Eleven samples over five minutes stayed healthy, restart-zero, OOM-free, and
  free of lease/drain/pool/runtime-ownership/panic/fatal signatures.
- Product decommission/revoke and exact cleanup removed the run-specific inner
  workload, agent, active credential, outer containers, three named volumes,
  two networks, and private browser/ticket state with zero active/runtime
  residue. Expected consumed-token and revoked-credential audit rows remain.
  The original mode-0600 owner-auth state remains for inspection.

Full r28 evidence is in
`./2026-07-22-r28-installed-candidate-qa.md`. The failed r27 24-hour checkpoint
below remains unproven; r28's loaded restart and five-minute watch do not replace
a fresh continuous soak on the final published signed candidate.

## Safety model

- Production no-delete boundary: read-only status, configuration, targeted
  task-log, datastore-capacity, and expected-group checks only. No production
  backup, snapshot, namespace, datastore, job, or credential was deleted.
- Disposable target: `/tmp/labtether-pbs-qa`, HTTPS on port 18007, initialized with one datastore, one backup group, a protected healthy snapshot, an unprotected failed snapshot, verify/prune/sync jobs, a traffic rule, and a certificate.
- Disposable credentials and private key remained in mode-0600 files. The request ledger records only whether authorization was present, never the credential value.
- The integration QA performed no broad production metadata walk or restore
  mutation. The later separately authorized recovery campaign is documented
  below; it advanced only Zeta VM 100, Zeta VM 101's bounded boot/config scope,
  and Theta VM 100 under the canonical contract.

## Production read-only observations

Observed live on 2026-07-15 at approximately 22:18 AEST:

| Check | Simba PBS | McCanns PBS |
|---|---|---|
| PBS version | 4.2.2 | 4.2.2 |
| PBS services | active | active |
| Datastore capacity | 3.7 TiB total, 1.8 TiB used, 1.9 TiB available, 50% | 3.6 TiB total, 353 GiB used, 3.2 TiB available, 10% |
| Scheduled integrity | Tuesday 03:00 | Wednesday 03:00 |
| Verification policy | `ignore-verified=true`, `outdated-after=30`, read threads 1, verify threads 2 | same |
| Pull safety | `verified-only=true`, `remove-vanished=false`, `transfer-last=3`, inbound limit 20 MiB/s, worker threads 1 | same policy where configured |

Current execution evidence:

- McCanns Wednesday verifier started at 03:00, covered eight groups, retained zeta/theta snapshots with zero errors, and finished `TASK OK`.
- Simba zeta pull ran from 06:00 to about 08:43, transferred two groups with three snapshots each, 102.059 GiB / 54,731 chunks at about 10.652 MiB/s, and finished `TASK OK`.
- Simba theta pull ran from 09:00 to about 10:53, transferred the latest three snapshots, 70.199 GiB / 39,054 chunks at about 10.581 MiB/s, and finished `TASK OK`.
- The six expected targeted groups were present: McCanns `offsite-delta` VM 100/101 and `offsite-gamma` VM 100; Simba `offsite-zeta` VM 100/101 and `offsite-theta` VM 100.
- No PBS task was active at the time of inspection.

This proved only that the replication and verification jobs were configured and
that the observed runs completed; at this initial inspection it did **not**
prove recovery. The later, separately authorized VM-100 destination-local
verification and isolated restore supersede that open state for that one
recovery unit only, as recorded below.

One incidental read-only task-list observation was material: the production
all-VM daily job lacked the required exclusion. A later explicitly authorized,
config-only correction added `exclude=102` while preserving `all=1`,
`enabled=1`, schedule `21:00`, storage `PBS-Storage`, and mode `snapshot`.
The prohibited target itself was not queried or acted on; exact correction and
rollback evidence is recorded below.

## Installed console workflows exercised

### Onboarding and transport security

- Fresh first-run setup and owner login succeeded.
- PBS fields were usable through their visible labels, including the custom CA PEM field.
- A self-signed server without a supplied CA failed closed with an unknown-authority TLS error.
- A supplied CA plus deliberately bad API-token credentials was rejected with HTTP 401.
- Explicitly choosing skip-verification with the correct credential connected successfully.
- The safest path, supplied CA with verification enabled, connected successfully and reported PBS 4.2.2.
- Saving the connector started and completed discovery; the PBS root asset was Online and its datastore inventory appeared.

### Read workflows

- Overview displayed version, endpoint, aggregate storage, backup-group/snapshot counts, and task summary.
- Datastore expansion and drill-down were keyboard accessible and showed capacity, groups, and snapshots.
- Backup Groups and Snapshots inventory loaded explicitly by datastore.
- Verification displayed both healthy and failed results and `Outdated After 30d`.
- Prune policy decoded the real PBS hyphenated fields as last 3, daily 7, weekly 4.
- Sync policy displayed the remote store, `Verified Only: Yes`, and `Transfer Last: 3`.
- Traffic Control displayed network and rate values from the real PBS response shape.
- Certificate subject, issuer, fingerprint, SAN, and expiry rendered correctly.
- Tasks exposed one named set of filters, selectable task details, status, exit status, and logs.

### Mutable workflows on the disposable PBS target

For every destructive or operational action, the browser confirmation and cancellation path was checked before confirmation. The emulator ledger then verified the actual upstream method/path/form.

- Datastore verification: cancellation sent no request; confirmation started a PBS verification task and surfaced the UPID/status.
- Maintenance: enter and exit both required confirmation and sent exact PBS forms (`maintenance-mode=read-only` and `delete=maintenance-mode`).
- Garbage collection: confirmation started the correct datastore GC endpoint.
- Backup-group forget: datastore/type/id had to be selected from loaded inventory, followed by review and a second confirmation.
- Snapshot verification and forget: actions were inventory-backed; the protected snapshot's forget selection stayed disabled; bulk forget filtered protected rows.
- Verification job: run and delete succeeded with confirmation and user-visible status.
- Prune job: run and delete succeeded; simulation exposed a missing backend route during QA and was implemented as a PBS dry run with the exact namespace and retention policy.
- Sync job: run and delete succeeded with confirmation and user-visible status.
- Traffic-control rule: delete succeeded with confirmation and user-visible status.
- Running task: stop cancellation sent no request; confirmation stopped the task and surfaced success.

### Failure, recovery, and persistence

- Injected PBS upstream failure produced an honest error/empty state rather than stale success data.
- Clearing the failure restored version/details on refresh.
- Immediately after the intentional Hub restart, authentication, connector configuration, asset identity, Home Assistant inventory, Windows-agent connectivity, and PBS discovery all repopulated correctly. This was only an initial persistence pass, not the final restart result.
- Roughly five minutes later the runtime lease was lost. Cancellation stopped the runtime loops, but the active PBS collector continued starting sequential status/groups/snapshots calls after cancellation. The tracked drain timed out at 30 seconds; older shutdown code then released the lease and closed PostgreSQL while those writes were still running, producing closed-pool errors until the process exited fatally. The installed r21 restart gate therefore **failed**.
- Source now stops a PBS pass between every best-effort network and persistence phase when its context is canceled, short-circuits already-canceled PBS client requests, keeps collector executions in the shutdown drain, and suppresses the untracked post-run link-suggestion write for canceled passes. No replacement image/restart was performed in this lane after the hold instruction; live proof remains open for r22.
- A stale, already-open pre-upgrade console tab continued to issue an older mutation contract after deployment. A brand-new cache-disabled tab loaded the current contract. Compatibility translation was added for legacy snapshot/group forget POSTs so an open tab does not silently break across an upgrade.

## Defects found and repaired

1. PBS Add Device controls lacked reliable label associations and a safe custom-CA path.
2. Destructive PBS actions lacked consistent confirmation, cancellation, and result feedback.
3. Snapshot/group destructive forms allowed free-form identifiers instead of inventory-backed selection.
4. Protected snapshots could be selected by bulk-delete logic even though the individual checkbox was disabled.
5. PBS job fields used friendly names while real PBS responses use hyphenated names, hiding retention, remote-store, verified-only, transfer-last, network, and outdated-after values.
6. Datastore verify and maintenance console actions had no complete Hub route.
7. Prune simulation was presented in the console but had no backend implementation. It now resolves the selected job and issues `prune-datastore` with `dry-run=1`, namespace, and retention settings.
8. Completed tasks still presented an enabled Stop Task button. They now render a disabled `Task Finished` control while running tasks retain Stop Task.
9. Snapshot verification's Next.js proxy dropped the `store` query parameter, causing the installed app to return HTTP 400 even though the React component sent the correct URL.
10. Snapshot/group forget needed both a current DELETE query contract and translation of the older open-tab POST JSON contract.
11. Duplicate task-filter controls and unnamed keyboard controls reduced accessibility and user confidence.
12. The Hub dispatcher classified every `groups` path as a legacy read-only route before the `groups/forget` action could run, so the installed console received HTTP 405. The dispatcher now leaves the base groups endpoint read-only while allowing the explicit forget sub-route.
13. A successful backup-group forget left the destructive form open and the deleted group visible until a manual refresh. Success now closes and resets the form and immediately refreshes backup coverage.
14. Runtime cancellation did not terminate the PBS collector's best-effort call chain: after lease loss it started additional PBS requests, exceeded the 30-second drain, and allowed post-cancellation/untracked writes to reach a closing pool. The collector now checks cancellation before and after every phase, the client rejects an already-canceled request before URL validation, canceled runs do not schedule link-suggestion work, and collector admission/drain accounting remains atomic.

## Source and automated regression evidence

The source changes are backed by these scoped checks:

- `go test ./internal/connectors/pbs ./internal/hubapi/pbs ./cmd/labtether`
- PBS console Vitest plus proxy mutation-route regressions: 2 files, 10 tests.
- `npm run -s tsc -- --noEmit`
- Scoped ESLint for all changed PBS console/proxy/test files.
- `git diff --check` scoped to the PBS change set.
- `npm run test:e2e -- e2e/console.spec.ts -g 'add device pbs flow'`: 2 browser tests passed.
- `go test ./internal/connectors/pbs ./internal/hubapi/collectors -count=1`
- `go test -race ./internal/connectors/pbs ./internal/hubapi/collectors -count=1`
- `go test ./cmd/labtether -run 'TestShutdownHubRuntime|TestReleaseHubRuntimeLease|TestCloseHubPostgresStore' -count=1`

The route tests explicitly cover current snapshot verify, current snapshot delete, legacy snapshot POST translation, and current plus legacy backup-group forget. The cancellation regression blocks a PBS usage request, cancels with the real runtime-lease-loss cause, and proves the tracked drain closes with zero follow-on PBS calls, zero active collector runs, and no post-run link-suggestion write.

## r21 installed candidate (superseded)

The r21 installed candidate was `labtether-installed-qa:20260715-r21`, digest `sha256:9f3464765d1099f44bfb7b5e68790b97d3bc5c5d271f9f28314deafb47518a3e`. It was loaded in a brand-new cache-disabled browser tab, not inferred from source tests or an already-open console bundle.

Installed re-certification passed:

- Snapshot verify reached PBS as `POST /api2/json/admin/datastore/qa-store/verify` with the selected datastore context and visible success.
- The protected snapshot remained ineligible for forget. The disposable unprotected snapshot reached PBS as exact `DELETE /api2/json/admin/datastore/qa-store/snapshots?backup-type=vm&backup-id=900&backup-time=1784001600`, produced visible success, and disappeared from inventory.
- Prune simulation showed a dry-run result and reached `POST /api2/json/admin/datastore/qa-store/prune-datastore` with `dry-run=1`, last 3, daily 7, and weekly 4.
- The completed verification task rendered disabled `Task Finished` rather than an actionable stop control.
- Backup-group forget selected `qa-store — vm/900` from discovered inventory, required review plus explicit permanent-removal confirmation, and reached exact authenticated `DELETE /api2/json/admin/datastore/qa-store/groups?backup-type=vm&backup-id=900`.
- After group success, the installed r21 console immediately closed the form, removed the `vm/900` row, refreshed to `qa-store 0 groups` / `No groups in this datastore`, and retained the visible success status. The emulator ledger showed fresh datastore/groups GETs after the DELETE.

Candidate disposition: **NO-GO as the final release candidate** because the later sustained restart gate failed as described above. The destructive PBS UX/API surface is installed-proven on r21; the cancellation/drain repair is source-proven only until an r22 image passes the same restart and sustained-runtime observation without a drain timeout, closed-pool tail, fatal exit, or unexpected container restart.

## r22 final installed re-certification

The replacement deployment reported `/version` `20260715-r22`. All three application containers used exact image ID `95e215d40f29`; Docker identified the Hub image as `sha256:95e215d40f293fa5f41a757d65ae487f232ac9fbf6531bd2398c5aa89958c0a9`. PostgreSQL state was preserved.

Before restart, a fresh cache-disabled installed browser tab proved:

- The existing owner session survived the upgrade and the disposable PBS asset loaded Online without the r21 `Refreshing...` stall.
- Overview reported PBS 4.2.2, one healthy `qa-store`, 256.0 GB / 1.0 TB used (25%), zero groups, and zero snapshots after the earlier disposable destructive tests.
- Backup Groups honestly showed `qa-store 0 groups` / `No groups in this datastore`; the deleted group did not reappear.
- Verification job `weekly-integrity`, prune job `qa-retention`, and sync job `qa-offsite` rendered their exact schedules and policies.
- Completed-task drill-down showed `stopped`, `OK`, `TASK OK`, and disabled `Task Finished`.
- Prune simulation produced visible success and the emulator ledger recorded one authenticated `POST /api2/json/admin/datastore/qa-store/prune-datastore` with `dry-run=1`, `keep-last=3`, `keep-daily=7`, and `keep-weekly=4`.

One return from Tasks to Overview briefly rendered the header as Offline before a manual Overview refresh immediately restored Online. The database simultaneously reported the PBS asset online and its collector `ok` with no error. This did not recur during or after the restart, but is retained as an honest UI-state watch observation.

Exactly one deliberate `docker restart labtetherqar17-labtether-1` was issued:

- Command start and end: `2026-07-15T15:31:50Z` UTC.
- Old `StartedAt`: `2026-07-15T15:24:57.931758522Z`.
- New `StartedAt`: `2026-07-15T15:31:50.866540854Z`.
- Shutdown logs were correctly staged: `shutdown initiated` at `15:31:50.619145289Z`, `HTTP connection drain complete` at `15:31:50.621139783Z`, then `runtime shutdown complete` at `15:31:50.621148950Z`.
- The new collector runner started at `15:31:51.029421717Z`, the Hub listened at `15:31:51.033869747Z`, and Windows reconnected at `15:31:51.421316514Z`.
- The Hub first reported healthy at `15:32:17Z`.

Post-restart installed proof passed:

- PBS stayed Online; Overview and an explicit Refresh returned version/details immediately with no stuck loading or refreshing state.
- Prune Jobs and Tasks reloaded, and completed-task drill-down retained disabled `Task Finished`, `stopped`, `OK`, and `TASK OK`.
- A post-restart PBS collector pass completed `ok` with no error at `15:32:46Z`; another completed at `15:37:06Z`.
- Home Assistant was Online with 17 entities in the installed console and 17 scoped child assets in PostgreSQL.
- Windows was Online and Agent Connected with current CPU/memory/disk/network telemetry; PostgreSQL retained `group_id = NULL` and exact `agent_version = qa-20260715-r19-win`.

At `2026-07-15T15:37:20Z`, more than five minutes after the deliberate restart, the Hub remained running and healthy with unchanged `StartedAt`, Docker `RestartCount=0`, PBS collector `ok`, and PBS asset Online. Logs contained none of `runtime shutdown drain timed out`, lease release failure, closed pool, fatal, panic, or `runtime_ownership`. The final cache-disabled browser load still rendered Online PBS details immediately. Chrome emitted its own `runtime.lastError: Could not establish connection. Receiving end does not exist` during controlled navigation; CDP identified it as extension/runtime messaging noise rather than a LabTether application exception.

Candidate disposition: **PASS for the r22 LabTether PBS integration and restart gate**. This supersedes the r21 no-go. It does not change the separate production recovery-proof boundary below.

## r23 follow-up installed candidate deployment — 2026-07-16

The source repairs discovered during the follow-up user-level pass were built
into exact local image `labtether-installed-qa:20260716-r23`, image/digest
`sha256:2d74dadc75671be6a120e29bf6ccd62b50a64527800ad457cc9b6a6f1c28cba9`.
The build used the production all-in-one `build/Dockerfile`, Linux arm64,
`APP_VERSION=20260716-r23`, and a no-cache dependency/build path. `npm ci`
reported zero vulnerabilities and the production Next.js build completed.

The repairs in this candidate are narrowly user-observable:

- Enrollment now returns the Hub's canonical persisted `group_id`, so a client
  that requested a nonexistent group can discard the stale placement intent.
- Identity-bound HTTP heartbeats preserve the Hub-owned stored group
  transactionally instead of replaying a stale client value.
- A successful PBS details request refreshes that asset's liveness; a failed
  details request deliberately preserves the previous Offline state.
- The PBS Overview Refresh control also refreshes the shared status model after
  the details request succeeds, preventing a stale Offline node header.
- Generic PBS transport failures now render an actionable reachability and
  connector-settings message instead of only `Internal Server Error`.

Pre-deployment verification passed:

- `git diff --check`
- `go test ./internal/hubapi/pbs ./cmd/labtether`
- `go vet ./internal/hubapi/pbs ./cmd/labtether`
- PBS user-QA Vitest: 1 file / 8 tests
- console TypeScript (`tsc --noEmit`)

Only `labtether`, `web-console`, and `console-ingress` in Compose project
`labtetherqar17` were force-recreated. PostgreSQL and every named data volume
were retained. The PostgreSQL container remained exact ID
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`,
`StartedAt=2026-07-15T14:40:46.403448608Z`, `RestartCount=0`. Database readback
matched before and after replacement: migration high-water/count `96/96`, one
user, 21 assets, two collectors, and five sessions.

All three application containers use exact r23 image ID `2d74dadc7567`, became
healthy, reported `RestartCount=0` / `OOMKilled=false`, and the private-CA
validated container route returned HTTP 200 with `/version`
`20260716-r23`. Startup loaded the existing CA, server certificate, SSH
identity and database; Windows reconnected one second after Hub start; and a
fresh PBS details request returned 200. The unauthenticated installed console
root also returned HTTP 200 through `localhost:23000`.

At `2026-07-16T09:29:32Z`, more than five minutes after the Hub started, all
three r23 application containers remained healthy with unchanged start times,
zero restarts and no OOM. Hub logs contained none of the migration, runtime
ownership, lease/drain, closed-pool, fatal, panic or OOM failure signatures.
Post-start PostgreSQL readback showed the PBS asset Online, its collector `ok`,
and the Windows asset Online with a current heartbeat.

The exact rollback remains available and does not replace PostgreSQL:

```sh
env LABTETHER_IMAGE=labtether-installed-qa@sha256:95e215d40f293fa5f41a757d65ae487f232ac9fbf6531bd2398c5aa89958c0a9 POSTGRES_IMAGE=postgres:18-alpine docker compose -p labtetherqar17 -f deploy/compose/docker-compose.deploy.yml -f /tmp/labtetherqa-r14/docker-compose.qa-override.yml up -d --no-deps --force-recreate labtether web-console console-ingress
```

Disposition at deployment handoff: **PASS for build, migration/persistence,
replacement, health and installed smoke**. Fresh cache-disabled browser
re-certification of the repaired status/error behaviors and the requested soak
remain separate gates; this section does not pre-claim them.

## r26 final installed browser re-certification — 2026-07-16

The follow-up repairs were rebuilt and installed as exact image
`labtether-installed-qa:20260716-r26`, image/digest
`sha256:09a833de6ced3f1cb1b943cf94939e080452db78ef6b57e1b78c704cd3c61af2`,
with the private-CA-validated `/version` route returning HTTP 200 and
`20260716-r26`.
The build used the production `build/Dockerfile`, Linux arm64, and a no-cache
dependency/build path. `npm ci` reported zero vulnerabilities and the
production Next.js build and typecheck passed.

Only `labtether`, `web-console`, and `console-ingress` were replaced. All three
became healthy on the exact r26 image with `RestartCount=0` and
`OOMKilled=false`. PostgreSQL and every named volume were preserved; before and
after readback remained migration high-water/count `96/96`, one user, 21
assets, two collectors, and five sessions.

The final cache-disabled installed-console journey used the protocol-faithful
disposable HTTPS PBS target and proved:

- Baseline rendered **Online**, PBS 4.2.2, node `localhost`, 1 TB total at 25%
  used, one backup group, and two snapshots.
- **Refresh** completed against the live upstream and retained the complete
  discovered inventory.
- A controlled emulator failure was enabled through its QA-only
  `POST https://127.0.0.1:18007/qa/fail?on=1` switch. The next **Refresh**
  immediately changed the device header to **Unresponsive** and rendered an
  actionable PBS reachability and connector-settings error instead of
  `Internal Server Error` or a false successful refresh. The last successful
  version, 1 TB / 25% capacity, one-group, and two-snapshot inventory remained
  visible as cached data while the failed upstream state was explicit.
- Clearing the emulator failure and pressing **Refresh** immediately restored
  **Online** and the complete 4.2.2, node, capacity, group, and snapshot data.

The scoped repair checks passed before installation: Hub PBS tests, Go vet,
PBS user-QA Vitest (1 file / 8 tests), console TypeScript, the production Next.js
build, and `git diff --check`. Final disposition: **PASS for the installed r26
PBS integration's normal details/inventory flow, honest upstream-failure UX,
and immediate recovery**. This does not broaden the production recovery claim
below.

## r27 final installed Hub re-certification — 2026-07-17

The Hub was rebuilt without cache as
`labtether-installed-qa:20260717-r27`, exact image ID
`sha256:7f088d78bfbb26b6a350096110fce2105ba6cd022192f15cc3d6534e5d0e0940`.
Only the three application containers were recreated. PostgreSQL retained exact
container ID
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`,
its original start time, restart count zero, volumes, and all 21 assets. The
private-CA-validated `/version` and `/healthz` routes returned HTTP 200 with
`20260717-r27` and PostgreSQL `ok`.

The exact r27 installed browser journey repeated the disposable PBS state
transitions rather than relying only on the prior candidate:

- baseline and a user-triggered **Refresh** rendered **Online**, PBS 4.2.2,
  node `localhost`, 1 TB total / 25% used, one backup group, and two snapshots;
- enabling only the emulator's QA failure switch and pressing **Refresh**
  immediately changed the header to **Unresponsive**, showed the actionable PBS
  reachability/settings message, and retained one group, two snapshots, and the
  25% disk summary as clearly stale inventory;
- clearing the emulator switch and pressing **Refresh** restored **Online**,
  version 4.2.2, and the complete inventory immediately.

At the 33-minute installed checkpoint, all r27 app containers remained healthy
with restart count zero and `OOMKilled=false`; a fatal-log scan found no lease
loss, incomplete drain, panic, fatal, OOM, or shutdown error. The longer r26
window is not treated as clean soak evidence because each lease loss matched a
Mac clamshell sleep that suspended Colima. A fresh exact-r27 24-hour check is
scheduled and remains pending rather than being described as passed.

Final disposition: **PASS for the exact installed r27 PBS normal,
upstream-failure, cached-inventory, and immediate-recovery journey**. The
production recovery boundary below is unchanged.

## r27 24-hour checkpoint — 2026-07-21

Checkpoint result: **FAIL / blocker for the exact-r27 24-hour installed soak**.

The checkpoint was run at `2026-07-21T15:53:18Z`, after the scheduled
`2026-07-16T16:15:00Z` r27 observation start. The installed Docker runtime was
not running: `docker context show` selected `colima`, `docker ps` failed because
`/Users/michael/.colima/default/docker.sock` was absent, the default-context
`docker ps` also failed at `/var/run/docker.sock`, and `colima status`
reported `colima is not running`.

Strict endpoint checks did not reach the installed hub: `https://192.168.0.118:28443/version`
and `/healthz` both timed out, and the local console port `http://localhost:3000`
refused connection. The Colima host-agent log shows the VM entered
`VirtualMachineStateError` at `2026-07-16T21:49:29+10:00` and socket forwarding
was stopped at `2026-07-16T21:49:37+10:00`. Colima was not restarted for this
checkpoint because doing so would invalidate the continuous 24-hour soak
question.

Because the runtime was down, this checkpoint cannot confirm the three
`labtetherqar17` application containers still used image
`sha256:7f088d78bfbb26b6a350096110fce2105ba6cd022192f15cc3d6534e5d0e0940`,
were healthy, had `RestartCount=0`, had `OOMKilled=false`, or had no fatal,
panic, lease-loss, incomplete-drain, closed-pool, or shutdown-error signature
since `2026-07-16T16:15:00Z`. It also cannot confirm the preserved PostgreSQL
container ID
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`, original
start time, restart count, database, `/version`, `/healthz`, or current PBS
console inventory.

No production connector, credential, backup job, recovery unit, prohibited
target, or household automation was changed by that checkpoint. At that time
the 13 connector rotations, backup-job exclusion, and unproven recovery units
were open. The later config-only correction recorded below supersedes only the
job-exclusion item; it does not change the failed soak result.

## r27 24-hour follow-up checkpoint - 2026-07-22

Checkpoint result: **FAIL / blocker; the exact-r27 24-hour installed soak
remains unproven**.

This follow-up checkpoint was run at `2026-07-22T02:21:48Z`. The current Docker
context was `default`, but `/var/run/docker.sock` was absent and `docker ps`
could not connect. The Colima Docker socket
`/Users/michael/.colima/default/docker.sock` was also absent, `colima status`
reported `colima is not running`, and no `Docker`, `colima`, `lima`, `qemu`, or
`vz` process was visible to `pgrep` at the checkpoint.

Strict endpoint checks again did not reach the installed hub:
`https://192.168.0.118:28443/version` and `/healthz` timed out with HTTP `000`,
and `http://localhost:3000` refused connection. The Colima logs show the VM had
been started and stopped earlier on `2026-07-22` (`10:31:18+09:00` to
`10:41:27+09:00`, then `10:42:39+09:00` to `10:44:58+09:00`), with LabTether
port forwarding including `3000` and `28443` torn down before this checkpoint.
Colima was not started for this checkpoint because that would invalidate the
continuous-soak question.

Because the runtime and console were unavailable, this checkpoint still cannot
confirm the three `labtetherqar17` application containers used image
`sha256:7f088d78bfbb26b6a350096110fce2105ba6cd022192f15cc3d6534e5d0e0940`,
were healthy, had `RestartCount=0`, had `OOMKilled=false`, or had no fatal,
panic, lease-loss, incomplete-drain, closed-pool, or shutdown-error signature
since `2026-07-16T16:15:00Z`. It also cannot confirm the preserved PostgreSQL
container ID
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`, original
start time, restart count, preserved QA database, strict `/version` and
`/healthz`, or current disposable PBS console inventory.

No production connector, credential, backup job, recovery unit, prohibited
target, or household automation was changed by that follow-up checkpoint. At
that time the 13 connector rotations, backup-job exclusion, and unproven
recovery units were open. The later config-only correction recorded below
supersedes only the job-exclusion item; it does not change the failed soak
result.

## r27 automation current-system checkpoint - 2026-07-23

Checkpoint result: **FAIL / blocker for the exact-r27 24-hour installed soak;
current runtime is a different candidate and PBS is not currently Online**.

This read-only automation checkpoint ran at `2026-07-23T02:21:28Z`. Docker was
reachable through context `colima`, but the live `labtetherqar17` application
containers no longer used r27 image
`sha256:7f088d78bfbb26b6a350096110fce2105ba6cd022192f15cc3d6534e5d0e0940`.
Hub, web console, and console ingress instead used
`labtether-installed-qa:20260723-r31-dockerfix-83695f01` /
`sha256:384b9f3f21afb8072b1091d634380d5cbafac07b2e5e4e770b7f768b38608ee1`,
with fresh `2026-07-23T02:13Z`-`02:15Z` start times. They were healthy,
restart-zero, and `OOMKilled=false`; a current-log scan found no requested
fatal, panic, lease-loss, incomplete-drain, closed-pool, or shutdown-error
strings, but this is r31-dockerfix evidence and cannot prove the r27 soak
window.

PostgreSQL retained container
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`, start
time `2026-07-22T09:25:33.948207088Z`, restart count zero, healthy state,
`OOMKilled=false`, and the preserved QA database (`96/96` migrations, one user,
24 assets, three collectors, and two sessions). Strict private-CA `/version`
returned `20260723-r31-dockerfix-83695f01`, not `20260717-r27`; `/healthz`
reported PostgreSQL `ok`.

The rendered installed console was reachable only at the unauthenticated login
screen during this run because the retained r28 cookie was stale (`401`), and
no credential or session state was mutated to force access. Read-only
PostgreSQL and collector readback showed the disposable PBS root
`Disposable PBS QA r17` as **Unresponsive**. Its collector last ran at
`2026-07-23T02:22:02.204002Z` and failed with connection refused to
`https://host.docker.internal:18007/api2/json/admin/datastore`. The retained
`qa-store` datastore child remained `online` only as stale prior inventory from
`2026-07-16T17:19:42.945347Z`; it is not current PBS Online proof.

No production PBS endpoint, datastore, backup group, namespace, verification
job, sync job, prune job, credential, recovery unit, prohibited VM, or
household automation was changed. The 13 credential rotations, unsigned Windows
release, and Delta VM 100, Delta VM 101, and Gamma VM 100 recovery proofs
remain open; the earlier config-only backup-job exclusion is the only
superseded residual from the original r27 checkpoint notes.

## r27 automation current-system checkpoint - 2026-07-24

Checkpoint result: **FAIL / blocker for the exact-r27 24-hour installed soak;
current runtime is again a different candidate and PBS is not currently
Online**.

This read-only automation checkpoint ran at `2026-07-24T02:27:51Z`. Docker was
reachable through context `colima`, but the live `labtetherqar17` application
containers no longer used r27 image
`sha256:7f088d78bfbb26b6a350096110fce2105ba6cd022192f15cc3d6534e5d0e0940`.
Hub, web console, and console ingress instead used
`labtether-installed-qa:20260723-main-1a4290af` /
`sha256:a4bcfa50ef25967af4759471ebe1ed2ab6ec79b2d711c7e00547092ab4ddd307`,
with fresh `2026-07-23T04:58:49Z`, `2026-07-23T04:58:54Z`, and
`2026-07-23T04:59:00Z` start times. They were healthy, restart-zero, and
`OOMKilled=false`; a current-log scan since `2026-07-16T16:15:00Z` found no
requested fatal, panic, lease-loss, incomplete-drain, closed-pool, OOM, or
shutdown-error strings, but this is main-candidate evidence and cannot prove
the r27 soak window.

PostgreSQL retained container
`d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`, start
time `2026-07-22T09:25:33.948207088Z`, restart count zero, healthy state,
`OOMKilled=false`, and the preserved QA database (`96/96` migrations, one user,
26 assets, two hub collectors, and zero active sessions). Strict private-CA
`/version` returned `20260723-main-1a4290af`, not `20260717-r27`; `/healthz`
reported PostgreSQL `ok`.

The installed console rendered only the unauthenticated login screen; protected
console API calls returned `401`, and no credential or session state was
mutated to force access. Read-only PostgreSQL and collector readback showed the
disposable PBS root `Disposable PBS QA r17` as **Unresponsive**. Its collector
last ran at `2026-07-24T02:26:14.292446Z` and failed with connection refused to
`https://host.docker.internal:18007/api2/json/admin/datastore`. The retained
`qa-store` datastore child remained `online` only as stale prior inventory from
`2026-07-16T17:19:42.945347Z`; it is not current PBS Online proof.

No production PBS endpoint, datastore, backup group, namespace, verification
job, sync job, prune job, credential, recovery unit, prohibited VM, or
household automation was changed. The 13 credential rotations, unsigned Windows
release, and Delta VM 100, Delta VM 101, and Gamma VM 100 recovery proofs
remain open; the earlier config-only backup-job exclusion remains the only
superseded residual from the original r27 checkpoint notes.

## Historical recovery observation — 2026-07-16 (classification superseded)

Michael subsequently authorized the separate recovery gate. One real offsite
recovery unit was selected and exercised end to end without querying or acting
on VM 102: McCann PBS namespace `offsite-delta`, VM 100 (`ContainerVM`), snapshot
`2026-07-13T11:00:00Z` / backup time `1783940400`.

Destination-local verification passed before restore:

- McCann PBS 4.2.2 had no active verification, sync, reader, backup, prune, or
  restore task when the run started.
- A targeted low-thread verifier ran from `2026-07-16T18:40:09+10:00` to
  `18:43:37+10:00` with `--ignore-verified false`, `--read-threads 1`, and
  `--verify-threads 2`.
- It checked the exact `qemu-server.conf.blob` and 128 GiB
  `drive-scsi0.img.fidx`, verified 100,460.00 MiB in 207.57 seconds, reported
  zero errors, and ended `TASK OK`.
- The exact destination snapshot's verification metadata changed from the
  inherited Simba source verifier to McCann-local task
  `UPID:pbs:0007D960:7C3A3EF3:000000BF:6A5898E9:verify_snapshot:PBS\x2dStorage\x3ans-offsite\x2ddelta-vm-100-6A54C530:root@pam:`.

The isolated restore then passed on `zetaserver`:

- A temporary PVE storage entry, `LTQA-PBS-Recovery`, exposed only namespace
  `offsite-delta`. It reused a mode-0600 copy of zeta's existing PBS credential
  without printing its value. The entry resolved only the three expected
  VM-100 snapshots.
- The verified snapshot restored to stopped disposable VMID `9900` and exact
  disk `local-zfs:vm-9900-disk-0` from `18:45:27` to `18:52:39+10:00`.
  The 137,438,953,472-byte image completed in 372.44 seconds at 351.93 MB/s.
- Before first boot, the clone was renamed `LTQA-Recovery-ContainerVM`, changed
  to `onboot=0`, capped at four cores / 16 GiB, and protected from accidental
  deletion. Its only PVE NIC (`net0`) was removed while stopped; config readback
  proved no `net*` device remained. The restored guest therefore had no
  household-LAN path.
- QGA proved Debian 13 / kernel 6.12.95 booted from the restored ext4 root
  filesystem. Systemd reported `running`, Docker 29.6.1 with `overlay2` was
  active, all 17 restored container definitions were present, and there were
  zero failed systemd units or storage/filesystem kernel errors.
- Application checks ran only over loopback inside the networkless clone:
  Uptime Kuma returned HTTP 302, Beszel HTTP 200, Portainer `/api/status`
  HTTP 200, and Termix HTTP 200. `pairdrop` exited and `gluetun` was unhealthy
  because their external network was deliberately absent; the four selected
  application surfaces and all critical restored definitions were available.

Cleanup also passed:

- VMID `9900` shut down gracefully from `18:56:47` to `18:56:59+10:00` and the
  exact disposable VM/disk were removed at `18:57:37+10:00`.
- Its PVE config, local-zfs inventory, ZFS volume, temporary storage entry, and
  copied credential file were all absent on the `18:58:23+10:00` reconciliation.
- Zeta's production `PBS-Storage` entry still selected namespace `zeta`; its
  22:00 all-VM backup job was unchanged; both zeta and McCann PBS had no active
  tasks; and the source snapshot remained present with the new local verifier
  metadata.

Historical observation: the destination verification, isolated restore, boot,
application checks, and cleanup above were real and remain useful evidence.
However, the later normalized machine-readable contract supersedes the former
unit-level PASS classification. Canonical Delta VM 100 remains `never_proven`
with null `last_proven_recoverable`; this historical section must not be used to
advance it.

## Canonical recovery boundary — 2026-07-22

The authoritative state is
`/Users/michael/Development/homelab/evidence/recovery/last-proven-recoverable.json`:

| Recovery unit | Canonical state | Evidence boundary |
|---|---|---|
| Zeta VM 100 | Passed; `2026-07-22T18:51:34+10:00` | Destination-local verification, isolated restore, workload checks, shutdown, and cleanup passed |
| Zeta VM 101 boot/config scope | Passed for that bounded scope; `2026-07-22T19:23:48+10:00` | Isolated boot/config passed; MainPool deliberately remained unattached, so no broader storage/workload claim is made |
| Theta Home Assistant VM 100 | Passed; `2026-07-22T20:17:02+10:00` | HAOS/Supervisor/Core, config, 2.42 GB SQLite integrity, 41 automations, local application checks, two egress-denial rounds, shutdown, and cleanup passed |
| Delta VM 100 | `never_proven`; null | Current contract-compliant proof remains open; historical observation above does not advance it |
| Delta VM 101 | `never_proven`; null | Destination verification, isolated restore, workload checks, and cleanup remain open |
| Gamma VM 100 | `never_proven`; null | Destination verification, isolated restore, workload checks, and cleanup remain open |

Detailed passing proof is in
`/Users/michael/Development/homelab/evidence/recovery/2026-07-22-delta-isolation-and-zeta-vm-recovery-proofs.md`
and
`/Users/michael/Development/homelab/evidence/recovery/2026-07-22-theta-ha-vm-100-recovery-proof.md`.
No further restore was started near the scheduled backup window. The prohibited
target remained untouched and is not recovery evidence.

The broader connector/service credential audit counted 13 credentials. On
`2026-07-22`, Michael authorized execution and two create-first OmegaNAS
attempts were made. Both stopped before old-key revocation when the immediate
post-restart MCP proof did not complete; both transactions restored the two
mode-`0600` local copies, restarted only the TrueNAS bridge, and deleted the
uncommitted replacement. The restored old path and both TrueNAS health calls
subsequently passed. TauNAS was not attempted, and the other 11 external-
authority blockers remain unchanged. Therefore **0 of 13 credentials are
rotated**; the exact attempt and rollback evidence is in the
[credential-rotation audit](./2026-07-16-credential-rotation-audit.md).

## Backup-job correction and canonical-record handoff

Michael explicitly authorized the bounded config-only correction on
`2026-07-22`. Exact job-only pre-readback for
`backup-5cbf16f7-7a25` showed `all=1`, no exclusion, `enabled=1`, schedule
`21:00`, storage `PBS-Storage`, and mode `snapshot`. The first mutation attempt
through a read-only API token failed with HTTP 403 and immediate readback proved
no change. The authorized root host path then added only `exclude=102`; exact
post-readback confirmed:

- `all=1` and `exclude=102`;
- `enabled=1`, schedule `21:00`, storage `PBS-Storage`, and mode `snapshot`
  unchanged; and
- rollback is
  `pvesh set /cluster/backup/backup-5cbf16f7-7a25 --delete exclude`.

No guest inventory call was used and the prohibited target itself was never
queried, enumerated, started, stopped, backed up manually, restored, or
otherwise operated on. Any retention/RPO decision for development VMs 103/104
remains separate and is not inferred from this prohibition.

The obsolete proposed JSON handoff was removed because the canonical
tracker now already contains separate per-VM objects and is authoritative:
`/Users/michael/Development/homelab/evidence/recovery/last-proven-recoverable.json`.
Do not recreate the former Delta VM 100 PASS object from this historical note.
Delta VMs 100 and 101 must both retain `status: "never_proven"` and
`last_proven_recoverable: null` until their own current contract-compliant
proofs pass.
