package persistence

import (
	"context"
	"time"
)

const touchAssetsSeenTimeout = 10 * time.Second

// TouchAssetsSeen refreshes last_seen_at and updated_at for unchanged assets in
// one statement.
func (s *PostgresStore) TouchAssetsSeen(ids []string, at time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), touchAssetsSeenTimeout)
	defer cancel()
	tag, err := s.pool.Exec(ctx,
		`UPDATE assets SET last_seen_at = $2, updated_at = $2 WHERE id = ANY($1)`,
		ids, at.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// TouchAssetsSeen mirrors the Postgres bulk refresh for the in-memory store.
func (m *MemoryAssetStore) TouchAssetsSeen(ids []string, at time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var touched int64
	for _, id := range ids {
		asset, ok := m.assets[id]
		if !ok {
			continue
		}
		asset.LastSeenAt = at.UTC()
		asset.UpdatedAt = at.UTC()
		m.assets[id] = asset
		touched++
	}
	return touched, nil
}
