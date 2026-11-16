package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

// Fault: a transfer from an explicit agent that never advertised files is
// created and its file.read sent, then it fails later instead of at once.
func TestFileTransferFromExplicitAgentIsRefusedBeforeCreate(t *testing.T) {
	peer := testutil.NewAgentPeer(t, unsupportedDevice)
	agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit})
	pool := fileproto.NewPool()
	defer pool.Close()
	transferStore := newTestFileTransferStore()
	deps := &Deps{
		AgentMgr:            peer.Manager,
		FileBridges:         &sync.Map{},
		FileProtoPool:       pool,
		FileConnectionStore: newTestTransferFileConnectionStore(&persistence.FileConnection{ID: "dest-a", ActorID: "actor-a"}),
		FileTransferStore:   transferStore,
		PrincipalActorID:    apiv2.PrincipalActorID,
		ActiveTransfers:     &sync.Map{},
		DecodeJSONBody:      decodeFileTransferTestJSONBody,
	}
	body := strings.NewReader(`{"source_type":"agent","source_id":"node01","source_path":"/etc/hosts",
		"dest_type":"connection","dest_id":"dest-a","dest_path":"/hosts"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-transfers", body)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
	rec := httptest.NewRecorder()
	deps.HandleFileTransfers(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || transferStore.Count() != 0 {
		t.Fatalf("got %d %s with %d transfers, want 422 and none", rec.Code, rec.Body.String(), transferStore.Count())
	}
	peer.Silent(t, 150*time.Millisecond)
}

// Fault: Wake-on-LAN picks an explicit agent that cannot send magic packets
// as the relay, so the wake is lost instead of going through another agent.
func TestWoLRelaysSkipExplicitAgentsWithoutWoL(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	for id, metadata := range map[string]map[string]string{
		"relay-go":   {"platform": "linux"},
		"relay-rust": {"platform": "linux", agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_network": "list"},
		"relay-wol":  {"platform": "linux", agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_wol": "send"},
	} {
		if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "host", Name: id, Source: "agent", Platform: "linux", Metadata: metadata}); err != nil {
			t.Fatal(err)
		}
	}
	peers := []*testutil.AgentPeer{testutil.NewAgentPeer(t, "relay-go")}
	manager := peers[0].Manager
	for _, id := range []string{"relay-rust", "relay-wol"} {
		peer := testutil.NewAgentPeer(t, id)
		manager.Register(peer.Conn)
	}
	d := &Deps{AgentMgr: manager, AssetStore: store}
	var got []string
	for _, relay := range d.EligibleWoLRelays("target01", assets.Asset{ID: "target01"}) {
		got = append(got, relay.AssetID)
	}
	if strings.Join(got, ",") != "relay-go,relay-wol" {
		t.Fatalf("relays %v, want relay-go and relay-wol", got)
	}
}
