package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientInvalidCAPEM(t *testing.T) {
	_, err := NewClient(Config{
		BaseURL:     "https://pbs.local:8007",
		TokenID:     "root@pam!labtether",
		TokenSecret: "secret",
		CAPEM:       "not-a-pem",
	})
	if err == nil {
		t.Fatalf("expected invalid PBS CA PEM error")
	}
}

func TestClientPingVersionAndAuthHeader(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	const authHeader = "PBSAPIToken root@pam!labtether:secret123"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != authHeader {
			t.Fatalf("unexpected authorization header: %q", got)
		}

		switch r.URL.Path {
		case "/api2/json/ping":
			_, _ = w.Write([]byte(`{"data":{"pong":true}}`))
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"3.4-1","version":"3.4"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		TokenID:     "root@pam!labtether",
		TokenSecret: "secret123",
		SkipVerify:  true,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ping, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if !ping.Pong {
		t.Fatalf("expected pong=true")
	}

	version, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if version.Release != "3.4-1" {
		t.Fatalf("unexpected release: %q", version.Release)
	}
}

func TestClientDatastoreTaskAndActionEndpoints(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore":
			_, _ = w.Write([]byte(`{"data":[{"store":"store-a","comment":"Main Store","mount-status":"mounted"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/store-a/status":
			_, _ = w.Write([]byte(`{"data":{"store":"store-a","total":1000,"used":400,"avail":600,"mount-status":"mounted","gc-status":{"upid":"UPID:GC:1","removed-bytes":42}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/store-a/groups":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-count":2,"last-backup":1739985600}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/admin/datastore/store-a/snapshots":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":1739985600,"protected":true}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/status/datastore-usage":
			_, _ = w.Write([]byte(`{"data":[{"store":"store-a","total":1000,"used":400,"avail":600,"mount-status":"mounted"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pbs/tasks":
			if got := r.URL.Query().Get("limit"); got != "5" {
				t.Fatalf("unexpected limit query: %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"upid":"UPID:1","node":"pbs","worker_type":"verify","status":"running"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pbs/tasks/UPID:1/status":
			_, _ = w.Write([]byte(`{"data":{"upid":"UPID:1","node":"pbs","status":"stopped","exitstatus":"OK"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pbs/tasks/UPID:1/log":
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"started"},{"n":2,"t":"done"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/pbs/tasks/UPID:1":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/admin/datastore/store-a/verify":
			_, _ = w.Write([]byte(`{"data":"UPID:VERIFY:1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/admin/datastore/store-a/prune-datastore":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.Form.Get("dry-run"); got != "1" {
				t.Fatalf("expected dry-run=1, got %q", got)
			}
			if got := r.Form.Get("keep-daily"); got != "7" {
				t.Fatalf("expected keep-daily=7, got %q", got)
			}
			_, _ = w.Write([]byte(`{"data":"UPID:PRUNE:1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/admin/datastore/store-a/gc":
			_, _ = w.Write([]byte(`{"data":"UPID:GC:2"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL + "/api2/json",
		TokenID:     "root@pam!labtether",
		TokenSecret: "secret123",
		SkipVerify:  true,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	datastores, err := client.ListDatastores(context.Background())
	if err != nil || len(datastores) != 1 || datastores[0].Store != "store-a" {
		t.Fatalf("ListDatastores unexpected result: datastores=%+v err=%v", datastores, err)
	}

	status, err := client.GetDatastoreStatus(context.Background(), "store-a", true)
	if err != nil || status.Total != 1000 || status.Used != 400 {
		t.Fatalf("GetDatastoreStatus unexpected result: status=%+v err=%v", status, err)
	}

	groups, err := client.ListDatastoreGroups(context.Background(), "store-a")
	if err != nil || len(groups) != 1 || groups[0].BackupID != "100" {
		t.Fatalf("ListDatastoreGroups unexpected result: groups=%+v err=%v", groups, err)
	}

	snapshots, err := client.ListDatastoreSnapshots(context.Background(), "store-a")
	if err != nil || len(snapshots) != 1 || snapshots[0].BackupTime == 0 {
		t.Fatalf("ListDatastoreSnapshots unexpected result: snapshots=%+v err=%v", snapshots, err)
	}

	usage, err := client.ListDatastoreUsage(context.Background())
	if err != nil || len(usage) != 1 || usage[0].Store != "store-a" {
		t.Fatalf("ListDatastoreUsage unexpected result: usage=%+v err=%v", usage, err)
	}

	tasks, err := client.ListNodeTasks(context.Background(), "pbs", 5)
	if err != nil || len(tasks) != 1 || !strings.Contains(tasks[0].UPID, "UPID:1") {
		t.Fatalf("ListNodeTasks unexpected result: tasks=%+v err=%v", tasks, err)
	}

	taskStatus, err := client.GetTaskStatus(context.Background(), "pbs", "UPID:1")
	if err != nil || taskStatus.ExitStatus != "OK" {
		t.Fatalf("GetTaskStatus unexpected result: status=%+v err=%v", taskStatus, err)
	}

	logLines, err := client.GetTaskLog(context.Background(), "pbs", "UPID:1", 100)
	if err != nil || len(logLines) != 2 || logLines[0].Text != "started" {
		t.Fatalf("GetTaskLog unexpected result: lines=%+v err=%v", logLines, err)
	}

	if err := client.StopTask(context.Background(), "pbs", "UPID:1"); err != nil {
		t.Fatalf("StopTask failed: %v", err)
	}

	verifyUPID, err := client.StartVerify(context.Background(), "store-a")
	if err != nil || verifyUPID != "UPID:VERIFY:1" {
		t.Fatalf("StartVerify unexpected result: upid=%q err=%v", verifyUPID, err)
	}

	pruneUPID, err := client.StartPruneDatastore(context.Background(), "store-a", PruneOptions{DryRun: true, KeepDaily: 7})
	if err != nil || pruneUPID != "UPID:PRUNE:1" {
		t.Fatalf("StartPruneDatastore unexpected result: upid=%q err=%v", pruneUPID, err)
	}

	gcUPID, err := client.StartGC(context.Background(), "store-a")
	if err != nil || gcUPID != "UPID:GC:2" {
		t.Fatalf("StartGC unexpected result: upid=%q err=%v", gcUPID, err)
	}
}
