package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/protocols"
)

func TestProtocolHealthSSHHostKeyCallbackFailsClosedByDefault(t *testing.T) {
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_KNOWN_HOSTS_PATH", "")
	t.Setenv("SSH_KNOWN_HOSTS_PATHS", "")
	if callback, err := buildProtocolHealthSSHHostKeyCallback(protocols.SSHConfig{StrictHostKey: false}); err == nil || callback != nil {
		t.Fatalf("expected missing host identity material to fail closed, callback=%v err=%v", callback, err)
	}
}

func TestProtocolHealthSSHHostKeyCallbackAllowsAcknowledgedInsecureMode(t *testing.T) {
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "true")
	if callback, err := buildProtocolHealthSSHHostKeyCallback(protocols.SSHConfig{StrictHostKey: false}); err != nil || callback == nil {
		t.Fatalf("expected acknowledged insecure callback, callback=%v err=%v", callback, err)
	}
}

func TestRunProtocolConfigHealthChecksStopsWhenContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32
	runProtocolConfigHealthChecks(ctx, []*protocols.ProtocolConfig{
		{AssetID: "asset-1", Protocol: protocols.ProtocolSSH},
		{AssetID: "asset-2", Protocol: protocols.ProtocolVNC},
	}, 1, func(context.Context, *protocols.ProtocolConfig) {
		calls.Add(1)
	})

	if calls.Load() != 0 {
		t.Fatalf("test function called %d times after cancellation, want 0", calls.Load())
	}
}

func TestRunProtocolConfigHealthChecksStopsSchedulingAfterMidRunCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	done := make(chan struct{})
	var calls atomic.Int32

	go func() {
		runProtocolConfigHealthChecks(ctx, []*protocols.ProtocolConfig{
			{AssetID: "asset-1", Protocol: protocols.ProtocolSSH},
			{AssetID: "asset-2", Protocol: protocols.ProtocolVNC},
		}, 1, func(context.Context, *protocols.ProtocolConfig) {
			if calls.Add(1) == 1 {
				close(firstStarted)
				<-releaseFirst
			}
		})
		close(done)
	}()

	<-firstStarted
	cancel()
	close(releaseFirst)
	<-done

	if calls.Load() != 1 {
		t.Fatalf("test function called %d times, want only the in-flight check", calls.Load())
	}
}
