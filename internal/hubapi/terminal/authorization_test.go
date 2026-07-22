package terminal

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
	terminalcfg "github.com/labtether/labtether/internal/terminal"
)

func TestTargetUsesStoredCredentialIncludesGroupJumpChain(t *testing.T) {
	groupStore := persistence.NewMemoryGroupStore()
	raw, err := json.Marshal(terminalcfg.JumpChain{Hops: []terminalcfg.HopConfig{{
		Host: "jump.internal", Port: 22, Username: "root", CredentialProfileID: "cred-jump",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	group, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Secured", JumpChain: raw})
	if err != nil {
		t.Fatal(err)
	}
	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-1", Name: "Target", Type: "host", Source: "agent", GroupID: group.ID,
	}); err != nil {
		t.Fatal(err)
	}
	deps := &Deps{AssetStore: assetStore, GroupStore: groupStore}
	uses, err := deps.targetUsesStoredCredential(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("targetUsesStoredCredential: %v", err)
	}
	if !uses {
		t.Fatal("expected group jump-chain credential to require credentials:use")
	}
}

func TestResolveJumpChainHopsRejectsOversizedLegacyChainBeforeLookup(t *testing.T) {
	deps := &Deps{}
	chain := terminalcfg.JumpChain{Hops: make([]terminalcfg.HopConfig, terminalcfg.MaxJumpChainHops+1)}
	if _, err := deps.ResolveJumpChainHops(chain); err == nil {
		t.Fatal("expected oversized legacy chain to fail before credential resolution")
	}
}
