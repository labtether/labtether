package persistence

import (
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) GetHubSSHKeySetting(assetID string) (HubSSHKeySetting, error) {
	ctx, cancel := agentLifecycleStoreContext()
	defer cancel()
	id := strings.TrimSpace(assetID)
	setting := HubSSHKeySetting{AssetID: id}
	err := s.pool.QueryRow(ctx,
		`SELECT enabled, removal_pending, updated_at FROM asset_hub_ssh_keys WHERE asset_id = $1`,
		id,
	).Scan(&setting.Enabled, &setting.RemovalPending, &setting.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return HubSSHKeySetting{AssetID: id}, nil
	}
	if err != nil {
		return HubSSHKeySetting{}, err
	}
	return setting, nil
}

func (s *PostgresStore) SetHubSSHKeySetting(assetID string, enabled, removalPending bool) (HubSSHKeySetting, error) {
	id := strings.TrimSpace(assetID)
	if id == "" {
		return HubSSHKeySetting{}, errHubSSHKeyAssetIDRequired
	}
	ctx, cancel := agentLifecycleStoreContext()
	defer cancel()
	setting := HubSSHKeySetting{AssetID: id}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO asset_hub_ssh_keys (asset_id, enabled, removal_pending, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (asset_id) DO UPDATE
		 SET enabled = EXCLUDED.enabled,
		     removal_pending = EXCLUDED.removal_pending,
		     updated_at = EXCLUDED.updated_at
		 RETURNING enabled, removal_pending, updated_at`,
		id, enabled, removalPending && !enabled, time.Now().UTC(),
	).Scan(&setting.Enabled, &setting.RemovalPending, &setting.UpdatedAt)
	if err != nil {
		return HubSSHKeySetting{}, err
	}
	return setting, nil
}

func (s *PostgresStore) ClearHubSSHKeyRemovalPending(assetID string) error {
	ctx, cancel := agentLifecycleStoreContext()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE asset_hub_ssh_keys
		 SET removal_pending = FALSE, updated_at = $2
		 WHERE asset_id = $1 AND enabled = FALSE AND removal_pending`,
		strings.TrimSpace(assetID), time.Now().UTC(),
	)
	return err
}
