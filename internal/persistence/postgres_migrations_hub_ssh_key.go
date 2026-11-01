package persistence

// schemaMigrationAssetHubSSHKeySettings stores the per-device Hub SSH key
// setting. A device without a row has the key off, so devices added after
// this migration start off. Every device that exists when it runs keeps
// today's behaviour: the Hub keeps adding its key until an admin turns it off.
func schemaMigrationAssetHubSSHKeySettings() schemaMigration {
	return schemaMigration{Version: 107, Name: "asset_hub_ssh_key_settings", Statements: []string{
		`CREATE TABLE IF NOT EXISTS asset_hub_ssh_keys (
 asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
 enabled BOOLEAN NOT NULL DEFAULT FALSE,
 removal_pending BOOLEAN NOT NULL DEFAULT FALSE,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
		`INSERT INTO asset_hub_ssh_keys (asset_id, enabled)
 SELECT id, TRUE FROM assets
 ON CONFLICT (asset_id) DO NOTHING`,
	}}
}
