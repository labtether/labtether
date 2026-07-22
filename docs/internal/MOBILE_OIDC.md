# Native mobile OIDC contract

LabTether's iOS and iPadOS application uses an external user-agent through
`ASWebAuthenticationSession`. The native flow is separate from the browser
console flow and requires RFC 7636 PKCE with `S256`.

## Identity-provider configuration

Register this exact redirect URI on the same OIDC client configured for the
hub:

```text
com.labtether.mobile:/oauth2redirect
```

The hub does not accept alternate schemes, hosts, paths, query strings, or
fragments. The private-use scheme is based on the app's reverse-domain bundle
identifier as required by RFC 8252 and is separate from LabTether's ordinary
`labtether` deep-link scheme. Because iOS custom schemes are not globally
exclusive, PKCE remains mandatory and protects the authorization code if
another installed app claims the scheme.

## Discover support

`GET /auth/providers` returns these fields inside `oidc` when OIDC is enabled:

```json
{
  "mobile_supported": true,
  "mobile_redirect_uri": "com.labtether.mobile:/oauth2redirect",
  "pkce_methods_supported": ["S256"]
}
```

## Start authorization

Generate a cryptographically random RFC 7636 verifier, retain it only in the
in-progress sign-in task, and send its SHA-256 base64url challenge:

```http
POST /auth/oidc/mobile/start
Content-Type: application/json

{
  "redirect_uri": "com.labtether.mobile:/oauth2redirect",
  "code_challenge": "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
  "code_challenge_method": "S256"
}
```

Success returns:

```json
{
  "auth_url": "https://identity.example/authorize?...",
  "state": "opaque-server-state",
  "expires_at": "2026-07-14T12:00:00Z"
}
```

The client opens `auth_url` with callback scheme `com.labtether.mobile` and should compare
the callback's `state` with the returned value before exchanging it. The hub
also binds the state, OIDC nonce, redirect URI, flow type, and PKCE challenge in
a one-time entry that expires after five minutes.

## Complete authorization

After `ASWebAuthenticationSession` returns the callback, send the code, state,
and original verifier directly to the hub over HTTPS:

```http
POST /auth/oidc/mobile/callback
Content-Type: application/json

{
  "code": "provider-authorization-code",
  "state": "opaque-server-state",
  "redirect_uri": "com.labtether.mobile:/oauth2redirect",
  "code_verifier": "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
}
```

The hub consumes state before contacting the provider, verifies the S256 proof
and OIDC nonce, applies the existing OIDC provisioning and role policy, and
creates the same 24-hour server session used by local and browser login.
Success includes a Secure, HttpOnly `labtether_session` cookie and:

```json
{
  "user": {
    "id": "user-id",
    "username": "operator",
    "role": "operator"
  },
  "created": false,
  "session_id": "session-id",
  "expires_at": "2026-07-15T12:00:00Z"
}
```

The mobile client must reject a response without a non-empty `session_id` and
clear any newly installed cookie if local session persistence fails.

## Error and abuse behavior

- `400`: malformed payload, redirect mismatch, invalid/expired state, or PKCE
  mismatch. State/PKCE failures intentionally share one generic message.
- `401`: the provider code exchange or ID-token verification failed.
- `404`: OIDC is disabled.
- `409`: initial owner setup must be completed before OIDC auto-provisioning.
- `429`: per-client/global rate limit or pending-state capacity reached.
- `500`/`503`: session storage, provisioning, or auth services unavailable.

Start and callback have both per-client and global rate limits. Successful
native logins and provider exchange denials emit `auth.oidc.login` audit events;
authorization codes, state, nonce, verifier, and challenge are never audited.
