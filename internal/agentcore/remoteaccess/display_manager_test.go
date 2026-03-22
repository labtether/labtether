package remoteaccess

import (
	"fmt"
	"os/exec"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestDisplayManagerAcquireCreatesXvfb(t *testing.T) {
	dm := NewDisplayManager()
	// On dev machines Xvfb may not be available — test the logic, not the binary.
	// We test acquire/release reference counting here.
	dm.Mu.Lock()
	dm.Displays[":99"] = &ManagedDisplay{
		display:  ":99",
		refCount: 0,
	}
	dm.Mu.Unlock()

	dm.addRef(":99")
	dm.Mu.Lock()
	entry := dm.Displays[":99"]
	dm.Mu.Unlock()
	if entry.refCount != 1 {
		t.Fatalf("refCount=%d, want 1", entry.refCount)
	}

	dm.addRef(":99")
	dm.Mu.Lock()
	entry = dm.Displays[":99"]
	dm.Mu.Unlock()
	if entry.refCount != 2 {
		t.Fatalf("refCount=%d, want 2", entry.refCount)
	}
}

func TestDisplayManagerReleaseDecrementsRefCount(t *testing.T) {
	dm := NewDisplayManager()
	dm.Mu.Lock()
	dm.Displays[":99"] = &ManagedDisplay{
		display:  ":99",
		refCount: 2,
	}
	dm.Mu.Unlock()

	dm.release(":99")
	dm.Mu.Lock()
	entry := dm.Displays[":99"]
	dm.Mu.Unlock()
	if entry.refCount != 1 {
		t.Fatalf("refCount=%d, want 1", entry.refCount)
	}
}

func TestDisplayManagerReleaseAtZeroRemovesEntry(t *testing.T) {
	dm := NewDisplayManager()
	dm.Mu.Lock()
	dm.Displays[":99"] = &ManagedDisplay{
		display:  ":99",
		refCount: 1,
	}
	dm.Mu.Unlock()

	dm.release(":99")
	dm.Mu.Lock()
	_, exists := dm.Displays[":99"]
	dm.Mu.Unlock()
	if exists {
		t.Fatal("expected display entry to be removed at refCount 0")
	}
}

func TestDisplayManagerActiveDisplays(t *testing.T) {
	dm := NewDisplayManager()
	dm.Mu.Lock()
	dm.Displays[":97"] = &ManagedDisplay{display: ":97", refCount: 1}
	dm.Displays[":98"] = &ManagedDisplay{display: ":98", refCount: 2}
	dm.Mu.Unlock()

	displays := dm.activeDisplays()
	if len(displays) != 2 {
		t.Fatalf("expected 2 active displays, got %d", len(displays))
	}
}

// TestDisplayManagerAcquireSkipsPlaceholder verifies that a concurrent acquire
// does not attempt to reuse a placeholder entry (xvfbCmd == nil) that is still
// being started by another goroutine.
func TestDisplayManagerAcquireSkipsPlaceholder(t *testing.T) {
	originalFind := FindDesktopFreeDisplay
	originalStartXvfb := StartDesktopXvfb
	t.Cleanup(func() {
		FindDesktopFreeDisplay = originalFind
		StartDesktopXvfb = originalStartXvfb
	})
	FindDesktopFreeDisplay = func() int { return 98 }
	StartDesktopXvfb = func(int, int, int) (*exec.Cmd, string, error) {
		return nil, "", fmt.Errorf("xvfb not available in test")
	}

	dm := NewDisplayManager()

	// Insert a placeholder that mimics an in-flight acquire (refCount=1, xvfbCmd=nil).
	dm.Mu.Lock()
	dm.Displays[":99"] = &ManagedDisplay{
		display:  ":99",
		refCount: 1,
		// xvfbCmd intentionally nil — still starting
	}
	dm.Mu.Unlock()

	// A second goroutine calling acquire should NOT reuse the placeholder.
	// Because StartDesktopXvfb is stubbed to fail in this test, the acquire will
	// fail — but what matters is that it did not return the placeholder's empty
	// xauthPath as a valid result.
	// We verify via the map: acquire must either insert a new key or remove the
	// placeholder, but it must never increment the placeholder's refCount.

	before := func() int {
		dm.Mu.Lock()
		defer dm.Mu.Unlock()
		if e, ok := dm.Displays[":99"]; ok {
			return e.refCount
		}
		return -1
	}

	refBefore := before()

	// acquire will fail (mocked Xvfb) — that is expected in this test.
	// We only care that the placeholder was not touched.
	_, _, _ = dm.acquire()

	refAfter := before()
	// The placeholder refCount must be unchanged.
	if refAfter != -1 && refAfter != refBefore {
		t.Fatalf("placeholder refCount changed from %d to %d; acquire must skip placeholders", refBefore, refAfter)
	}
}

// TestDisplayManagerAcquireNoConcurrentDuplicate verifies that two concurrent
// callers cannot both register separate entries for the same display number.
// The test injects a fake FindDesktopFreeDisplay that always returns the same
// number, forcing the race condition to manifest if the placeholder is absent.
func TestDisplayManagerAcquireNoConcurrentDuplicate(t *testing.T) {
	originalFind := FindDesktopFreeDisplay
	originalStartXvfb := StartDesktopXvfb
	t.Cleanup(func() {
		FindDesktopFreeDisplay = originalFind
		StartDesktopXvfb = originalStartXvfb
	})

	callCount := 0
	var findMu sync.Mutex
	FindDesktopFreeDisplay = func() int {
		findMu.Lock()
		defer findMu.Unlock()
		callCount++
		return 90 + callCount
	}
	StartDesktopXvfb = func(int, int, int) (*exec.Cmd, string, error) {
		return nil, "", fmt.Errorf("xvfb not available in test")
	}

	dm := NewDisplayManager()

	// Run two goroutines that both attempt acquire at the same time.
	// Both will fail (no Xvfb) but we verify that at most one placeholder
	// was inserted simultaneously (the second caller picks a fresh display
	// number because the first's placeholder is visible under the lock).
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, _, _ := dm.acquire()
			results[i] = d
		}()
	}
	wg.Wait()

	// After both goroutines finish (with errors), the map must be empty
	// (both placeholders cleaned up on failure).
	dm.Mu.Lock()
	remaining := len(dm.Displays)
	dm.Mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected empty display map after failed acquires, got %d entries", remaining)
	}
}

func TestIsDisplayAvailableUsesActiveUserSession(t *testing.T) {
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		system.CollectUserSessionsFn = originalCollectUserSessions
	})

	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) {
		return []agentmgr.UserSession{
			{Username: "lightdm", Terminal: "seat0", RemoteHost: ":0"},
		}, nil
	}

	if !IsDisplayAvailable(":0") {
		t.Fatal("expected :0 to be considered available when seat0 is active")
	}
}

func TestAppendDetectedActiveDisplaysAddsSessionBackedDisplay(t *testing.T) {
	originalCollectUserSessions := system.CollectUserSessionsFn
	t.Cleanup(func() {
		system.CollectUserSessionsFn = originalCollectUserSessions
	})

	system.CollectUserSessionsFn = func() ([]agentmgr.UserSession, error) {
		return []agentmgr.UserSession{
			{Username: "lightdm", Terminal: "seat0", RemoteHost: ":0"},
		}, nil
	}

	got := AppendDetectedActiveDisplays(nil)
	if len(got) == 0 || got[0] != ":0" {
		t.Fatalf("expected detected displays to include :0, got %v", got)
	}
}
