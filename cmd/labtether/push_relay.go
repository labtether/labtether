package main

import (
	"context"
	"log"
	"os"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/securityruntime"
)

const pushRelayURLEnv = "LABTETHER_PUSH_RELAY_URL"

// pushRelayURLFromEnv returns the LabTether push relay base URL. Unset means
// the official relay; an empty value or "off" disables relay mode, so APNs
// channels then need the operator's own key. An invalid value disables relay
// mode with a warning rather than sending pushes to an unintended host.
func pushRelayURLFromEnv() string {
	raw, set := os.LookupEnv(pushRelayURLEnv)
	if !set {
		raw = notifications.DefaultPushRelayURL
	}
	relayURL, err := notifications.ParsePushRelayURL(raw)
	if err != nil {
		securityruntime.Logf("labtether warning: %s is invalid (%v); the iOS push relay is disabled", pushRelayURLEnv, err)
		return ""
	}
	if relayURL == "" {
		securityruntime.Logf("labtether: iOS push relay disabled by %s", pushRelayURLEnv)
		return ""
	}
	securityruntime.Logf("labtether: iOS push relay at %s", relayURL)
	return relayURL
}

// ensureDefaultRelayPushRoute runs after a relay-backed iOS device registers.
// It creates the default relay channel and route once, only when no APNs
// route exists. Failures are logged without device data and retried on the
// next relay registration.
func (s *apiServer) ensureDefaultRelayPushRoute(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	created, err := s.ensureAlertingDeps().EnsureDefaultRelayPushRoute(ctx, s.db)
	if err != nil {
		securityruntime.Logf("notifications: default iOS push relay channel setup failed: %v", err)
		return
	}
	if created {
		log.Printf("notifications: created the default iOS push relay channel and route")
	}
}
