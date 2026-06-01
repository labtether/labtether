package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/protocols"
)

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
