# LabTether push relay

A Cloudflare Worker that lets self-hosted LabTether Hubs send push
notifications and Live Activity updates to the official iOS app
(`com.labtether.mobile`) without holding LabTether's APNs key.

- The iOS app registers its APNs token with the relay and receives an opaque
  `registration_id` (256 random bits). It gives that id to its Hub instead of
  the raw token.
- The Hub sends pushes to the relay with the id. The relay looks up the token,
  signs an APNs provider token with the LabTether key and forwards the push.
- The registration id is a bearer capability: whoever holds it can push to that
  one app install, nothing more. The app and its Hub(s) hold it; the relay
  stores only its SHA-256.

Plain JavaScript ES modules with JSDoc types, no build step, no runtime
dependencies. APNs is reached with plain `fetch` over HTTP/2.

## API v1

All bodies are JSON. Requests with a body must send
`Content-Type: application/json` (UTF-8) and be at most 8 KiB, or they are
refused before parsing (`415`, `413`). Unknown fields are refused. There is no
CORS. Errors always look like `{"error": "<code>"}`, plus `"field"` for
validation errors and `"reason"` for APNs errors. Request content is never
echoed.

### `POST /v1/registrations` (iOS app)

```json
{ "token": "<hex APNs token>", "kind": "alert", "environment": "production" }
```

- `token`: the APNs device token (for `kind: "alert"`) or the ActivityKit
  per-activity push token (for `kind: "liveactivity"`), as hex, 1 to 100 bytes.
  Case is normalized to lowercase.
- `kind`: `"alert"` or `"liveactivity"`.
- `environment`: `"production"` or `"sandbox"` (DEBUG builds use sandbox).

`201 {"registration_id": "<43-char base64url>"}`. Every call creates a new id;
the app should keep its id and register again only when its token changes.
Rate limited per client IP (IPv6 per /64): 20 per minute.

Alert registrations expire after 400 days without a successful push (each
successful push extends this, at most once per 30 days). Live Activity
registrations expire after 24 hours.

### `DELETE /v1/registrations/:id` (iOS app)

`204` whether or not the id existed. `400` for a malformed id. Same per-IP
rate limit as registration.

### `POST /v1/push` (Hub)

```json
{
  "registration_id": "<id>",
  "push_type": "alert",
  "priority": 10,
  "expiration": 1800000000,
  "collapse_id": "al_42",
  "payload": { "aps": { "alert": { "title": "...", "body": "..." } } }
}
```

- `push_type`: `"alert"` or `"background"` for `alert` registrations,
  `"liveactivity"` for `liveactivity` registrations.
- `priority`: `5` or `10`; `background` requires `5`.
- `expiration` (optional): APNs expiry, Unix seconds, `0` allowed.
- `collapse_id` (optional): 1 to 64 printable ASCII characters.
- `payload`: the complete APNs payload object; it must contain an `aps`
  object. Serialized size at most 4096 bytes (5120 for `liveactivity`).

APNs topic is `APNS_TOPIC` (alert, background) or
`APNS_TOPIC.push-type.liveactivity`, and the host follows the registration's
environment. Rate limited per client IP (IPv6 per /64): 300 per minute,
checked before the body is read or any registration is looked up, so probing
random ids is bounded per caller. Then per registration: 60 per minute.

| Status | Body | Hub action |
|---|---|---|
| 200 | `{"ok": true, "apns_id": "<uuid or null>"}` | delivered to APNs |
| 400 | `invalid_request` + `field`, `invalid_json`, `push_type_mismatch` | bug; do not retry |
| 404 | `unknown_registration` | drop the registration |
| 410 | `unregistered` | drop the registration (APNs said the token is dead; the relay deleted it) |
| 413 | `body_too_large`, `payload_too_large` | do not retry |
| 415 | `unsupported_media_type` | bug; do not retry |
| 429 | `rate_limited` (`Retry-After: 60`) | retry later |
| 502 | `apns_error` + `reason` (Apple's code, e.g. `TooManyRequests`) | retry with backoff unless the reason is permanent, such as `PayloadTooLarge` or `BadTopic` |
| 502 | `apns_unreachable` | retry with backoff |
| 503 | `not_configured` | relay is not set up; retry later |

The relay deletes a registration when APNs answers `410`, or `400` with
`BadDeviceToken` or `DeviceTokenNotForTopic`.

### `GET /v1/health`

`200 {"ok": true}`. Unknown routes answer `404`, wrong methods `405`.

## Deploy

Run from this folder on a machine with Node 22 or newer. The
`wrangler@4.113.0` dev dependency is pinned; `npm install` it there first.

1. Create the KV namespace and paste the printed id into `wrangler.jsonc`
   (`kv_namespaces[0].id`):

   ```sh
   npx wrangler kv namespace create REGISTRATIONS
   ```

2. Set `APNS_TEAM_ID` and `APNS_KEY_ID` in `wrangler.jsonc` `vars` (the
   10-character Apple Team ID and the APNs auth key ID). `APNS_TOPIC` stays
   `com.labtether.mobile`.

3. Upload the APNs `.p8` key as a secret. Michael does this himself; the key
   never goes into this folder, the repo or chat. Pipe the file so its line
   breaks survive:

   ```sh
   npx wrangler secret put APNS_PRIVATE_KEY < /path/to/AuthKey_XXXXXXXXXX.p8
   ```

4. Deploy and check health:

   ```sh
   npm run deploy
   curl https://labtether-push-relay.<subdomain>.workers.dev/v1/health
   ```

5. Custom domain: once `labtether.com` is a zone on the same Cloudflare
   account, uncomment the `routes` entry for `push.labtether.com` in
   `wrangler.jsonc` and deploy again. Hubs default to
   `https://push.labtether.com`, so this must be live before Hubs ship relay
   mode.

Until `APNS_TEAM_ID`, `APNS_KEY_ID` and `APNS_PRIVATE_KEY` are all set and
valid, registrations work and pushes answer `503 not_configured`.

## Privacy

- The relay stores APNs tokens only: KV key `reg:` + SHA-256(registration id)
  maps to `{token, kind, environment, created_at, refreshed_at}`. It never
  stores payloads and never stores registration ids.
- Payloads pass through to Apple and are not logged. The code logs only fixed
  messages (signing key unusable, registration refresh failed, unexpected
  error class), never tokens, ids, payloads or upstream error text.
- Workers invocation logs are disabled in `wrangler.jsonc` because they record
  request URLs, and `DELETE /v1/registrations/:id` carries an id.
- Rate limits are keyed by client IP and by the hash of the registration id.
  Bindings: `REGISTER_LIMIT` (register/delete per IP), `PUSH_IP_LIMIT`
  (push per IP), `PUSH_LIMIT` (push per registration). A missing binding
  fails closed with `503 not_configured`.

## Notes

- Provider tokens (ES256 JWTs) are signed with WebCrypto and cached in module
  scope. Every isolate derives `iat` from the same 50-minute epoch, so tokens
  are never refreshed more than once per 50 minutes per isolate and are never
  older than 50 minutes. Apple refuses refreshes more often than every 20
  minutes and tokens older than 60 minutes.
- Workers KV is eventually consistent. A deleted registration can still be
  served from another location's cache for up to about 60 seconds.
- Workers rate limits are per location and approximate, not exact quotas.

## Tests

```sh
npm test   # node --test, no install needed
```

Tests use Node's built-in runner with fakes for KV, the rate-limiting
binding and APNs (`test-support/fakes.js`). They never contact Apple.

License: Apache-2.0, see `LICENSE`.
