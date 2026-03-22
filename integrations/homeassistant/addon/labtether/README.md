# LabTether Home Assistant Add-on

This add-on runs the LabTether hub runtime inside Home Assistant.

## What This Add-on Provides

- Starts the LabTether hub binary (`cmd/labtether`) inside the add-on container.
- Supports either:
  - external Postgres via `database_url`, or
  - bundled local Postgres initialized in `/data/postgres` when `database_url` is empty.
- Persists generated/runtime credentials under `/data/labtether-addon/`.

## Required / Recommended Options

- `labtether_owner_token` (optional when `auto_generate_credentials=true`)
- `labtether_admin_password` (optional when `auto_generate_credentials=true`)
- `encryption_key` (optional when `auto_generate_credentials=true`; must decode to 32 bytes)
- `database_url` (optional; leave empty for local Postgres)
- `tls_mode` (`auto`, `external`, `disabled`)
- `auto_generate_credentials` (`true` recommended for first install)

## Generated Credentials

When auto-generation is enabled and required values are missing, the add-on writes generated values to:

- `/data/labtether-addon/generated-credentials.txt`

Treat this file as sensitive.

## Notes

- This add-on package currently targets the LabTether hub runtime and API endpoints.
- For Home Assistant entity/sensor integration, continue using the custom integration in `integrations/homeassistant/custom_components/labtether`.

## Release Automation

- Workflow: `.github/workflows/homeassistant-addon-release.yml`.
- Produces:
  - GHCR images per architecture (`labtether-homeassistant-addon-amd64`, `labtether-homeassistant-addon-aarch64`),
  - repository layout artifacts (`dist/ha-addon-repository` + tarball),
  - hosted repository branch `homeassistant-addon-repo`.
