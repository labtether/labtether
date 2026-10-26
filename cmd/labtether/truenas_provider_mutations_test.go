package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
)

func (f *trueNASMutationFixture) setResponse(response func(string, []any) (any, *trueNASRPCError, bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.response = response
}

func trueNASWrites(calls []trueNASMutationCall) []trueNASMutationCall {
	var writes []trueNASMutationCall
	for _, call := range calls {
		if !strings.HasSuffix(call.method, ".query") {
			writes = append(writes, call)
		}
	}
	return writes
}

func TestTrueNASShareMutationResolvesNumericOwner(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	for _, tc := range []struct{ kind, id, group, method, body string }{
		{"smb", "7", "share-smb", "DELETE", ""},
		{"nfs", "8", "share-nfs", "PUT", `{"enabled":false}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			f.windows.blocked = tc.group
			rec := f.request(tc.method, "shares/"+tc.kind+"/"+tc.id, tc.body, false)
			calls := f.takeCalls()
			if rec.Code != http.StatusLocked || len(calls) != 1 || len(trueNASWrites(calls)) != 0 {
				t.Fatalf("share lock bypass: %d %+v %s", rec.Code, calls, rec.Body)
			}
			wantID := float64(7)
			if tc.kind == "nfs" {
				wantID = 8
			}
			wantQuery := []any{[]any{[]any{"id", "=", wantID}}}
			if !reflect.DeepEqual(calls[0].params[:1], wantQuery) {
				t.Fatalf("lookup did not use numeric identity: %+v", calls)
			}
			f.windows.blocked = ""
			rec = f.request(tc.method, "shares/"+tc.kind+"/"+tc.id, tc.body, false)
			writes := trueNASWrites(f.takeCalls())
			if rec.Code != http.StatusOK || len(writes) != 1 || writes[0].params[0] != wantID {
				t.Fatalf("wrong numeric write: %d %+v %s", rec.Code, writes, rec.Body)
			}
		})
	}
	for _, id := range []string{"0", "-1", "x", "7/extra"} {
		rec := f.request("DELETE", "shares/smb/"+id, "", false)
		if calls := f.takeCalls(); rec.Code != http.StatusBadRequest || len(calls) != 0 {
			t.Fatalf("invalid ID dispatched: %s %d %+v", id, rec.Code, calls)
		}
	}
	_, err := f.s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetid.ScopeCollectorAssetID(tnconnector.ResourceAssetID("share-smb", "destination"), "nas-a"),
		Name:    "destination", Source: "truenas", Type: "share-smb", GroupID: "share-destination",
		Metadata: map[string]string{"collector_id": "nas-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.windows.blocked = "share-destination"
	for _, path := range []string{"shares/smb/7", "shares/smb"} {
		method := "PUT"
		if path == "shares/smb" {
			method = "POST"
		}
		rec := f.request(method, path, `{"name":"destination","path":"/mnt/tank/data"}`, false)
		if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusLocked || len(writes) != 0 {
			t.Fatalf("share destination bypass: %d %+v %s", rec.Code, writes, rec.Body)
		}
	}
	f.windows.blocked = ""
	f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
		return []map[string]any{{"id": 7, "name": "renamed-on-provider", "path": "/mnt/tank/data"}}, nil, method == "sharing.smb.query"
	})
	f.windows.blocked = "share-smb"
	rec := f.request("DELETE", "shares/smb/7", "", false)
	if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusLocked || len(writes) != 0 {
		t.Fatalf("provider rename lost original group: %d %+v %s", rec.Code, writes, rec.Body)
	}
	f.windows.blocked = ""
	for _, rows := range []any{[]any{}, []map[string]any{{"id": 8, "name": "wrong"}}, []map[string]any{{"id": 7}, {"id": 7}}, []map[string]any{{"id": "7"}}} {
		f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
			return rows, nil, method == "sharing.smb.query"
		})
		rec := f.request("DELETE", "shares/smb/7", "", false)
		if writes := trueNASWrites(f.takeCalls()); rec.Code < 400 || len(writes) != 0 {
			t.Fatalf("unverified share dispatched: %d %+v", rec.Code, writes)
		}
	}
}

func TestTrueNASMutationErrorDoesNotRepeatSideEffect(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	for _, tc := range []struct{ path, method string }{{"services/nfs/restart", "service.restart"}, {"vms/42/restart", "vm.restart"}} {
		f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
			// A restart may succeed before the provider returns an internal error.
			return nil, &trueNASRPCError{Code: -32001, Message: "post-write failure with synthetic-private-provider-text"}, method == tc.method
		})
		rec := f.request("POST", tc.path, "", false)
		if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusBadGateway || len(writes) != 1 ||
			!strings.Contains(rec.Body.String(), "not retried") || !strings.Contains(rec.Body.String(), "outcome may be unknown") ||
			strings.Contains(rec.Body.String(), "synthetic-private-provider-text") {
			t.Fatalf("mutation failure repeated/hidden: %d %+v %s", rec.Code, writes, rec.Body)
		}
	}
	// Wrapping a mutation error must preserve the CORE unsupported-method path.
	f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
		return nil, &trueNASRPCError{Code: -32601, Message: "method not found"}, method == "vm.restart"
	})
	rec := f.request("POST", "vms/42/restart", "", false)
	if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusNotImplemented || len(writes) != 1 {
		t.Fatalf("CORE compatibility lost: %d %+v %s", rec.Code, writes, rec.Body)
	}
}

func TestTrueNASManualScrubKeepsAcceptedJobIdentity(t *testing.T) {
	f := newTrueNASMutationFixture(t)
	rec := f.request("POST", "pools/tank/scrub", "", false)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	writes := trueNASWrites(f.takeCalls())
	if rec.Code != http.StatusOK || body["status"] != "accepted" || body["job_id"] != float64(777) || len(writes) != 1 ||
		writes[0].method != "pool.scrub.scrub" || !reflect.DeepEqual(writes[0].params, []any{"tank", "START"}) {
		t.Fatalf("manual scrub contract: %d %+v %+v", rec.Code, writes, body)
	}
	invokeConnector := func() connectorsdk.ActionResult {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/connectors/truenas/actions/pool.scrub/execute", strings.NewReader(`{"params":{"pool_name":"tank","collector_id":"nas-a"}}`))
		response := httptest.NewRecorder()
		f.s.ensureCollectorsDeps().HandleConnectorActions(response, request)
		var envelope struct {
			Result connectorsdk.ActionResult `json:"result"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Result
	}
	result := invokeConnector()
	writes = trueNASWrites(f.takeCalls())
	if result.Status != "succeeded" || result.Metadata["provider_job_id"] != "777" || result.Metadata["submission_status"] != "accepted" ||
		len(writes) != 1 || !reflect.DeepEqual(writes[0].params, []any{"tank", "START"}) || strings.Contains(result.Message, "started") {
		t.Fatalf("connector scrub acceptance: %+v %+v", result, writes)
	}
	for _, value := range []any{nil, 0, -1, "777", map[string]any{"ok": true}} {
		f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
			return value, nil, method == "pool.scrub.scrub"
		})
		rec = f.request("POST", "pools/tank/scrub", "", false)
		if writes := trueNASWrites(f.takeCalls()); rec.Code != http.StatusBadGateway || len(writes) != 1 {
			t.Fatalf("invalid job acceptance: %d %+v", rec.Code, writes)
		}
	}
	f.setResponse(func(method string, _ []any) (any, *trueNASRPCError, bool) {
		return nil, &trueNASRPCError{Code: -32001, Message: "job enqueue failed"}, method == "pool.scrub.scrub"
	})
	result = invokeConnector()
	if writes := trueNASWrites(f.takeCalls()); result.Status != "failed" || len(writes) != 1 {
		t.Fatalf("scrub fallback/retry: %+v %+v", result, writes)
	}
}
