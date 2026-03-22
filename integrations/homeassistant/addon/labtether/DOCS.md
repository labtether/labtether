# LabTether Home Assistant Add-on

## Configuration

- `labtether_owner_token`: owner API token (optional when `auto_generate_credentials=true`)
- `labtether_admin_password`: local admin password (optional when `auto_generate_credentials=true`)
- `encryption_key`: base64 key that decodes to 32 bytes (optional when `auto_generate_credentials=true`)
- `database_url`: optional Postgres DSN. Leave blank to use bundled local Postgres in `/data/postgres`.
- `tls_mode`: `auto`, `external`, or `disabled`
- `auto_generate_credentials`: when enabled, missing required credentials are generated and persisted.

## Generated Credentials

When credentials are generated automatically, they are written to:

- `/data/labtether-addon/generated-credentials.txt`

Treat this file as sensitive.

## Networking

Default exposed ports:

- `8080/tcp` (HTTP)
- `8443/tcp` (HTTPS)

## Notes

This add-on runs the LabTether hub runtime.
For Home Assistant entities/services integration, install the custom integration in `integrations/homeassistant/custom_components/labtether`.
