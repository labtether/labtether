# Integrating the push relay with the Hub and the iOS app

> Status 2026-10-26: Hub part (section 1) implemented on branch
> `claude/release-push-relay-20261026`, except the console form (1.5): the
> console still has no "relay / own key" choice. Differences from this plan:
> routing reads an explicit `transport` (stored relay channels always carry
> `"transport": "relay"`), `DeletePushDeviceByToken` also matches the
> transport, the default channel (risk 1) is created once per Hub through a
> `notification_bootstrap_markers` row (migration 106), and the relay now
> rate-limits `POST /v1/push` per client IP. The iOS contract is in
> `IOS-CONTRACT.md`.

Plan for wiring `services/push-relay/` (this folder) into the Hub and the iOS
app. Paths and function names come from `origin/main` of `hub` (569336b) and
`ios` (f1390a7). Every function named below also exists on the
`release-20261026` Hub worktree, which has two unpushed commits that rework
digest delivery (`notification_digest_delivery.go`, new
`notification_digest_policy.go`); re-check those lines there before editing.

## Goal and rules

- The official app (`com.labtether.mobile`) can only be reached with
  LabTether's APNs key, so a self-hosted Hub reaches it through the relay.
- Relay mode is the default: an APNs notification channel with no operator key
  (`auth_key_path`, `key_id`, `team_id` all empty) delivers through the relay.
  Operators with their own app build and key keep today's direct mode.
- In relay mode the Hub never sees an APNs token. The app registers its token
  with the relay and gives the Hub only the `registration_id`, a 43-character
  base64url bearer secret. Treat it exactly like a device token: never log it,
  never put it in errors, and **never change its case** (it is case-sensitive;
  hex tokens are lowercased today).
- Relay answers that mean "drop this registration": `410 unregistered` and
  `404 unknown_registration`. Map both to the existing permanent-rejection
  path so current cleanup code runs unchanged.

## 1. Hub

### 1.1 Configuration

- `cmd/labtether/startup_server.go`: read
  `envOrDefault("LABTETHER_PUSH_RELAY_URL", "https://push.labtether.com")`.
  Accept only an absolute `https` URL with no user info, query or fragment
  (allow `http` only when `securityruntime.InsecureTransportAllowed()`).
  An explicit empty value disables relay mode. Pass it to
  `defaultNotificationAdapters(relayURL string)`, which sets
  `&notifications.APNsAdapter{Relay: notifications.NewAPNsRelayClient(relayURL)}`.
  Update the other caller, `cmd/labtether/test_helpers_test.go:147`.
- Document the variable next to `LABTETHER_EXTERNAL_URL` in `.env.example`,
  `.env.deploy.example`, `deploy/compose/docker-compose.deploy.yml` and
  `deploy/release/docker-compose.deploy.yml.tmpl`.

### 1.2 Relay client: new `internal/notifications/apns_relay.go`

- Constants: `APNsTransportDirect = "direct"`, `APNsTransportRelay = "relay"`,
  `DefaultPushRelayURL`, `OfficialAppBundleID = "com.labtether.mobile"`.
- `func APNsUsesRelay(config map[string]any) bool`: true when
  `config["transport"] == "relay"`, or when `auth_key_path`, `key_id` and
  `team_id` are all blank.
- `func ValidRelayRegistrationID(id string) bool`: `^[A-Za-z0-9_-]{43}$`.
- `type APNsRelayClient struct { baseURL string; httpClient *http.Client }`
  with a 15 s timeout, and `NewAPNsRelayClient(baseURL string) *APNsRelayClient`
  (nil for an empty URL).
- `func (c *APNsRelayClient) push(ctx context.Context, p apnsRelayPush) error`
  posts JSON to `{baseURL}/v1/push`:
  `{registration_id, push_type, priority, expiration?, collapse_id?, payload}`
  where `payload` is `json.RawMessage` of the already-built APNs body.
  Read at most 1 KiB of the answer and map it:
  - `200`: nil.
  - `410` or `404`: `&apnsResponseError{statusCode: http.StatusGone, reason: "Unregistered"}`.
    `isPermanentAPNsTokenRejection` then returns true, so the invalid-token
    handler and `IsPermanentAPNsTokenRejection` callers work unchanged.
  - `502 {"error":"apns_error","reason":R}`:
    `&apnsResponseError{statusCode: 502, reason: boundedUTF8(R, 128)}`.
  - `429`, `503`, `502 apns_unreachable`, transport errors: plain retryable
    errors ("push relay unavailable", "push relay rate limited").
  - `400`, `413`, `415`: plain error naming the relay's `error` code.
  - Never include the registration id or the relay URL path in an error.
- The relay requires `collapse_id` to be 1 to 64 printable ASCII characters.
  `boundedHeaderValue` keeps non-ASCII, so add `relayCollapseID(value string)`
  that drops anything outside `0x20..0x7e` and returns "" when empty.

### 1.3 Alert sends: `internal/notifications/apns.go`

- Add `Relay *APNsRelayClient` to `APNsAdapter`.
- In `APNsAdapter.Send`, branch first on `APNsUsesRelay(config)`. Relay mode
  skips the key checks and `authorizationJWT`, returns
  `errors.New("apns: push relay is disabled")` when `a.Relay == nil`, and
  builds the body with the existing `buildAPNsPayload(payload)`.
- Move the worker-pool loop out of `Send` into
  `deliverAPNsBatch(ctx, config, tokens []string, deliver func(context.Context, string) error) error`
  so both modes share batching, the invalid-token handler and
  `apnsDeliveryBatchError`. Direct mode passes `a.sendToDevice`; relay mode
  passes `a.sendViaRelay`.
- `func (a *APNsAdapter) sendViaRelay(ctx, registrationID string, body []byte, payload map[string]any) error`:
  `push_type "alert"`, priority from `apns_priority` (default 10), expiration
  from `apns_expiration_unix`, collapse id from the same
  `collapse_id`/`alert_id`/`incident_id` fallback as `sendToDevice`.
- Put the relay code in `apns_relay.go`, not `apns.go`, to stay under the
  500-code-line limit.

### 1.4 Live Activity sends: `internal/notifications/activitykit_apns.go`

- In `SendLiveActivity`, branch on `APNsUsesRelay(config)` **before** the
  normalization block: today it runs
  `push.DeviceToken = strings.ToLower(...)`, which would corrupt a relay id.
- Relay branch: require `push.BundleID == OfficialAppBundleID` and
  `ValidRelayRegistrationID(push.DeviceToken)` (instead of
  `validLiveActivityPushToken`), keep the `ActivityID` and `Event` checks,
  build the body with `buildLiveActivityPayload(push)` and send
  `push_type "liveactivity"`, `priority` (5 or 10, default 10),
  `collapse_id: relayCollapseID(push.ActivityID)` and
  `expiration: push.ExpiresAt.Unix()` when set.

### 1.5 Channel configuration

- `internal/hubapi/alerting/notification_channel_validation.go`,
  `validateAPNsNotificationConfig`: accept `transport` of `"direct"` or
  `"relay"` (absent means: relay when no key fields, else direct). Relay mode
  rejects any of `auth_key_path`/`key_id`/`team_id`, rejects
  `allowed_bundle_ids`, and requires `bundle_id` to be empty or
  `com.labtether.mobile`. A partial key set stays an error.
- Where APNs channel config is normalized before storing
  (`internal/hubapi/alerting/notification_validation.go`, the
  `notifications.ChannelTypeAPNs` case), write `transport: "relay"` and
  `bundle_id: "com.labtether.mobile"` for relay channels, so stored config is
  explicit.
- Console: `web/console/app/[locale]/(console)/settings/components/notificationChannelForm.ts`
  and `NotificationChannelsCard.tsx`: the APNs form gets "Delivery: LabTether
  push relay (default) / My own APNs key"; relay hides the key fields. Strings
  go in `web/console/messages/en/notifications.json`.
- The channel test endpoint (`notification_handlers.go`, the APNs branch near
  line 202) needs no change; it reaches relay devices through the normal path.

### 1.6 Device storage and registration

- Migration in `internal/persistence/postgres_migrations_notifications.go`,
  next free version (101 is the highest on `origin/main`, 105 on the release
  worktree):
  - `ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS push_transport TEXT NOT NULL DEFAULT 'direct' CHECK (push_transport IN ('direct', 'relay'))`
  - the same column on `live_activity_push_tokens`.
- Store the relay id in the existing `push_token` column with
  `push_transport = 'relay'`. Dedupe and reclaim in `UpsertPushDevice`,
  `DeletePushDeviceByToken`, retry fingerprints (`newAPNsRetryTarget`) and
  digest claims all key on `push_token`, so they stay correct. A relay id
  (base64url, 43 chars) cannot equal a hex token.
- `internal/persistence/postgres_push_device_store.go`: add
  `PushTransport string json:"push_transport"` to `PushDevice`; write it in
  `UpsertPushDevice` (insert and `ON CONFLICT` update); add it to
  `pushDeviceSelectSQL` and `scanPushDevice`.
- `internal/hubapi/resources/device_handlers.go`:
  - `RegisterDeviceRequest` gains `RelayRegistrationID string json:"relay_registration_id"`.
  - `normalizeAndValidateDeviceRegistration`: exactly one of `push_token` and
    `relay_registration_id`. For relay: trim but do not change case, check
    `notifications.ValidRelayRegistrationID`, require `bundle_id` empty or
    official (default it), then set `PushToken = RelayRegistrationID` and
    `PushTransport = "relay"`. Old clients keep sending `push_token`
    (`PushTransport = "direct"`).
  - `HandleDeviceRegister` passes `PushTransport` to `UpsertPushDevice` and
    adds `"push_transport"` to its response.
  - If importing `internal/notifications` from `resources` creates a cycle,
    move `ValidRelayRegistrationID` to a small leaf package.

### 1.7 Dispatch

- `internal/hubapi/alerting/notification_apns_dispatch.go`,
  `sendAPNsNotification`: compute `relayChannel := notifications.APNsUsesRelay(baseConfig)`
  and skip any device whose transport does not match, **before** the digest
  branch. Without this, a direct channel sends a relay id to Apple
  (`BadDeviceToken`), and the existing cleanup handler deletes a valid
  registration; a relay channel would send raw tokens to the relay (`404`)
  with the same result. In relay mode the allowed bundle set is only
  `com.labtether.mobile`.
- `internal/hubapi/alerting/notification_digest_delivery.go`,
  `processPushDigestClaim`: apply the same transport check to
  `claim.Device.PushTransport` against the claim's channel and `complete()`
  the claim when they differ.
- `incident_notification_delivery.go` (`dispatchIncidentNotificationSync`)
  iterates every enabled APNs channel and needs no change beyond the filter.
- 410 handling needs no new code: the relay mapping in 1.2 triggers the
  existing `SetAPNsInvalidDeviceTokenHandler` callback, which calls
  `DeletePushDeviceByToken(token, bundleID, environment)`.

### 1.8 Live Activities

- `internal/hubapi/alerting/live_activity_handlers.go`:
  - `registerLiveActivityRequest` gains `RelayRegistrationID string json:"relay_registration_id"`.
  - `normalizeLiveActivityRegistration` must not lowercase the relay id.
  - `validateLiveActivityRegistration`: exactly one of `push_token` and
    `relay_registration_id`; relay ids use `ValidRelayRegistrationID` and the
    official bundle id.
  - `HandleIncidentLiveActivityTokens` encrypts the relay id into
    `TokenCiphertext` with the same `liveActivityTokenAAD(recordID)`, hashes
    it into `TokenHash`, and sets `Transport: "relay"`.
- `internal/persistence/postgres_live_activity_store.go`: add `Transport` to
  `LiveActivityPushToken` and to its upsert, select and scan code.
- `internal/hubapi/alerting/live_activity_payload.go`: change
  `liveActivityChannelKey(bundleID, environment)` to
  `liveActivityChannelKey(transport, bundleID, environment)`.
- `internal/hubapi/alerting/live_activity_delivery.go`:
  - `liveActivityChannels`: accept relay channels without key fields, and
    register a relay channel under both the `sandbox` and `production` keys
    for `com.labtether.mobile` (the relay picks the APNs host per
    registration).
  - `deliverLiveActivityRegistration`: look up the channel with
    `registration.Transport`; after decrypting, validate relay rows with
    `ValidRelayRegistrationID` instead of `ValidLiveActivityPushToken`. The
    existing `IsPermanentAPNsTokenRejection` branch then deletes rows the
    relay reports as gone.

### 1.9 Hub tests (one per real risk)

- `internal/notifications/apns_relay_test.go` with `httptest.Server`: request
  JSON shape; `410` and `404` are permanent rejections; `502` keeps Apple's
  reason; `429`/`503` are retryable and not permanent; the registration id
  never appears in an error string.
- `internal/hubapi/alerting/notification_delivery_apns_test.go`: a relay
  channel sends only relay devices and a direct channel never sends a relay id
  (risk: valid registrations deleted after a cross-transport send).
- `internal/hubapi/alerting` Live Activity registration test: a mixed-case
  relay id is stored and sent unchanged (risk: lowercasing breaks every push).
- `internal/hubapi/resources` device registration test: exactly one of
  `push_token`/`relay_registration_id`, and relay id format.

## 2. iOS

### 2.1 Configuration

- Use the relay only when `Bundle.main.bundleIdentifier == "com.labtether.mobile"`.
  A self-built app with another bundle id keeps sending raw tokens to a Hub
  that has its own key.
- `LabTetherMobile/Info.plist`: add `LTPushRelayURL` =
  `$(PUSH_RELAY_URL)`, defined in `project.yml` as
  `https://push.labtether.com` (DEBUG may point at the `workers.dev` URL).
- Environment stays as today: DEBUG builds `sandbox`, others `production`
  (`PushManager.apnsEnvironment`, `IncidentLiveActivityManager.apnsEnvironment`).

### 2.2 New `LabTetherMobile/Core/Push/PushRelayClient.swift`

```swift
struct PushRelayClient: Sendable {
    enum Kind: String, Sendable { case alert, liveactivity }
    let baseURL: URL
    func register(token: String, kind: Kind, environment: String) async throws -> String
    func unregister(registrationID: String) async   // best effort, errors ignored
}
```

- `register` posts `{token, kind, environment}` to `/v1/registrations`,
  expects `201 {"registration_id": ...}` and checks `^[A-Za-z0-9_-]{43}$`.
- `unregister` sends `DELETE /v1/registrations/{id}`.
- Ephemeral `URLSession` (no cookies, no cache), 15 s timeout, no logging of
  tokens or ids.

### 2.3 New `LabTetherMobile/Core/Push/PushRelayRegistrationStore.swift`

Keychain item (service `com.labtether.mobile.push-relay`, account `alert`,
`kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`) holding
`{registrationID, tokenSHA256, environment}`. Keychain, not `UserDefaults`,
because the id is a bearer secret.

### 2.4 `LabTetherMobile/Core/Push/PushManager.swift`

- `DeviceRegisterRequest`: `pushToken` becomes `String?`; add
  `relayRegistrationID: String?` with key `relay_registration_id`. Optional
  properties encode with `encodeIfPresent`, so only one is sent.
- `registerWithHub(token:generation:)`: when the relay is enabled, call a new
  `ensureRelayRegistration(token:) async throws -> String` first, then send
  `relay_registration_id`. If the Hub answers `400` (an older Hub that
  requires `push_token`), resend once with `push_token` and remember that for
  the session.
- `ensureRelayRegistration(token:)`: return the stored id when the token hash
  and environment match. Otherwise register a new id, store it, and
  `unregister` the previous id after the Hub accepted the new one. Check
  `registrationGeneration` and `Task.isCancelled` before storing; if
  superseded, `unregister` the id just created. This is also the token-change
  path: APNs calls `didRegisterForRemoteNotifications(deviceToken:)` with the
  new token and the hash no longer matches.
- `deregister()` (logout via `ShellCoordinator` and permission revocation):
  after `unregisterForRemoteNotifications()` and the Hub call, always
  `unregister` the stored relay id and clear the Keychain item, even when the
  Hub call failed. That stops delivery from a Hub that is unreachable or that
  the user left.

### 2.5 Live Activities

- `LabTetherMobile/Core/Networking/APIClient+LiveActivities.swift`:
  `LiveActivityTokenRegistrationRequest.pushToken` becomes `String?`; add
  `relayRegistrationID: String?` (`relay_registration_id`).
- `LabTetherMobile/Core/LiveActivities/IncidentLiveActivityManager.swift`:
  add `var relayRegistrations: [String: (token: String, id: String)] = [:]`
  keyed by activity id. Memory is enough: activities last about 12 hours and
  relay Live Activity registrations expire after 24.
- `IncidentLiveActivityManager+RemoteRegistration.swift`:
  - `registerRemoteToken(_:for:)`: before the Hub `PUT` loop, reuse the relay
    id when the token is unchanged, otherwise
    `relay.register(token:kind: .liveactivity, environment: Self.apnsEnvironment)`.
    Send `relayRegistrationID` instead of `pushToken`. Include the relay id in
    `registrationFingerprint` so a retry does not register twice. After the
    Hub accepted a rotated token (`pushTokenUpdates`), `unregister` the old id.
  - `deregisterRemoteToken(for:registrationID:)`, the `.ended`/`.dismissed`
    branch in `observeRemoteUpdates`, and `cancelAllRemoteObservers`:
    `unregister` the relay id (best effort) and drop the entry.
  - Same old-Hub fallback as 2.4.

### 2.6 iOS tests (one per real risk)

- `PushRelayClient`: parses `201`, rejects a malformed id, sends the right
  JSON and method.
- `PushManager`: no new relay id when the token is unchanged (avoids churn and
  Hub updates on every launch); a rotated token registers a new id and deletes
  the old one; `deregister()` deletes the relay id even when the Hub fails.
- Live Activity registration sends `relay_registration_id`, never
  `push_token`, when the relay is enabled.

## 3. Rollout and checks

1. Deploy the relay (README "Deploy"), set up the custom domain, and check
   `GET /v1/health`.
2. Land Hub support, then iOS support with the old-Hub fallback.
3. Functional QA on the lab machines, never on the MacBook: Hub stack on VM104
   (`LinuxDevelopmentVM`), app on the Mac mini simulator or a real iPhone.
   Check: alert push arrives; digest push arrives; Live Activity update and
   end arrive; logout makes the next Hub push get `404` and the Hub drops the
   row; deleting the app then pushing gets `410` and the Hub drops the row;
   relay `503` while unconfigured leaves Hub rows intact and retries.

## 4. Open decisions and risks

- **Default channel.** Alerts reach devices only through routes to an APNs
  channel. Should the Hub create a relay channel and attach it to the default
  route when the first relay device registers and no APNs channel exists?
  Recommended, since otherwise "default" still needs operator setup.
- **Payload privacy.** Alert titles, bodies and Live Activity state pass
  through the relay (not stored, not logged) on their way to Apple. The
  privacy policy should say so. A later "minimal payload" mode could send
  only ids and let a notification service extension fetch details from the
  Hub.
- **Abuse.** Anyone can register a token and push to it, but only to an app
  install they control, under per-IP and per-registration limits. There is
  no Hub identity; per-Hub keys could be added later if abuse appears.
- **Live Activity payload cap.** The relay allows 5120 bytes as specified;
  Apple documents 4 KB for ActivityKit updates, and the Hub already caps at
  4 KB. APNs answers `PayloadTooLarge` (relayed as `502`) if 5120 is too
  high.
- **Mass deletion on misconfiguration.** A wrong `APNS_TOPIC` makes APNs
  answer `DeviceTokenNotForTopic` and the relay delete every registration it
  touches, as the Hub does today for direct channels. Change the topic only
  with care.
- **Plaintext at rest.** `push_devices.push_token` is stored in plaintext
  today and relay ids would be too. Encrypting it like
  `live_activity_push_tokens` is optional hardening.
