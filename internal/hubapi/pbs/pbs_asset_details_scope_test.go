package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
)

func TestRestrictedPBSDatastoreDetailsOmitNodeWideTasks(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	var taskRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"version":"4.0"}}`))
		case "/api2/json/admin/datastore/backup/status":
			_, _ = w.Write([]byte(`{"data":{"store":"backup","mount-status":"mounted"}}`))
		case "/api2/json/admin/datastore/backup/groups", "/api2/json/admin/datastore/backup/snapshots":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/localhost/tasks":
			taskRequests.Add(1)
			_, _ = w.Write([]byte(`{"data":[{"upid":"UPID:own","worker_id":"backup:vm/101","user":"own"},{"upid":"UPID:sibling","worker_id":"backup-prod:vm/202","user":"sibling"}]}`))
		default:
			t.Errorf("unexpected PBS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := pbsconnector.NewClient(pbsconnector.Config{
		BaseURL: server.URL, TokenID: "test@pbs!token", TokenSecret: "test", SkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	d := &Deps{}
	asset := assets.Asset{ID: "backup-asset", Source: "pbs", Type: "storage-pool", Metadata: map[string]string{"store": "backup"}}
	runtime := &PBSRuntime{Client: client}

	unrestricted, err := d.LoadPBSAssetDetails(context.Background(), asset, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(unrestricted.Tasks) != 2 || taskRequests.Load() != 1 {
		t.Fatalf("expected existing node-wide task list for unrestricted access, tasks=%+v requests=%d", unrestricted.Tasks, taskRequests.Load())
	}

	restrictedContext := apiv2.ContextWithAllowedAssets(context.Background(), []string{asset.ID})
	restricted, err := d.LoadPBSAssetDetails(restrictedContext, asset, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(restricted.Tasks) != 0 || taskRequests.Load() != 1 {
		t.Fatalf("restricted datastore details must not fetch or expose node-wide tasks, tasks=%+v requests=%d", restricted.Tasks, taskRequests.Load())
	}
}
