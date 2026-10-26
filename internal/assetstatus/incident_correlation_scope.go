package assetstatus

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/persistence"
)

var _ persistence.GroupAssetIDScopeReader = (*Store)(nil)

// IDs need no status decoration. A missing context owner is unknown, never a
// reason to fall back to an uncancellable full inventory read.
func (s *Store) ReadGroupAssetIDsContext(ctx context.Context, groupID string, visit func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, ok := s.AssetStore.(persistence.GroupAssetIDScopeReader)
	if !ok {
		return fmt.Errorf("incident group scope reader unavailable")
	}
	return owner.ReadGroupAssetIDsContext(ctx, groupID, visit)
}
