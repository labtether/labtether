# Upgrading LabTether Hub

This guide covers the upgrade procedure for LabTether hub deployments, including database migration safety and rollback procedures.

## Before Upgrading

1. **Back up the database:**

   ```bash
   make db-backup
   # or
   ./scripts/db-backup.sh
   ```

   Backups are stored in `backups/` with 7-day retention by default.

2. **Note the current version:**

   Check the `schema_migrations` table for the latest applied migration:

   ```bash
   make db-migrate-status
   ```

   Or query directly:

   ```sql
   SELECT version, name, applied_at, checksum IS NOT NULL AS checksum_verified
   FROM schema_migrations ORDER BY version DESC LIMIT 5;
   ```

   Or check the container image tag:

   ```bash
   docker compose -f deploy/compose/docker-compose.deploy.yml ps
   ```

3. **Read the CHANGELOG** for breaking changes between your current version and the target version.

## Upgrade Procedure (Docker Compose)

```bash
# 1. Backup
make db-backup

# 2. Pull new images
./scripts/upgrade-compose.sh v2026.X
# or manually:
docker compose -f deploy/compose/docker-compose.deploy.yml pull

# 3. Restart (migrations run automatically on hub startup)
docker compose -f deploy/compose/docker-compose.deploy.yml up -d

# 4. Verify
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/readyz
docker compose logs labtether | grep "migration"
```

## Rollback Procedure

LabTether uses forward-only migrations. There are no Down() rollbacks. Rollback is performed by restoring from a database backup.

> **Important:** Each migration's SQL statements are checksummed (SHA-256) when first applied and re-verified on every subsequent startup. If a migration is modified after being applied the hub will refuse to start with a clear error message. This is intentional — migrations are append-only. If you see a checksum error after a failed partial upgrade, restore from backup.

1. **Stop the hub:**

   ```bash
   docker compose -f deploy/compose/docker-compose.deploy.yml down
   ```

2. **Restore the database:**

   ```bash
   ./scripts/db-restore.sh backups/latest.sql.gz
   ```

3. **Deploy the previous version:**

   Update the image tag in `.env.deploy` to the previous version, then restart:

   ```bash
   docker compose -f deploy/compose/docker-compose.deploy.yml up -d
   ```

> **Note:** Rollback is restore-from-backup only. Forward-only migrations mean you cannot undo individual migrations. Always ensure you have a recent backup before upgrading.

## When Upgrades Require Downtime

- Major version upgrades (e.g., v2026.1 to v2026.2) may include breaking migrations that alter table structures or rename columns.
- Always read the CHANGELOG before upgrading.
- Plan for 1-5 minutes of downtime during migration, depending on database size.
- For zero-downtime requirements, test the upgrade on a staging environment first.

## Troubleshooting

### Migration stuck

A stuck migration is usually caused by a held advisory lock. Check for it:

```sql
SELECT * FROM pg_locks WHERE locktype = 'advisory';
```

If a lock is held by a dead session, terminate it:

```sql
SELECT pg_terminate_backend(pid) FROM pg_stat_activity
WHERE pid IN (SELECT pid FROM pg_locks WHERE locktype = 'advisory');
```

### Migration failed

1. Check the hub logs for the specific error:

   ```bash
   docker compose logs labtether | grep -i "migration\|error"
   ```

2. Restore from backup:

   ```bash
   ./scripts/db-restore.sh backups/latest.sql.gz
   ```

3. Report the issue with the error message and migration version number.

### Migration checksum mismatch

If the hub logs an error like:

```
schema migration vN (name) has been modified after being applied (stored checksum ..., computed ...)
```

This means the SQL statements for an already-applied migration were edited in source code. Migrations are append-only — do **not** edit an existing migration. The correct resolution is:

1. Revert the source code change for that migration.
2. If the change was intentional and the environment is non-production, drop and recreate the database, then re-run `make db-migrate`.
3. For production environments, restore from a backup taken before the change was made.

### Hub won't start after upgrade

1. Check container logs:

   ```bash
   docker compose -f deploy/compose/docker-compose.deploy.yml logs labtether
   ```

2. Common causes:
   - Database connection failure: verify `DATABASE_URL` in `.env.deploy`
   - Migration error: check logs for the failing migration version, restore backup, report issue
   - Port conflict: ensure port 8080 is available

3. If the issue is migration-related, roll back using the procedure above.
