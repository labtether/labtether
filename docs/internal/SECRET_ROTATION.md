# Secret Rotation Runbook

This document covers the secrets used by LabTether Hub, how to rotate each one,
and the recommended rotation schedule.

---

## Inventory of Secrets

| Secret | Env Var | Purpose |
|--------|---------|---------|
| Encryption key | `LABTETHER_ENCRYPTION_KEY` | AES-256-GCM encryption of stored credentials (connector passwords, API keys in DB). Also used to derive the TOTP encryption key via HKDF. |
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
stored credentials unreadable.

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

4. **Re-encrypt existing credentials (if any).**
   Currently there is no built-in CLI for bulk re-encryption. If you have
   stored credential profiles, you must:
   - Export each credential using the old key (write a small Go program using
     `secrets.NewManagerFromEncodedKey(oldKey)` + `DecryptString`).
   - Re-encrypt with the new key (`NewManagerFromEncodedKey(newKey)` +
     `EncryptString`), preserving the same AAD (credential profile ID).
   - Update the rows in Postgres.

   If no credential profiles exist yet, skip this step.

5. **Update the key.**
   - Edit `.env` and set `LABTETHER_ENCRYPTION_KEY` to the new value.
   - Also delete or update the persisted install-state file
     (`data/install/secrets.json`) so the hub does not restore the old key.

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

---

## Rotating the Owner Token

The owner token is the primary API authentication bearer token.

### Procedure

1. **Generate a new token.**
   ```bash
   openssl rand -hex 32
   ```

2. **Update `.env`.**
   Set `LABTETHER_OWNER_TOKEN` to the new value.

3. **Update the install-state file** (or delete it to let the hub regenerate).
   ```bash
   rm data/install/secrets.json   # hub will persist the new env value on next start
   ```

4. **Restart the hub.**
   ```bash
   docker compose restart labtether
   ```

5. **Update all clients** that use the owner token (scripts, CLI
   configurations, bookmarks with token query params).

---

## Rotating the API Token

The API token is used by non-browser clients talking through the web-console
proxy.

### Procedure

Follow the same steps as the owner token rotation, substituting
`LABTETHER_API_TOKEN`.

If `LABTETHER_API_TOKEN_FILE` is set, the hub writes the active token to that
path on startup. Downstream services reading that file will pick up the new
token automatically after restart.

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
