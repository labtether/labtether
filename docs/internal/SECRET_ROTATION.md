# Secret Rotation Runbook

This document covers the secrets used by LabTether Hub, how to rotate each one,
and the recommended rotation schedule.

---

## Inventory of Secrets

| Secret | Env Var | Purpose |
|--------|---------|---------|
| Encryption key | `LABTETHER_ENCRYPTION_KEY` | AES-256-GCM encryption of stored credentials and the DB-backed OIDC client secret. Also used to derive the TOTP encryption key via HKDF. |
| Owner token | `LABTETHER_OWNER_TOKEN` | Primary authentication token for the hub owner / operator. |
| API token | `LABTETHER_API_TOKEN` | Authentication token for non-browser clients talking to the web-console proxy. |
| Admin password | `LABTETHER_ADMIN_PASSWORD` | Password for the bootstrapped admin user (MVP single-user auth). |
| TOTP key | `LABTETHER_TOTP_KEY` | (Optional) Explicit AES-256 key for encrypting 2FA TOTP secrets. If unset, derived from the encryption key. |
| Postgres password | `POSTGRES_PASSWORD` | Database authentication. |
| Session signing | Console cookie / session layer | Handled by the Next.js console; signed with its own secret. |

All secrets are loaded at startup via environment variables (`.env` or container
env). They are also persisted to the install-state file
(`$LABTETHER_DATA_DIR/install/secrets.json`) so the hub can auto-generate
missing values on first run.

---

## Rotating the Encryption Key

The encryption key protects all credential profiles stored in Postgres using
AES-256-GCM (see `internal/secrets/manager.go`). Ciphertext is prefixed with
`v2:` and includes per-row AAD binding.

**Impact:** Changing the key without re-encrypting existing rows will make all
stored credentials and the DB-backed OIDC client secret unreadable.

### Procedure

1. **Stop the hub.**
   ```bash
   docker compose down
   ```

2. **Back up the database.**
   ```bash
   pg_dump -Fc labtether > labtether_backup_$(date +%Y%m%d).dump
   ```

3. **Generate a new 32-byte base64 key.**
   ```bash
   openssl rand -base64 32
   ```

4. **Re-encrypt existing credentials and OIDC settings (if any).**
   Currently there is no built-in CLI for bulk re-encryption. If you have
   stored credential profiles, you must:
   - Export each credential using the old key (write a small Go program using
     `secrets.NewManagerFromEncodedKey(oldKey)` + `DecryptString`).
   - Re-encrypt with the new key (`NewManagerFromEncodedKey(newKey)` +
     `EncryptString`), preserving the same AAD (credential profile ID).
   - Update the rows in Postgres.

   If `system_settings.key = 'oidc'` contains
   `client_secret_ciphertext`, decrypt and re-encrypt that value with the fixed
   AAD `system_settings:oidc:client_secret`. Never put the plaintext back into
   the JSON row.

   If neither stored credential profiles nor a DB-backed OIDC secret exist,
   skip this step.

5. **Update the key.**
   - Edit `.env` and set `LABTETHER_ENCRYPTION_KEY` to the new value.
   - Since 1.7.1 the Hub refuses to start when `LABTETHER_ENCRYPTION_KEY`
     differs from the key stored in `data/install/secrets.json`, because data
     encrypted with the stored key would become unreadable. After re-encrypting
     (step 4), start once with `LABTETHER_ENCRYPTION_KEY_REPLACE=true` so the
     new key replaces the stored one, then remove that variable. That start
     also rewrites the key check in `system_settings` (see below).

6. **Restart the hub.**
   ```bash
   docker compose up -d
   ```

7. **Verify.**
   - Check hub logs for startup errors (especially "invalid runtime encryption
     key").
   - Test that existing credential profiles decrypt correctly (e.g., trigger a
     connector sync).
   - Confirm 2FA/TOTP still works (the TOTP key is derived from the encryption
     key via HKDF unless `LABTETHER_TOTP_KEY` is set independently).

### Restoring a database onto another Hub

Since 1.7.1 the database keeps a key check: a fixed value encrypted with the
Hub's key, stored as `system_settings.key = 'encryption_key_check'` with the
key's ID. A Hub whose key cannot read it refuses to start and logs both key
IDs. A key ID is the first 12 characters of
`printf '%s' "$KEY" | base64 -d | sha256sum`. Set `LABTETHER_ENCRYPTION_KEY`
to the `encryption_key` from the `secrets.json` saved with that database, or
start once with `LABTETHER_ENCRYPTION_KEY_REPLACE=true` to discard the old
secrets.

### OIDC storage upgrade and rollback

On startup, Hub replaces any legacy plaintext OIDC client secret in
`system_settings` with encrypted storage. The startup fails if that secret
cannot be encrypted. Existing database backups, WAL archives, replicas, and
snapshots may still contain the old plaintext and must be protected or expired.
Rotate the client secret at the identity provider if any old copy may have been
exposed.

An older Hub binary cannot read the encrypted field. For rollback, restore the
matching pre-upgrade database backup, or set `LABTETHER_OIDC_CLIENT_SECRET` and
do not save OIDC settings from the old UI. Saving from an old binary writes the
secret as plaintext again.

---

## Rotating the Owner Token

The owner token is the primary API authentication bearer token. The Hub keeps
it in `$LABTETHER_DATA_DIR/install/secrets.json`, next to the encryption key.

**Never delete `secrets.json` to rotate a token.** It also holds the
encryption key. Without that key, saved credentials, webhook secrets and 2FA
enrolments cannot be read, and since 1.7.1 a Hub whose database holds them
refuses to start rather than make a new key.

### Procedure: token set in the environment

Use this when `LABTETHER_OWNER_TOKEN` is set, for example in the development
`docker-compose.yml` with `.env`.

1. **Generate a new token.**
   ```bash
   openssl rand -hex 32
   ```

2. **Update `.env`.**
   Set `LABTETHER_OWNER_TOKEN` to the new value.

3. **Recreate the hub container** so it reads the new value
   (`docker compose restart` keeps the old environment).
   ```bash
   docker compose up -d labtether
   ```
   The hub stores the new token in `secrets.json` and keeps the encryption
   key as it is.

4. **Update all clients** that use the owner token (scripts, CLI
   configurations, bookmarks with token query params).

### Procedure: token generated by the Hub (deploy Compose)

The deploy Compose file does not pass `LABTETHER_OWNER_TOKEN`. Remove only the
`owner_token` field from `secrets.json`; the hub generates a new token on its
next start and leaves every other value, including `encryption_key`, as it is.
Run these from the install directory:

1. **Stop the hub.**
   ```bash
   docker compose --env-file .env.deploy -f docker-compose.deploy.yml stop labtether
   ```

2. **Remove the stored owner token.**
   ```bash
   docker compose --env-file .env.deploy -f docker-compose.deploy.yml run --rm --no-deps \
     --entrypoint /usr/local/bin/node labtether -e '
       const fs = require("node:fs");
       const path = "/data/install/secrets.json";
       const secrets = JSON.parse(fs.readFileSync(path, "utf8"));
       if (!secrets.encryption_key) throw new Error("secrets.json has no encryption_key; stop and check the backup");
       delete secrets.owner_token;
       fs.writeFileSync(path + ".rotate", JSON.stringify(secrets, null, 2) + "\n", { mode: 0o600 });
       fs.renameSync(path + ".rotate", path);
     '
   ```

3. **Start the hub.**
   ```bash
   docker compose --env-file .env.deploy -f docker-compose.deploy.yml up -d labtether
   ```

4. **Read the new token** only in a private terminal, and update every client
   that used the old one.
   ```bash
   docker compose --env-file .env.deploy -f docker-compose.deploy.yml exec -T labtether \
     /usr/local/bin/node -e 'console.log(JSON.parse(require("node:fs").readFileSync("/data/install/secrets.json", "utf8")).owner_token)'
   ```

---

## Rotating the API Token

The API token is used by non-browser clients talking through the web-console
proxy.

### Procedure

Follow the owner token steps, using `LABTETHER_API_TOKEN` in the environment
procedure, or removing the `api_token` field (not `owner_token`) in the
deploy Compose procedure. Never delete `secrets.json`.

If `LABTETHER_API_TOKEN_FILE` is set, the hub writes the active token to that
path on startup. Downstream services reading that file, such as the bundled
web console, pick up the new token automatically after restart.

---

## Rotating the Admin Password

1. **Update `.env`.**
   Set `LABTETHER_ADMIN_PASSWORD` to a new strong password.

2. **Restart the hub.** The bootstrap logic will update the admin user record.

---

## Rotating the Postgres Password

1. **Update the password in Postgres.**
   ```sql
   ALTER USER labtether WITH PASSWORD 'new-strong-password';
   ```

2. **Update `.env`.**
   Set `POSTGRES_PASSWORD` and update the password portion of `DATABASE_URL`.

3. **Restart the hub.**

---

## Rotating the TOTP Key

If `LABTETHER_TOTP_KEY` is set explicitly:

1. **Warning:** Rotating this key invalidates all enrolled 2FA TOTP secrets.
   Users will need to re-enroll their authenticator apps.

2. Generate a new key:
   ```bash
   openssl rand -base64 32
   ```

3. Update `LABTETHER_TOTP_KEY` in `.env` and restart.

If `LABTETHER_TOTP_KEY` is not set (default), the TOTP key is derived from
`LABTETHER_ENCRYPTION_KEY`. Rotating the encryption key will also rotate the
TOTP key -- meaning 2FA re-enrollment is required unless you pin
`LABTETHER_TOTP_KEY` independently before rotating the encryption key.

---

## Recommended Rotation Schedule

| Secret | Interval | Notes |
|--------|----------|-------|
| Encryption key | Every 12 months | Coordinate with a maintenance window; requires credential re-encryption. |
| Owner token | Every 12 months | Or immediately if compromised. |
| API token | Every 6 months | Or immediately if compromised. |
| Admin password | Every 6 months | Or immediately if compromised. |
| Postgres password | Every 12 months | Coordinate with DB maintenance. |
| TOTP key | Only on compromise | Rotation forces all users to re-enroll 2FA. |

---

## Emergency Rotation (Compromise Response)

If you suspect any secret has been compromised:

1. **Immediately** rotate the compromised secret(s) following the procedures above.
2. Rotate the owner token and API token even if only the encryption key was compromised (defense in depth).
3. Review hub audit logs for unauthorized access.
4. If the encryption key was compromised, assume all stored credentials are exposed -- rotate credentials stored in connector profiles as well.
5. After rotation, verify all integrations and connectors are functioning.
