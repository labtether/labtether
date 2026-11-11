package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Backups in PBS namespaces were invisible: the Hub listed only the root
// namespace, so a datastore whose backups all live in namespaces showed none.
// A token may also read only some namespaces; the readable ones still count.
func TestListAcrossNamespacesReadsEveryReadableNamespace(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	var forgot string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/api2/json/admin/datastore/store-a/"
		switch {
		case r.URL.Path == base+"namespace":
			_, _ = w.Write([]byte(`{"data":[{"ns":"site-a"},{"ns":"site-b"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == base+"groups":
			switch r.URL.Query().Get("ns") {
			case "": // the root: this token may not read it
				http.Error(w, `{"errors":"permission check failed"}`, http.StatusForbidden)
			case "site-a":
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-count":3}]}`))
			case "site-b":
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-count":1},{"backup-type":"ct","backup-id":"200"}]}`))
			}
		case r.Method == http.MethodGet && r.URL.Path == base+"snapshots":
			if r.URL.Query().Get("ns") == "site-b" {
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":1739985600}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodDelete && r.URL.Path == base+"groups":
			forgot = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL + "/api2/json", TokenID: "qa@pbs!t", TokenSecret: "s", SkipVerify: true, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	groups, err := client.ListDatastoreGroups(ctx, "store-a")
	if err != nil {
		t.Fatalf("ListDatastoreGroups: %v", err)
	}
	if len(groups) != 3 || groups[0].NS != "site-a" || groups[1].NS != "site-b" || groups[2].NS != "site-b" {
		t.Fatalf("groups = %+v, want the vm/100 of site-a and both groups of site-b, each with its namespace", groups)
	}
	snapshots, err := client.ListDatastoreSnapshots(ctx, "store-a")
	if err != nil || len(snapshots) != 1 || snapshots[0].NS != "site-b" {
		t.Fatalf("snapshots = %+v err=%v, want the one site-b snapshot", snapshots, err)
	}
	if err := client.ForgetGroup(ctx, "store-a", "site-b", "vm", "100"); err != nil {
		t.Fatalf("ForgetGroup: %v", err)
	}
	if forgot != "backup-type=vm&backup-id=100&ns=site-b" {
		t.Fatalf("forget query = %q, want the group's namespace", forgot)
	}
}

// PBS 4 sends {"version":"4.2","release":"10"}; the Hub showed "10" as the
// version on the device page and in Test connection.
func TestVersionDisplay(t *testing.T) {
	for _, tc := range []struct {
		v    Version
		want string
	}{
		{Version{Version: "4.2", Release: "10"}, "4.2.10"},
		{Version{Version: "3.4", Release: "3.4-1"}, "3.4-1"},
		{Version{Release: "3.2"}, "3.2"},
		{Version{Version: "4.2"}, "4.2"},
	} {
		if got := tc.v.Display(); got != tc.want {
			t.Errorf("%+v.Display() = %q, want %q", tc.v, got, tc.want)
		}
	}
}
