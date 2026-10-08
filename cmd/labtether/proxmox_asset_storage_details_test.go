package main

import (
	"context"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadProxmoxAssetDetailsStorageContentSorting(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":300,"avail":700}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"local-zfs:z-vol","content":"rootdir"},{"volid":"local-zfs:a-backup","content":"backup"}]}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "local-zfs"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails storage branch failed: %v", err)
	}
	if details.Config["total"] != float64(1000) {
		t.Fatalf("expected storage status config payload, got %+v", details.Config)
	}
	if len(details.StorageContent) != 2 || details.StorageContent[0].Content != "backup" || details.StorageContent[0].VolID != "local-zfs:a-backup" {
		t.Fatalf("expected sorted storage content, got %+v", details.StorageContent)
	}
}

func TestLoadProxmoxAssetDetailsNodeZFSWarningAndSuccess(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	makeClient := func(t *testing.T, handler http.HandlerFunc) *proxmox.Client {
		t.Helper()
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		client, err := proxmox.NewClient(proxmox.Config{
			BaseURL:     server.URL,
			TokenID:     "id",
			TokenSecret: "secret",
			Timeout:     5 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		return client
	}

	t.Run("zfs warning", func(t *testing.T) {
		client := makeClient(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
			case "/api2/json/nodes/pve01/status":
				_, _ = w.Write([]byte(`{"data":{"status":"online"}}`))
			case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/nodes/pve01/firewall/rules", "/api2/json/cluster/backup":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
			case "/api2/json/nodes/pve01/disks/zfs":
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"errors":"zfs failed"}`))
			default:
				t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
			}
		})

		sut := newTestAPIServer(t)
		details, err := sut.loadProxmoxAssetDetails(
			context.Background(),
			"proxmox-node-pve01",
			proxmoxSessionTarget{Kind: "node", Node: "pve01"},
			proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		)
		if err != nil {
			t.Fatalf("loadProxmoxAssetDetails node warning branch failed: %v", err)
		}
		if !strings.Contains(strings.Join(details.Warnings, " | "), "zfs pools unavailable") {
			t.Fatalf("expected zfs warning, got %+v", details.Warnings)
		}
	})

	t.Run("zfs success and unknown kind warning", func(t *testing.T) {
		client := makeClient(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
			case "/api2/json/nodes/pve01/status":
				_, _ = w.Write([]byte(`{"data":{"status":"online"}}`))
			case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/pve01/firewall/rules":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
			case "/api2/json/nodes/pve01/disks/zfs":
				_, _ = w.Write([]byte(`{"data":[{"name":"pool-a","size":1000,"alloc":100,"free":900,"health":"ONLINE"}]}`))
			default:
				t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
			}
		})

		sut := newTestAPIServer(t)
		details, err := sut.loadProxmoxAssetDetails(
			context.Background(),
			"proxmox-node-pve01",
			proxmoxSessionTarget{Kind: "node", Node: "pve01"},
			proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		)
		if err != nil {
			t.Fatalf("loadProxmoxAssetDetails node success branch failed: %v", err)
		}
		if len(details.ZFSPools) != 1 || details.ZFSPools[0].Name != "pool-a" {
			t.Fatalf("expected zfs pools to be included, got %+v", details.ZFSPools)
		}

		unknownDetails, err := sut.loadProxmoxAssetDetails(
			context.Background(),
			"proxmox-weird",
			proxmoxSessionTarget{Kind: "weird", Node: "pve01"},
			proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		)
		if err != nil {
			t.Fatalf("loadProxmoxAssetDetails unknown kind branch failed: %v", err)
		}
		if !strings.Contains(strings.Join(unknownDetails.Warnings, " | "), "unsupported proxmox kind") {
			t.Fatalf("expected unsupported-kind warning, got %+v", unknownDetails.Warnings)
		}
	})
}

func TestLoadProxmoxAssetDetailsStorageContentErrorAndTieSort(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":300,"avail":700}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"local-zfs:z-vol","content":"images"},{"volid":"local-zfs:a-vol","content":"images"}]}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "local-zfs"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails storage tie-sort failed: %v", err)
	}
	if len(details.StorageContent) != 2 || details.StorageContent[0].VolID != "local-zfs:a-vol" {
		t.Fatalf("expected volid tie sort for same content type, got %+v", details.StorageContent)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":300,"avail":700}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"content failed"}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer errorServer.Close()

	errorClient, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     errorServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	errorDetails, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "local-zfs"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(errorClient, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails storage content warning branch failed: %v", err)
	}
	if !strings.Contains(strings.Join(errorDetails.Warnings, " | "), "storage content unavailable") {
		t.Fatalf("expected storage content warning, got %+v", errorDetails.Warnings)
	}
}
