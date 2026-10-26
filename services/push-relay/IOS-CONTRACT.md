# iOS contract: push relay and Hub

What the official iOS app (`com.labtether.mobile`) must call so a self-hosted
Hub can push to it through the LabTether push relay. Hub support lands with
the push relay change (branch `claude/release-push-relay-20261026`); the full
relay API is in `README.md`.

## Rules

- Use the relay only when `Bundle.main.bundleIdentifier == "com.labtether.mobile"`.
  A self-built app with another bundle id keeps sending raw tokens
  (`push_token`) to a Hub that has its own Apple key.
- Relay base URL: `https://push.labtether.com` (Info.plist `LTPushRelayURL`;
  DEBUG builds may use the `workers.dev` URL).
- Environment: `"sandbox"` for DEBUG builds, `"production"` otherwise. Send
  the same value to the relay and to the Hub.
- A `registration_id` is a bearer secret: whoever holds it can push to this
  app install. Keep it in the Keychain, never log it, never change its case.
  It always matches `^[A-Za-z0-9_-]{43}$`; refuse anything else.
- Never send both `push_token` and `relay_registration_id` to the Hub.
- Use one relay id per Hub association. Leaving a Hub must delete its id, so
  that Hub can no longer reach the phone.

## Relay endpoints

All requests and answers are JSON. Send `Content-Type: application/json`.
Bodies are at most 8 KiB. Unknown fields are refused with
`400 {"error":"invalid_request","field":"body"}`.

### Register a token

`POST {relay}/v1/registrations`

```json
{ "token": "<APNs token as hex>", "kind": "alert", "environment": "production" }
```

- `token`: the device token from
  `didRegisterForRemoteNotificationsWithDeviceToken` (`kind: "alert"`), or an
  ActivityKit per-activity push token (`kind: "liveactivity"`), as hex.
- Answer `201 {"registration_id": "<43 characters>"}`. Every call creates a
  new id, so call it only when the token or environment changed.
- Alert registrations expire after 400 days without a successful push; each
  push extends them. Live Activity registrations expire after 24 hours.

### Delete a registration

`DELETE {relay}/v1/registrations/{registration_id}`, no body.

- Answer `204` whether or not the id existed; `400` for a malformed id.
- Best effort: ignore failures, but do not keep the id afterwards.

### Errors (both endpoints)

| Status | Body | App action |
|---|---|---|
| 400 | `invalid_request` + `field`, `invalid_json` | bug; do not retry |
| 413 / 415 | `body_too_large` / `unsupported_media_type` | bug; do not retry |
| 429 | `rate_limited`, `Retry-After: 60` | retry after the header (20 calls per minute per IP) |
| 503 | `not_configured` | retry later with backoff |

## Hub endpoints

The Hub base URL and session authentication are the same as today.

### Register the device for alerts

`POST {hub}/api/v1/devices/register`

```json
{
  "device_id": "3F2A0C1E-...",
  "platform": "ios",
  "relay_registration_id": "<43 characters>",
  "bundle_id": "com.labtether.mobile",
  "environment": "production",
  "time_zone": "Australia/Sydney",
  "notify_critical_alerts": true,
  "notify_node_offline": true,
  "notify_service_down": true,
  "push_category": "alerts_and_incidents",
  "minimum_severity": "warning",
  "quiet_hours_enabled": false,
  "quiet_hours_start_minutes": 1320,
  "quiet_hours_end_minutes": 420,
  "digest_window_seconds": 180
}
```

- Same body as today with `relay_registration_id` instead of `push_token`.
  `bundle_id` may be omitted; if sent it must be `com.labtether.mobile`.
- Answer `200 {"registered": true, "device_id": "...", "push_transport": "relay"}`.
- `400` with a message for invalid input, `429` when the user already has 32
  registered devices.
- The first relay registration on a Hub without APNs routing creates the
  channel "LabTether iOS app" and a catch-all route, so alerts arrive without
  operator setup. The app needs to do nothing for this.
- Older Hubs answer `400 "device_id and push_token are required"`. Such a Hub
  cannot use the relay. Resend once with `push_token` (raw hex token) only if
  you want to support a Hub that has its own Apple key; remember the result
  for the session.

### Deregister on logout

`DELETE {hub}/api/v1/devices/register?device_id={device_id}` answers `204`
(unchanged).

### Register a Live Activity

`PUT {hub}/live-activities/incidents/{incident_id}/activities/{activity_id}`

```json
{
  "device_id": "3F2A0C1E-...",
  "relay_registration_id": "<43 characters, kind liveactivity>",
  "bundle_id": "com.labtether.mobile",
  "environment": "production",
  "show_full_details": false
}
```

- Answer `200`:

  ```json
  {
    "registered": true,
    "registration_id": "lat_...",
    "activity_id": "...",
    "incident_id": "...",
    "expires_at": "2026-10-27T10:00:00Z",
    "push_transport": "relay"
  }
  ```

  Careful: this `registration_id` is the Hub's row id (`lat_...`), not the
  relay id. Keep using it for the Hub `DELETE` below, as today.
- Hub registrations last 12 hours; the relay keeps the id for 24 hours.
- Older Hubs answer `400 "push_token must be a bounded hexadecimal ActivityKit token"`.

`DELETE {hub}/live-activities/incidents/{incident_id}/activities/{activity_id}?device_id={device_id}&registration_id={lat_id}`
answers `204` (unchanged).

## Lifecycle

### Alerts

1. APNs calls `didRegisterForRemoteNotificationsWithDeviceToken`. Hex-encode
   the token.
2. If the Keychain holds a relay id for the same token hash, environment and
   Hub, reuse it. Otherwise `POST /v1/registrations` with `kind: "alert"`.
3. `POST /api/v1/devices/register` with that id.
4. Only after the Hub accepted a new id: store it (with the token hash,
   environment and Hub) and `DELETE` the previous relay id. If the
   registration task was superseded or cancelled before storing, `DELETE` the
   id just created instead.
5. A token change arrives through the same callback: the hash no longer
   matches, so steps 2 to 4 rotate the id.

### Logout, Hub switch or notification permission revoked

1. `unregisterForRemoteNotifications()`.
2. `DELETE /api/v1/devices/register?device_id=...` on the Hub.
3. Always `DELETE /v1/registrations/{id}` at the relay and clear the Keychain
   item, even when step 2 failed. This stops delivery from a Hub that is
   unreachable or that the user left.

### Live Activities

1. For each `pushTokenUpdates` value, reuse the relay id if the activity's
   token is unchanged; otherwise `POST /v1/registrations` with
   `kind: "liveactivity"`. Memory is enough (activities last about 12 hours).
2. `PUT` the Hub registration with `relay_registration_id`. Include the relay
   id in the retry fingerprint so a retry does not register twice.
3. After the Hub accepted a rotated token, `DELETE` the old relay id.
4. When the activity ends or is dismissed, and in `cancelAllRemoteObservers`,
   `DELETE` the Hub registration and the relay id (best effort).

## What the app receives

Unchanged. Alert pushes carry the same `aps` object and custom keys
(`alert_id`, `incident_id`, `severity`, `status`, `summary`, `event`,
`rule_id`, `deep_link`, `runbook_id`, `digest_count`) as with a Hub that has
its own key. Live Activity updates carry the same `content-state`. The relay
forwards the payload unchanged to Apple and does not store it.

## Checks for the iOS change

- The relay client parses `201`, refuses a malformed id and sends the right
  method and JSON.
- No new relay id when the token is unchanged; a rotated token registers a
  new id and deletes the old one; logout deletes the relay id even when the
  Hub call fails.
- Live Activity registration sends `relay_registration_id`, never
  `push_token`, when the relay is in use.
- End to end on the lab (never on the MacBook Pro): alert push arrives;
  digest push arrives; Live Activity update and end arrive; after logout the
  next Hub push gets `404` from the relay and the Hub drops the row; after
  deleting the app the next push gets `410` and the Hub drops the row.
