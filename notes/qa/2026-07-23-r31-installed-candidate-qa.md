# LabTether r29-r31 Home Assistant Quick Config QA — 2026-07-23

## Disposition

**r29 was rejected for Home Assistant onboarding.** Exact main commit
`8245720e2cd5112111eb9338e2a3b2a817870d3a`, installed as
`labtether-installed-qa:20260723-r29` /
`sha256:8cfa2d7e8ed0522b1f4a213b41fe4585545087c2ddb8b3c54bb708f7997ebcbd`,
passed strict health and a fresh 19-route browser sweep. It was not acceptable
because Home Assistant Quick Config selected the container-internal
`https://labtether:8443` origin and offered automatically discovered Colima/LAN
addresses that were not proven reachable from the Home Assistant host.

**r30 passed the exercised pre-merge repair scope.** Review commit
`852279256289c94a2ae5a12e45cb65cb1cbb3106`, tree
`70d4b7271983bf68c55bc88cd55befa2b4a01de5`, was installed as
`labtether-installed-qa:20260723-r30` /
`sha256:814cb311e464098bcdb9fb4791b36e97076cb7c6e4d464e669d01e809168fab5`.
Because it was built from the review branch, r30 is supporting installed
evidence rather than the canonical merged candidate.

**r31 passed the same installed scope on exact merged source.** PR #108 merged
as `9c408d2c7aee561aef612293eb52a85b20f52915`; its tree
`70d4b7271983bf68c55bc88cd55befa2b4a01de5` exactly matches the reviewed r30
tree. The detached exact-merge worktree built
`labtether-installed-qa:20260723-r31` /
`sha256:953c7df82da26cb419c1de40ad1eaf3f653a76ffb56d41b8b009dfb11bf5b129`.
The image labels record the exact merge revision, version `20260723-r31`, and
the LabTether source repository. Strict `/version` returned `20260723-r31`.

Official release remains NO-GO; the exact residuals are listed below.

## LTQA-276 — Home Assistant Quick Config origin

Home Assistant Quick Config had reused the endpoint-agent enrollment target.
In the installed r29 console this could select a container-internal address or
suggest automatically discovered LAN/container-interface addresses without
proving that the Home Assistant host could reach them.

The repair separates Home Assistant's direct-Hub origin from the agent target.
The enrollment read reconstructs `X-Forwarded-Host` and
`X-Forwarded-Proto` from the browser request, ignores client-supplied forwarding
headers, leaves the upstream `Host` unchanged, and returns `Cache-Control:
no-store`. Home Assistant suggests only configured external or Tailscale
origins, excludes the current console origin, and otherwise starts blank.
Operators may enter a credential-free HTTP(S) origin manually; paths, queries,
fragments, credentials, control characters, and invalid ports are rejected.
Non-loopback HTTP remains copyable but shows the exact Home Assistant
`Allow insecure HTTP` warning.

## Source, review, and image proof

- Two independent reviews found no remaining concrete correctness, UX, or
  security issue after the final patch revision.
- The complete console suite passed 101 files / 386 tests, followed by
  TypeScript, full ESLint, the production Next.js build, `git diff --check`,
  and the complete Docker image build.
- PR #108 passed all 21 checks at exact reviewed head
  `852279256289c94a2ae5a12e45cb65cb1cbb3106`, including both CI trigger sets
  and both CodeQL runs.
- The reviewed and squash-merged commits have the same exact tree. A detached
  worktree at the merge commit produced the r31 image with the correct embedded
  runtime version and OCI labels.
- Trivy found zero HIGH/CRITICAL vulnerabilities and zero image secrets. The
  JSON evidence is
  `/Users/michael/Development/LabTether/.qa-evidence/trivy-labtether-installed-qa-20260723-r31.json`,
  SHA-256 `c0f548bb6be48f8e1d68e6a85b354b2e1ad9755801e5c2b36330d6150f484a72`.
- The exact deploy-hardening contract passed locally on the merged worktree.
  The first post-merge GitHub CI attempt was cancelled after its runner hung in
  that one-second contract past the workflow's ten-minute job limit; the
  identical tree had already passed the same job twice before merge. A complete
  post-merge rerun then passed all eight CI jobs. All four post-merge CodeQL
  jobs were also successful, so exact merged main completed 12/12 jobs green.

## Installed r31 proof

Only Hub, web console, and console ingress were recreated. PostgreSQL retained
container `d4fd5c5478ef59cf99b7d0923e3c6da2b358dbdaf63a6d49fba8af74ebb038f3`,
start time `2026-07-22T09:25:33.948207088Z`, restart count zero, healthy state,
and its existing data. All three application services run the exact r31 digest
and are healthy, restart-zero, and OOM-free.

- Strict private-CA `/version` and `/healthz` passed; PostgreSQL reported `ok`.
- Security enrollment selected `http://127.0.0.1:23000` and
  `ws://127.0.0.1:23000/ws/agent`. Automatically discovered LAN alternatives
  retained direct Hub port `8443`, proving the proxy did not overwrite upstream
  `Host`.
- Home Assistant Quick Config opened with a blank Hub Address and direct-Hub
  guidance. It did not render or suggest the console origin,
  `https://labtether:8443`, or any automatically discovered Colima/LAN address,
  and it rendered no Hub URL copy row while invalid.
- `https://hub.example.invalid:8443` produced the exact Hub URL copy row.
- `http://192.0.2.20:8080` produced the explicit `Allow insecure HTTP` warning
  while retaining an honest copy row.
- A credential-bearing URL with path/query content was rejected as not being a
  credential-free origin and produced no Hub URL copy row.
- A fresh authenticated sweep passed all 19 main routes with zero captured
  browser errors. The Schedules page's product heading is `Saved Schedule
  Definitions`; the Health route and navigation label both render `Health`.
- Post-run fatal/panic/OOM/closed-pool/shutdown-timeout pattern counts were zero
  for Hub, console, and ingress.

## Cleanup and release boundary

The run-specific QA session was deleted and then proved unable to access a
protected route. The original owner password hash was restored exactly without
being printed, and one earlier pre-existing owner session was deliberately
left untouched. The browser QA tabs were closed. No signing operation was
performed and no signing material was accessed, copied, staged, or uploaded.

This repair does not close signed/notarized macOS distribution, signed Windows
distribution, interactive connected Windows tray/Settings/About/export/update
acceptance, physical-device/TestFlight iOS acceptance, owner-approved GHCR
visibility, 13 coordinated credential rotations, three unproven recovery
units, coordinated Hub/Home Assistant publication, or the fresh continuous
soak that must begin from the final published signed candidate. The failed r27
continuous-soak checkpoint remains failed; r31's healthy replacement and
bounded installed acceptance are not a substitute for that final soak.
