package main

import (
	"fmt"
	"github.com/labtether/labtether/internal/agentmgr"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDesktopBridgeStartRecordingLockedSerializesConcurrentStarts(t *testing.T) {
	bridge := &desktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	const workers = 20
	var (
		startCalls atomic.Int64
		wg         sync.WaitGroup
	)
	ids := make(chan string, workers)
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec, _, err := bridge.StartRecordingLocked(func() (*activeRecording, error) {
				startCalls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return &activeRecording{ID: fmt.Sprintf("rec-%02d", i)}, nil
			})
			if err != nil {
				errs <- err
				return
			}
			if rec == nil {
				errs <- fmt.Errorf("nil recording returned")
				return
			}
			ids <- rec.ID
		}()
	}

	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected start error: %v", err)
		}
	}

	if got := startCalls.Load(); got != 1 {
		t.Fatalf("expected one recording initialization, got %d", got)
	}

	firstID := ""
	for id := range ids {
		if firstID == "" {
			firstID = id
			continue
		}
		if id != firstID {
			t.Fatalf("expected shared recording id %q, got %q", firstID, id)
		}
	}
}

func TestDesktopBridgeStopRecordingLockedClearsOnlyOnce(t *testing.T) {
	bridge := &desktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	bridge.SetRecording(&activeRecording{ID: "rec-1"})

	var stopCalls atomic.Int64
	firstStop := bridge.StopRecordingLocked(func(rec *activeRecording) {
		if rec == nil || rec.ID != "rec-1" {
			t.Fatalf("unexpected recording passed to stop: %+v", rec)
		}
		stopCalls.Add(1)
	})
	if !firstStop {
		t.Fatal("expected first stop to succeed")
	}

	secondStop := bridge.StopRecordingLocked(func(*activeRecording) {
		stopCalls.Add(1)
	})
	if secondStop {
		t.Fatal("expected second stop to report no active recording")
	}

	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("expected one stop callback, got %d", got)
	}
}

func TestFinalizeAgentDesktopSessionSendsCloseAfterStart(t *testing.T) {
	var srv apiServer
	bridge := &desktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}
	bridge.SetRecording(&activeRecording{ID: "rec-finalize"})
	srv.desktopBridges.Store("sess-finalize", bridge)

	var (
		closeCalls int
		closedSess string
	)
	srv.finalizeAgentDesktopSession(
		"sess-finalize",
		bridge,
		nil,
		true,
		func(_ *agentmgr.AgentConn, sessionID string) {
			closeCalls++
			closedSess = sessionID
		},
	)

	if closeCalls != 1 {
		t.Fatalf("expected one desktop.close send, got %d", closeCalls)
	}
	if closedSess != "sess-finalize" {
		t.Fatalf("unexpected closed session id %q", closedSess)
	}
	if _, ok := srv.desktopBridges.Load("sess-finalize"); ok {
		t.Fatal("expected desktop bridge to be removed")
	}
	if bridge.CurrentRecording() != nil {
		t.Fatal("expected recording to be cleared during finalize")
	}
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("expected bridge closed channel to be closed")
	}
}

func TestFinalizeAgentDesktopSessionSkipsCloseBeforeStart(t *testing.T) {
	var srv apiServer
	bridge := &desktopBridge{
		OutputCh: make(chan []byte, 1),
		ClosedCh: make(chan struct{}),
	}

	var closeCalls int
	srv.finalizeAgentDesktopSession(
		"sess-no-start",
		bridge,
		nil,
		false,
		func(_ *agentmgr.AgentConn, _ string) {
			closeCalls++
		},
	)

	if closeCalls != 0 {
		t.Fatalf("expected no desktop.close send before start, got %d", closeCalls)
	}
}

func TestCloseDesktopBridgesForAssetClosesMatchingSessionsOnly(t *testing.T) {
	var srv apiServer
	matching := &desktopBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-1",
	}
	nonMatching := &desktopBridge{
		OutputCh:        make(chan []byte, 1),
		ClosedCh:        make(chan struct{}),
		ExpectedAgentID: "node-2",
	}

	srv.desktopBridges.Store("sess-node-1", matching)
	srv.desktopBridges.Store("sess-node-2", nonMatching)
	srv.desktopBridges.Store("non-bridge-entry", "ignore-me")

	srv.closeDesktopBridgesForAsset("node-1")

	select {
	case <-matching.ClosedCh:
	default:
		t.Fatal("expected matching desktop bridge to be closed")
	}

	select {
	case <-nonMatching.ClosedCh:
		t.Fatal("expected non-matching desktop bridge to remain open")
	default:
	}
}
