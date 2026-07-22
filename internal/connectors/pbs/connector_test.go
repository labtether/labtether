package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestConnectorUnconfiguredModeFailsClosed(t *testing.T) {
	connector := NewWithConfig(Config{})

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection returned unexpected error: %v", err)
	}
	if health.Status != "failed" || !strings.Contains(strings.ToLower(health.Message), "not configured") {
		t.Fatalf("unexpected unconfigured health response: %+v", health)
	}

	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover returned unexpected error: %v", err)
	}
	if assets == nil || len(assets) != 0 {
		t.Fatalf("expected non-nil empty unconfigured inventory, got %+v", assets)
	}

	for _, descriptor := range connector.Actions() {
		for _, dryRun := range []bool{false, true} {
			result, execErr := connector.ExecuteAction(context.Background(), descriptor.ID, connectorsdk.ActionRequest{
				Params: map[string]string{"store": "disposable"},
				DryRun: dryRun,
			})
			if execErr != nil || result.Status != "failed" || !strings.Contains(result.Message, "not configured") {
				t.Fatalf("ExecuteAction(%q, dry_run=%v) = %+v, err=%v, want fail-closed unconfigured result", descriptor.ID, dryRun, result, execErr)
			}
		}
	}
}

func TestConnectorDiscoverAndAction(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/ping":
			_, _ = w.Write([]byte(`{"data":{"pong":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"3.4-1","version":"3.4"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore":
			_, _ = w.Write([]byte(`{"data":[{"store":"store-a","comment":"Main Store","mount-status":"mounted"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/status/datastore-usage":
			_, _ = w.Write([]byte(`{"data":[{"store":"store-a","total":1000,"used":500,"avail":500}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/store-a/status":
			_, _ = w.Write([]byte(`{"data":{"store":"store-a","total":1000,"used":500,"avail":500,"mount-status":"mounted"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/store-a/groups":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-count":2,"last-backup":1739985600}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/store-a/snapshots":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":1739985600}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/admin/datastore/store-a/verify":
			_, _ = w.Write([]byte(`{"data":"UPID:VERIFY:1"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	connector := NewWithConfig(Config{
		BaseURL:     server.URL,
		TokenID:     "root@pam!labtether",
		TokenSecret: "secret123",
		SkipVerify:  true,
		Timeout:     5 * time.Second,
	})

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection returned unexpected error: %v", err)
	}
	if health.Status != "ok" || !strings.Contains(health.Message, "3.4-1") {
		t.Fatalf("unexpected health response: %+v", health)
	}

	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected root + datastore assets, got %d", len(assets))
	}
	if assets[1].ID != "pbs-datastore-store-a" {
		t.Fatalf("unexpected datastore asset id: %s", assets[1].ID)
	}

	result, err := connector.ExecuteAction(context.Background(), "datastore.verify", connectorsdk.ActionRequest{
		TargetID: "store-a",
		Params:   map[string]string{"store": "store-a"},
	})
	if err != nil {
		t.Fatalf("ExecuteAction returned unexpected error: %v", err)
	}
	if result.Status != "succeeded" || result.Metadata["upid"] != "UPID:VERIFY:1" {
		t.Fatalf("unexpected action result: %+v", result)
	}

	dryRunResult, err := connector.ExecuteAction(context.Background(), "datastore.gc", connectorsdk.ActionRequest{
		TargetID: "store-a",
		DryRun:   true,
		Params:   map[string]string{"store": "store-a"},
	})
	if err != nil {
		t.Fatalf("ExecuteAction dry-run returned unexpected error: %v", err)
	}
	if dryRunResult.Status != "succeeded" || !strings.Contains(dryRunResult.Output, "would run") {
		t.Fatalf("unexpected dry-run result: %+v", dryRunResult)
	}

	scopedDryRun, err := connector.ExecuteAction(context.Background(), "datastore.gc", connectorsdk.ActionRequest{
		TargetID: assetid.ScopeCollectorAssetID("pbs-datastore-store-a", "collector-pbs-a"),
		DryRun:   true,
	})
	if err != nil || !strings.Contains(scopedDryRun.Output, `datastore "store-a"`) {
		t.Fatalf("scoped target was not resolved to store-a: result=%+v err=%v", scopedDryRun, err)
	}
}

func TestConnectorAdvertisedActionsDryRunAndUnknownFailClosed(t *testing.T) {
	connector := NewWithConfig(Config{
		BaseURL:     "https://pbs.invalid",
		TokenID:     "root@pam!ltqa",
		TokenSecret: "disposable",
	})
	for _, descriptor := range connector.Actions() {
		result, err := connector.ExecuteAction(context.Background(), descriptor.ID, connectorsdk.ActionRequest{
			Params: map[string]string{"store": "ltqa-store"},
			DryRun: true,
		})
		if err != nil || result.Status != "succeeded" {
			t.Fatalf("advertised action %q dry-run = %+v, err=%v", descriptor.ID, result, err)
		}
	}
	unknown, err := connector.ExecuteAction(context.Background(), "datastore.destroy", connectorsdk.ActionRequest{
		Params: map[string]string{"store": "ltqa-store"},
		DryRun: true,
	})
	if err != nil || unknown.Status != "failed" || !strings.Contains(unknown.Message, "unsupported") {
		t.Fatalf("unknown dry-run did not fail closed: %+v, err=%v", unknown, err)
	}
}
