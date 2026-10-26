package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/persistence"
)

func seedTrueNASDatasetOwner(t *testing.T, f *trueNASMutationFixture, name, group, collector string) string {
	t.Helper()
	id := assetid.ScopeCollectorAssetID(tnconnector.ResourceAssetID("dataset", name), collector)
	_, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Name: name, Source: "truenas", Type: "dataset", GroupID: group,
		Metadata: map[string]string{"collector_id": collector, "dataset_name": name}})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTrueNASReplicationGuardsOnlyProvenLocalDatasets(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	seedTrueNASDatasetOwner(t, f, "tank/data/child", "source-child", "nas-a")
	seedTrueNASDatasetOwner(t, f, "tank/data/skip", "source-excluded", "nas-a")
	seedTrueNASDatasetOwner(t, f, "tank/database", "boundary", "nas-a")
	seedTrueNASDatasetOwner(t, f, "dest/parent/child", "target-child", "nas-a")
	seedTrueNASDatasetOwner(t, f, "dest/parent/skip", "target-excluded", "nas-a")
	seedTrueNASDatasetOwner(t, f, "tank/data/foreign", "foreign", "nas-b")
	for _, tc := range []struct {
		name, direction, transport, group string
		want                              int
	}{
		{"push source", "PUSH", "SSH", "source-child", http.StatusLocked},
		{"push remote same name", "PUSH", "SSH", "target-child", http.StatusOK},
		{"pull target", "PULL", "SSH", "target-child", http.StatusLocked},
		{"pull remote source", "PULL", "SSH", "source-child", http.StatusOK},
		{"local source", "PUSH", "LOCAL", "source-child", http.StatusLocked},
		{"local target", "PUSH", "LOCAL", "target-child", http.StatusLocked},
		{"source excluded", "PUSH", "LOCAL", "source-excluded", http.StatusOK},
		{"target exclusion translated", "PUSH", "LOCAL", "target-excluded", http.StatusOK},
		{"path boundary", "PUSH", "LOCAL", "boundary", http.StatusOK},
		{"foreign collector", "PUSH", "LOCAL", "foreign", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.setResponse(func(method string, params []any) (any, *trueNASRPCError, bool) {
				if method != "replication.query" {
					return nil, nil, false
				}
				if len(params) != 2 || params[0].([]any)[0].([]any)[2] != float64(9) {
					t.Errorf("replication query is not exact numeric ID: %+v", params)
				}
				return []map[string]any{{"id": 9, "direction": tc.direction, "transport": tc.transport,
					"source_datasets": []string{"tank/data"}, "target_dataset": "dest/parent", "recursive": true,
					"exclude": []string{"tank/data/skip"}}}, nil, true
			})
			f.windows.blocked = tc.group
			rec := f.request("POST", "replication/9/run", "", false)
			writes := trueNASWrites(f.takeCalls())
			if rec.Code != tc.want || (tc.want == http.StatusLocked && len(writes) != 0) ||
				(tc.want == http.StatusOK && (len(writes) != 1 || writes[0].method != "replication.run" || writes[0].params[0] != float64(9))) {
				t.Fatalf("status=%d writes=%+v body=%s", rec.Code, writes, rec.Body)
			}
		})
	}
}

func TestTrueNASReplicationPagesAllStoredOwners(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	for i := 0; i < 300; i++ {
		seedTrueNASDatasetOwner(t, f, fmt.Sprintf("tank/data/child-%03d", i), fmt.Sprintf("child-%03d", i), "nas-a")
	}
	// Real discovery metadata keeps identity even when the operator renames it.
	id := assetid.ScopeCollectorAssetID(tnconnector.ResourceAssetID("dataset", "tank/data/child-299"), "nas-a")
	displayName := "Pretty label"
	if _, err := f.s.assetStore.UpdateAsset(id, assets.UpdateRequest{Name: &displayName}); err != nil {
		t.Fatal(err)
	}
	exclude := []string{}
	f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
		return []map[string]any{{"id": 9, "direction": "PUSH", "transport": "SSH", "source_datasets": []string{"tank/data"},
			"target_dataset": "remote/data", "recursive": true, "exclude": exclude}}, nil, method == "replication.query"
	})
	f.windows.blocked = "child-299"
	rec := f.request("POST", "replication/9/run", "", false)
	if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusLocked || len(writes) != 0 {
		t.Fatalf("later page owner bypassed: %d %+v %s", rec.Code, writes, rec.Body)
	}
	exclude = []string{"tank/data/child-299"}
	rec = f.request("POST", "replication/9/run", "", false)
	if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusOK || len(writes) != 1 {
		t.Fatalf("large valid inventory rejected: %d %+v %s", rec.Code, writes, rec.Body)
	}
}

func TestTrueNASReplicationMapsMultipleSourceTargets(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	seedTrueNASDatasetOwner(t, f, "dest/parent/one/child", "mapped-child", "nas-a")
	seedTrueNASDatasetOwner(t, f, "dest/parent/one/skip", "mapped-excluded", "nas-a")
	seedTrueNASDatasetOwner(t, f, "dest/parent/unrelated", "unrelated", "nas-a")
	f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
		return []map[string]any{{"id": 9, "direction": "PULL", "transport": "SSH",
			"source_datasets": []string{"tank/data/one", "tank/data/two"}, "target_dataset": "dest/parent",
			"recursive": true, "exclude": []string{"tank/data/one/skip"}}}, nil, method == "replication.query"
	})
	for _, tc := range []struct {
		group string
		want  int
	}{{"mapped-child", http.StatusLocked}, {"mapped-excluded", http.StatusOK}, {"unrelated", http.StatusOK}} {
		t.Run(tc.group, func(t *testing.T) {
			f.windows.blocked = tc.group
			rec := f.request("POST", "replication/9/run", "", false)
			writes := trueNASWrites(f.takeCalls())
			if rec.Code != tc.want || (tc.want == http.StatusLocked && len(writes) != 0) ||
				(tc.want == http.StatusOK && len(writes) != 1) {
				t.Fatalf("provider common-base mapping: %d %+v %s", rec.Code, writes, rec.Body)
			}
		})
	}
}

type failingTrueNASDatasetPages struct{ persistence.AssetStore }

func (s failingTrueNASDatasetPages) FindTrueNASActionAssets(collector, kind, id string) ([]assets.Asset, error) {
	return s.AssetStore.(persistence.TrueNASActionAssetStore).FindTrueNASActionAssets(collector, kind, id)
}
func (s failingTrueNASDatasetPages) ListTrueNASDatasetAssets(context.Context, string, string, string, int) ([]assets.Asset, error) {
	return nil, context.DeadlineExceeded
}

func TestTrueNASReplicationRefusesUnverifiedOwners(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{"id": 9, "direction": "PUSH", "transport": "SSH", "source_datasets": []string{"tank/data"},
			"target_dataset": "remote/data", "recursive": true, "exclude": []string{}}
	}
	for _, kind := range []string{"missing", "wrong ID", "unknown transport", "missing recursive", "invalid path", "query error", "page error", "ambiguous legacy"} {
		t.Run(kind, func(t *testing.T) {
			f := newTrueNASMutationFixture(t)
			row := valid()
			rows := []map[string]any{row}
			switch kind {
			case "missing":
				rows = nil
			case "wrong ID":
				row["id"] = 10
			case "unknown transport":
				row["transport"] = "unknown"
			case "missing recursive":
				delete(row, "recursive")
			case "invalid path":
				row["source_datasets"] = []string{"tank/data/../else"}
			case "page error":
				f.s.assetStore = failingTrueNASDatasetPages{f.s.assetStore}
			case "ambiguous legacy":
				_, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "truenas-dataset-tank--data--legacy", Name: "tank/data/legacy", Source: "truenas", Type: "dataset"})
				if err != nil {
					t.Fatal(err)
				}
			}
			f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
				if kind == "query error" {
					return nil, &trueNASRPCError{Code: -32001, Message: "unavailable"}, method == "replication.query"
				}
				return rows, nil, method == "replication.query"
			})
			rec := f.request("POST", "replication/9/run", "", false)
			if writes := trueNASWrites(f.takeCalls()); rec.Code < 400 || len(writes) != 0 || strings.Contains(rec.Body.String(), "started") {
				t.Fatalf("unverified task dispatched: %d %+v %s", rec.Code, writes, rec.Body)
			}
		})
	}
}
