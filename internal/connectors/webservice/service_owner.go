package webservice

import (
	"context"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
)

// ResolveServiceHost identifies one exact current service owner. Manual rows are
// read live; only agent reports come from the in-memory coordinator snapshot.
// It never infers ownership from URLs or the presentation cache.
func (c *Coordinator) ResolveServiceHost(ctx context.Context, serviceID string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if c == nil || serviceID == "" || strings.TrimSpace(serviceID) != serviceID {
		return "", false, nil
	}
	store, ok := c.store.(persistence.WebServiceIdentityContextStore)
	if !ok {
		return "", false, ErrStoreUnavailable
	}
	manual, found, err := store.GetManualWebServiceContext(ctx, serviceID)
	if err != nil {
		return "", false, err
	}
	ownerID := ""
	accept := func(id string) bool {
		if id == "" || strings.TrimSpace(id) != id || (ownerID != "" && ownerID != id) {
			return false
		}
		ownerID = id
		return true
	}
	if found && (manual.ID != serviceID || !accept(manual.HostAssetID)) {
		return "", false, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, host := range c.hosts {
		for _, service := range host.services {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			if service.ID == serviceID && !accept(service.HostAssetID) {
				return "", false, nil
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return ownerID, ownerID != "", nil
}
