package main

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/pbs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadPBSAssetDetailsDatastoreAndServerBranches(t *testing.T) {
	t.Run("datastore asset missing store metadata", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api2/json/version" {
				_, _ = w.Write([]byte(`{"data":{"release":"3.2-1"}}`))
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-metadata-missing",
		}
		_, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-a",
			Type:   "storage-pool",
			Source: "pbs",
		}, runtime)
		if err == nil || !strings.Contains(err.Error(), "missing store metadata") {
			t.Fatalf("expected missing store metadata error, got %v", err)
		}
	})

	t.Run("datastore details with version fallback and filtered tasks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"","version":"3.2"}}`))
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":1000,"used":250,"avail":750,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"}]}`))
			case "/api2/json/admin/datastore/backup/snapshots":
				_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":%d}]}`, time.Now().Unix()-300)))
			case "/api2/json/nodes/node-a/tasks":
				_, _ = w.Write([]byte(`{"data":[
					{"upid":"UPID:1:backup:","node":"node-a","worker_type":"verify","worker_id":"backup:vm/100","starttime":20},
					{"upid":"UPID:2:other:","node":"node-a","worker_type":"verify","worker_id":"other:vm/101","starttime":30},
					{"upid":"UPID:3:backup:","node":"node-a","worker_type":"gc","worker_id":"","starttime":10}
				]}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-datastore-success",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-datastore-backup",
			Type:   "storage-pool",
			Source: "pbs",
			Metadata: map[string]string{
				"store": "backup",
				"node":  "node-a",
			},
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if response.Kind != "datastore" || response.Store != "backup" {
			t.Fatalf("unexpected datastore response kind/store: %+v", response)
		}
		if response.Version != "3.2" {
			t.Fatalf("expected version fallback to 3.2, got %q", response.Version)
		}
		if len(response.Tasks) != 2 {
			t.Fatalf("expected filtered tasks=2, got %d", len(response.Tasks))
		}
	})

	t.Run("datastore warnings when version and tasks unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				http.Error(w, `{"errors":"version failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":1000,"used":250,"avail":750,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/backup/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/localhost/tasks":
				http.Error(w, `{"errors":"tasks failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-datastore-warnings",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-datastore-backup",
			Type:   "storage-pool",
			Source: "pbs",
			Metadata: map[string]string{
				"store": "backup",
			},
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if len(response.Warnings) == 0 {
			t.Fatalf("expected warnings in datastore response")
		}
		if !strings.Contains(strings.Join(response.Warnings, " | "), "version unavailable") {
			t.Fatalf("expected version warning, got %v", response.Warnings)
		}
		if !strings.Contains(strings.Join(response.Warnings, " | "), "task listing unavailable") {
			t.Fatalf("expected task warning, got %v", response.Warnings)
		}
	})

	t.Run("server details usage-summary and task warnings", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-1","version":"3.4"}}`))
			case "/api2/json/status/datastore-usage":
				http.Error(w, `{"errors":"usage failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[
					{"store":"","comment":"skip"},
					{"store":"bad","comment":"bad-comment"},
					{"store":"good","comment":"good-comment","mount-status":"mounted","maintenance":"read-only"}
				]}`))
			case "/api2/json/admin/datastore/bad/status":
				http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore/good/status":
				_, _ = w.Write([]byte(`{"data":{"store":"good","total":1000,"used":100,"avail":900,"mount-status":""}}`))
			case "/api2/json/admin/datastore/good/groups":
				_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"}]}`))
			case "/api2/json/admin/datastore/good/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/localhost/tasks":
				http.Error(w, `{"errors":"tasks failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-server-warnings",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-main",
			Type:   "storage-controller",
			Source: "pbs",
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if response.Kind != "server" {
			t.Fatalf("expected server kind, got %q", response.Kind)
		}
		if len(response.Datastores) != 1 {
			t.Fatalf("expected one usable datastore summary, got %d", len(response.Datastores))
		}
		summary := response.Datastores[0]
		if summary.Store != "good" {
			t.Fatalf("expected datastore summary for store=good, got %q", summary.Store)
		}
		if summary.Comment != "good-comment" {
			t.Fatalf("expected comment fallback from datastore listing, got %q", summary.Comment)
		}
		if summary.MountStatus != "mounted" {
			t.Fatalf("expected mount-status fallback from datastore listing, got %q", summary.MountStatus)
		}
		if summary.Maintenance != "read-only" {
			t.Fatalf("expected maintenance fallback from datastore listing, got %q", summary.Maintenance)
		}
		combined := strings.Join(response.Warnings, " | ")
		if !strings.Contains(combined, "datastore usage unavailable") ||
			!strings.Contains(combined, "datastore bad unavailable") ||
			!strings.Contains(combined, "task listing unavailable") {
			t.Fatalf("unexpected server warnings: %v", response.Warnings)
		}
	})

	t.Run("server details usage-success sorted summaries and tasks", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-2","version":"3.4"}}`))
			case "/api2/json/status/datastore-usage":
				_, _ = w.Write([]byte(`{"data":[
					{"store":" beta ","total":1000,"used":400,"avail":600},
					{"store":"alpha","total":2000,"used":500,"avail":1500},
					{"store":"","total":1,"used":1,"avail":0}
				]}`))
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[
					{"store":"beta","comment":"store-beta","mount-status":"mounted"},
					{"store":"alpha","comment":"store-alpha","mount-status":"mounted"}
				]}`))
			case "/api2/json/admin/datastore/alpha/status":
				_, _ = w.Write([]byte(`{"data":{"store":"alpha","total":2000,"used":500,"avail":1500,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/alpha/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/alpha/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/beta/status":
				_, _ = w.Write([]byte(`{"data":{"store":"beta","total":1000,"used":400,"avail":600,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/beta/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/beta/snapshots":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/localhost/tasks":
				_, _ = w.Write([]byte(`{"data":[
					{"upid":"UPID-2","node":"localhost","worker_type":"verify","worker_id":"beta","starttime":20},
					{"upid":"UPID-1","node":"localhost","worker_type":"verify","worker_id":"alpha","starttime":10}
				]}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-server-success",
		}
		response, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-main",
			Type:   "storage-controller",
			Source: "pbs",
		}, runtime)
		if err != nil {
			t.Fatalf("loadPBSAssetDetails() error = %v", err)
		}
		if len(response.Datastores) != 2 {
			t.Fatalf("expected two datastore summaries, got %d", len(response.Datastores))
		}
		if response.Datastores[0].Store != "alpha" || response.Datastores[1].Store != "beta" {
			t.Fatalf("expected sorted datastore order [alpha,beta], got %+v", response.Datastores)
		}
		if len(response.Tasks) != 2 {
			t.Fatalf("expected server tasks to be included, got %d", len(response.Tasks))
		}
	})

	t.Run("server details fail when datastores list fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-1"}}`))
			case "/api2/json/status/datastore-usage":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore":
				http.Error(w, `{"errors":"list failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		runtime := &pbsRuntime{
			Client:      mustNewPBSClient(t, server.URL),
			CollectorID: "collector-pbs-server-error",
		}
		_, err := newTestAPIServer(t).loadPBSAssetDetails(context.Background(), assets.Asset{
			ID:     "pbs-server-main",
			Type:   "storage-controller",
			Source: "pbs",
		}, runtime)
		if err == nil {
			t.Fatalf("expected datastores list error")
		}
	})
}

func TestLoadPBSDatastoreSummaryBranches(t *testing.T) {
	t.Run("status fetch error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api2/json/admin/datastore/backup/status" {
				http.Error(w, `{"errors":"status failed"}`, http.StatusBadGateway)
				return
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		defer server.Close()

		_, _, err := loadPBSDatastoreSummary(context.Background(), mustNewPBSClient(t, server.URL), "backup", pbs.DatastoreUsage{})
		if err == nil {
			t.Fatalf("expected datastore status error")
		}
	})

	t.Run("usage fallback with groups/snapshots warnings", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":0,"used":0,"avail":0,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				http.Error(w, `{"errors":"groups failed"}`, http.StatusBadGateway)
			case "/api2/json/admin/datastore/backup/snapshots":
				http.Error(w, `{"errors":"snapshots failed"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		summary, warnings, err := loadPBSDatastoreSummary(
			context.Background(),
			mustNewPBSClient(t, server.URL),
			"backup",
			pbs.DatastoreUsage{Store: "backup", Total: 200, Used: 20, Avail: 180},
		)
		if err != nil {
			t.Fatalf("loadPBSDatastoreSummary() error = %v", err)
		}
		if summary.TotalBytes != 200 || summary.UsedBytes != 20 || summary.AvailBytes != 180 {
			t.Fatalf("expected usage fallback totals, got %+v", summary)
		}
		if summary.UsagePercent != 10 {
			t.Fatalf("expected usage percent 10, got %v", summary.UsagePercent)
		}
		if len(warnings) != 2 {
			t.Fatalf("expected two warnings, got %d (%v)", len(warnings), warnings)
		}
	})

	t.Run("usage clamp and future backup day clamp", func(t *testing.T) {
		future := time.Now().UTC().Add(2 * time.Hour).Unix()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/admin/datastore/backup/status":
				_, _ = w.Write([]byte(`{"data":{"store":"backup","total":100,"used":150,"avail":0,"mount-status":"mounted"}}`))
			case "/api2/json/admin/datastore/backup/groups":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/admin/datastore/backup/snapshots":
				_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"backup-type":"vm","backup-id":"100","backup-time":%d}]}`, future)))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		summary, warnings, err := loadPBSDatastoreSummary(context.Background(), mustNewPBSClient(t, server.URL), "backup", pbs.DatastoreUsage{})
		if err != nil {
			t.Fatalf("loadPBSDatastoreSummary() error = %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings, got %v", warnings)
		}
		if summary.UsagePercent != 100 {
			t.Fatalf("expected usage clamp to 100, got %v", summary.UsagePercent)
		}
		if summary.LastBackupAt == "" {
			t.Fatalf("expected last backup timestamp")
		}
		if summary.DaysSinceBackup != 0 {
			t.Fatalf("expected future backup day clamp to 0, got %v", summary.DaysSinceBackup)
		}
	})
}
