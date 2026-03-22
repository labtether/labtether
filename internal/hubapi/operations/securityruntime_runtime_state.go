package operations

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/securityruntime"
)

// BuildSecurityRuntimeEnvOverrides extracts security runtime env overrides from the given map.
func BuildSecurityRuntimeEnvOverrides(overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}

	values := make(map[string]string)
	if raw := strings.TrimSpace(overrides[runtimesettings.KeySecurityOutboundAllowPrivate]); raw != "" && !strings.EqualFold(raw, "auto") {
		values["LABTETHER_OUTBOUND_ALLOW_PRIVATE"] = raw
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// ApplySecurityRuntimeOverrides applies security runtime overrides from the given map.
func ApplySecurityRuntimeOverrides(overrides map[string]string) {
	securityruntime.SetRuntimeEnvOverrides(BuildSecurityRuntimeEnvOverrides(overrides))
}

// RefreshSecurityRuntimeSettingsDirect periodically refreshes security runtime settings from the database.
func RefreshSecurityRuntimeSettingsDirect(ctx context.Context, store persistence.RuntimeSettingsStore) {
	if store == nil {
		ApplySecurityRuntimeOverrides(nil)
		return
	}

	apply := func() {
		overrides, err := store.ListRuntimeSettingOverrides()
		if err != nil {
			log.Printf("labtether security runtime settings refresh failed: %v", err)
			return
		}
		ApplySecurityRuntimeOverrides(overrides)
	}

	apply()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("labtether security runtime settings refresh stopped")
			return
		case <-ticker.C:
			apply()
		}
	}
}
