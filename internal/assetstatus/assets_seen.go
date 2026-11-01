package assetstatus

import (
	"errors"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

var errAssetSeenUnsupported = errors.New("asset store has no bulk last-seen refresh")

// TouchAssetsSeen passes the bulk last-seen refresh to the wrapped store.
// Callers fall back to full heartbeat writes when it returns an error.
func (s *Store) TouchAssetsSeen(ids []string, at time.Time) (int64, error) {
	store, ok := s.AssetStore.(persistence.AssetSeenStore)
	if !ok {
		return 0, errAssetSeenUnsupported
	}
	return store.TouchAssetsSeen(ids, at)
}
