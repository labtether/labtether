package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/synthetic"
)

const (
	serviceHealthLinkerInterval      = 5 * time.Minute
	serviceHealthLinkerStartupDelay  = 90 * time.Second
	serviceHealthLinkerCheckInterval = 60
)

// runServiceHealthLinker periodically ensures manual web services have linked
// synthetic HTTP health checks. Runs every 5 minutes after a 90s startup delay.
func (s *apiServer) runServiceHealthLinker(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(serviceHealthLinkerStartupDelay):
	}

	ticker := time.NewTicker(serviceHealthLinkerInterval)
	defer ticker.Stop()
	log.Printf("service health linker started (interval=5m)")

	// Run immediately on start, then on each tick.
	s.syncServiceHealthChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("service health linker stopped")
			return
		case <-ticker.C:
			s.syncServiceHealthChecks(ctx)
		}
	}
}

func (s *apiServer) syncServiceHealthChecks(ctx context.Context) {
	if s.webServiceCoordinator == nil || s.syntheticStore == nil {
		return
	}

	// List all manual services (empty host filter returns all).
	manuals, err := s.webServiceCoordinator.ListManualServices("")
	if err != nil {
		log.Printf("service health linker: failed to list manual services: %v", err)
		return
	}

	// Build a set of service IDs that still exist, for orphan cleanup.
	activeServiceIDs := make(map[string]struct{}, len(manuals))

	for _, svc := range manuals {
		serviceID := strings.TrimSpace(svc.ID)
		if serviceID == "" {
			continue
		}
		target := strings.TrimSpace(svc.URL)
		if target == "" {
			continue
		}

		activeServiceIDs[serviceID] = struct{}{}

		existing, lookupErr := s.syntheticStore.GetSyntheticCheckByServiceID(ctx, serviceID)
		if lookupErr != nil {
			log.Printf("service health linker: failed to look up check for service %s: %v", serviceID, lookupErr)
			continue
		}
		if existing != nil {
			// Check already linked; nothing to do.
			continue
		}

		name := strings.TrimSpace(svc.Name)
		if name == "" {
			name = serviceID
		}

		enabled := true
		req := synthetic.CreateCheckRequest{
			Name:            "Health: " + name,
			CheckType:       synthetic.CheckTypeHTTP,
			Target:          target,
			IntervalSeconds: serviceHealthLinkerCheckInterval,
			Enabled:         &enabled,
			ServiceID:       serviceID,
		}

		created, createErr := s.syntheticStore.CreateSyntheticCheck(req)
		if createErr != nil {
			log.Printf("service health linker: failed to create check for service %s: %v", serviceID, createErr)
			continue
		}
		log.Printf("service health linker: created synthetic check %s for service %s (%s)", created.ID, serviceID, target)
	}

	// Clean up orphaned checks: service_id set but service no longer exists.
	s.cleanOrphanedServiceHealthChecks(ctx, activeServiceIDs)
}

func (s *apiServer) cleanOrphanedServiceHealthChecks(ctx context.Context, activeServiceIDs map[string]struct{}) {
	checks, err := s.syntheticStore.ListSyntheticChecks(500, false)
	if err != nil {
		log.Printf("service health linker: failed to list checks for orphan cleanup: %v", err)
		return
	}

	for _, check := range checks {
		serviceID := strings.TrimSpace(check.ServiceID)
		if serviceID == "" {
			continue
		}
		if _, active := activeServiceIDs[serviceID]; active {
			continue
		}
		if err := s.syntheticStore.DeleteSyntheticCheck(check.ID); err != nil {
			log.Printf("service health linker: failed to delete orphaned check %s (service %s): %v", check.ID, serviceID, err)
			continue
		}
		log.Printf("service health linker: deleted orphaned check %s for removed service %s", check.ID, serviceID)
	}
}
