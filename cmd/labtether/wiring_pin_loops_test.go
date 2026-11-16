package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
)

// TestWiringPinRuntimeLoops pins the names of the background loops
// startRuntimeLoops starts, in normal and in demo mode. It replaces safeGo
// with a recorder, so no loop runs. The Postgres store and failover store
// are empty placeholders: startRuntimeLoops only checks that they are
// present before it starts the loops that need them.
//
// Risk covered: a C2 move that drops, renames or duplicates a loop, or
// changes which mode starts it. It does not see goroutines started without
// safeGo (the agent shutdown sender, the metrics bridge registry, cert
// reloaders and challenge cleanup in runHub, loops started inside handlers),
// and it does not run any loop.
//
// Regenerate (then review the diff line by line):
//
//	LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
func TestWiringPinRuntimeLoops(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Runtime loop pin: names startRuntimeLoops passes to safeGo, sorted.\n")
	b.WriteString("# Regenerate: " + wiringPinUpdateCommand + "\n")
	for _, demo := range []bool{false, true} {
		names := recordRuntimeLoops(t, demo)
		mode := "normal mode"
		if demo {
			mode = "demo mode"
		}
		fmt.Fprintf(&b, "\n== %s\ncount %d\n", mode, len(names))
		for _, name := range names {
			b.WriteString(name + "\n")
		}
	}
	compareWiringPin(t, "runtime_loops.txt", []byte(b.String()))
}

func recordRuntimeLoops(t *testing.T, demo bool) []string {
	t.Helper()
	sut := newTestAPIServer(t)
	sut.demoMode = demo
	pgStore := &persistence.PostgresStore{}
	sut.failoverStore = pgStore
	// The agent shutdown sender (a plain goroutine) runs when ctx ends.
	sut.agentMgr = agentmgr.NewManager()

	var mu sync.Mutex
	var names []string
	original := safeGo
	safeGo = func(_ context.Context, _ *sync.WaitGroup, name string, _ func(context.Context)) {
		mu.Lock()
		defer mu.Unlock()
		names = append(names, name)
	}
	defer func() { safeGo = original }()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	startRuntimeLoops(ctx, sut, pgStore, nil, nil)

	mu.Lock()
	defer mu.Unlock()
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("loop %q started twice", name)
		}
		seen[name] = true
	}
	sort.Strings(names)
	return names
}
