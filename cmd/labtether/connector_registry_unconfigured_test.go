package main

import (
	"context"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestBuildConnectorRegistryAdvertisesUnconfiguredConnectorsFailClosed(t *testing.T) {
	for _, key := range []string{
		"PROXMOX_BASE_URL", "PROXMOX_TOKEN_ID", "PROXMOX_TOKEN_SECRET", "PROXMOX_CA_CERT_PEM",
		"PBS_BASE_URL", "PBS_TOKEN_ID", "PBS_TOKEN_SECRET", "PBS_CA_CERT_PEM",
		"HA_BASE_URL", "HA_TOKEN",
		"PORTAINER_BASE_URL", "PORTAINER_API_KEY", "PORTAINER_USERNAME", "PORTAINER_PASSWORD",
	} {
		t.Setenv(key, "")
	}

	registry := buildConnectorRegistry()
	wantIDs := []string{"home-assistant", "pbs", "portainer", "proxmox", "truenas"}
	descriptors := registry.List()
	if len(descriptors) != len(wantIDs) {
		t.Fatalf("registry advertised %d connectors, want %d: %+v", len(descriptors), len(wantIDs), descriptors)
	}

	ctx := context.Background()
	for _, connectorID := range wantIDs {
		connector, ok := registry.Get(connectorID)
		if !ok {
			t.Fatalf("production registry does not advertise %q", connectorID)
		}

		health, err := connector.TestConnection(ctx)
		if err != nil || health.Status != "failed" || !strings.Contains(strings.ToLower(health.Message), "not configured") {
			t.Fatalf("%s TestConnection() = %+v, err=%v, want failed unconfigured health", connectorID, health, err)
		}

		assets, err := connector.Discover(ctx)
		if err != nil || assets == nil || len(assets) != 0 {
			t.Fatalf("%s Discover() = %+v, err=%v, want non-nil empty inventory", connectorID, assets, err)
		}

		actions := connector.Actions()
		if len(actions) == 0 {
			t.Fatalf("%s has no advertised actions to exercise", connectorID)
		}
		for _, action := range actions {
			for _, dryRun := range []bool{false, true} {
				result, execErr := connector.ExecuteAction(ctx, action.ID, connectorsdk.ActionRequest{
					TargetID: "pve01/100",
					Params: map[string]string{
						"service": "light.turn_on",
						"store":   "disposable",
					},
					DryRun: dryRun,
				})
				if execErr != nil || result.Status != "failed" || !strings.Contains(strings.ToLower(result.Message), "not configured") {
					t.Fatalf("%s action %q dry_run=%v = %+v, err=%v, want fail-closed unconfigured result", connectorID, action.ID, dryRun, result, execErr)
				}
			}
		}
	}
}
