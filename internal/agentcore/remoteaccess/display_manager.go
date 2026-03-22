package remoteaccess

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/labtether/labtether/internal/agentcore/system"
)

// ManagedDisplay tracks a shared Xvfb display with reference counting.
type ManagedDisplay struct {
	display   string
	xvfbCmd   *exec.Cmd
	xauthPath string
	refCount  int
}

// DisplayManager manages shared Xvfb displays for VNC and WebRTC sessions.
type DisplayManager struct {
	Mu       sync.Mutex
	Displays map[string]*ManagedDisplay
}

func NewDisplayManager() *DisplayManager {
	return &DisplayManager{
		Displays: make(map[string]*ManagedDisplay),
	}
}

// acquire gets or creates an Xvfb display. Returns the display string and xauth path.
// The caller must call release() when done.
func (dm *DisplayManager) acquire() (display, xauthPath string, err error) {
	dm.Mu.Lock()
	// Reuse an existing display if available. Skip placeholders (xvfbCmd == nil)
	// that are still being started by another goroutine.
	for _, entry := range dm.Displays {
		if entry.refCount > 0 && entry.xvfbCmd != nil {
			entry.refCount++
			dm.Mu.Unlock()
			return entry.display, entry.xauthPath, nil
		}
	}

	// Reserve a display number under the lock to prevent concurrent duplicates.
	displayNum := FindDesktopFreeDisplay()
	displayStr := fmt.Sprintf(":%d", displayNum)
	placeholder := &ManagedDisplay{
		display:  displayStr,
		refCount: 1,
	}
	dm.Displays[displayStr] = placeholder
	dm.Mu.Unlock()

	// Start Xvfb outside the lock (slow operation).
	xvfbCmd, xauth, xvfbErr := StartDesktopXvfb(displayNum, 1920, 1080)
	if xvfbErr != nil {
		dm.Mu.Lock()
		delete(dm.Displays, displayStr)
		dm.Mu.Unlock()
		return "", "", fmt.Errorf("failed to start Xvfb: %w", xvfbErr)
	}

	dm.Mu.Lock()
	placeholder.xvfbCmd = xvfbCmd
	placeholder.xauthPath = xauth
	dm.Mu.Unlock()

	log.Printf("desktop: started shared Xvfb on %s (xauth=%s)", displayStr, xauth)
	return displayStr, xauth, nil
}

// addRef increments the reference count for an existing display.
func (dm *DisplayManager) addRef(display string) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	if entry, ok := dm.Displays[display]; ok {
		entry.refCount++
	}
}

// release decrements the reference count and kills Xvfb when it hits zero.
func (dm *DisplayManager) release(display string) {
	dm.Mu.Lock()
	entry, ok := dm.Displays[display]
	if !ok {
		dm.Mu.Unlock()
		return
	}
	entry.refCount--
	if entry.refCount > 0 {
		dm.Mu.Unlock()
		return
	}
	delete(dm.Displays, display)
	dm.Mu.Unlock()

	log.Printf("desktop: last consumer released %s, stopping Xvfb", display)
	TerminateProcess(entry.xvfbCmd)
	if entry.xauthPath != "" {
		if err := os.Remove(entry.xauthPath); err != nil && !os.IsNotExist(err) {
			log.Printf("desktop: remove xauth %s: %v", entry.xauthPath, err)
		}
	}
}

// activeDisplays returns a list of currently managed displays.
func (dm *DisplayManager) activeDisplays() []string {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	displays := make([]string, 0, len(dm.Displays))
	for d := range dm.Displays {
		displays = append(displays, d)
	}
	return displays
}

// closeAll terminates all managed Xvfb displays (agent shutdown).
func (dm *DisplayManager) CloseAll() {
	dm.Mu.Lock()
	entries := make([]*ManagedDisplay, 0, len(dm.Displays))
	for _, e := range dm.Displays {
		entries = append(entries, e)
	}
	dm.Displays = make(map[string]*ManagedDisplay)
	dm.Mu.Unlock()

	for _, e := range entries {
		TerminateProcess(e.xvfbCmd)
		if e.xauthPath != "" {
			if err := os.Remove(e.xauthPath); err != nil && !os.IsNotExist(err) {
				log.Printf("desktop: remove xauth %s during shutdown: %v", e.xauthPath, err)
			}
		}
	}
}

// isDisplayAvailable checks whether an X11 display appears to be running
// by looking for its lock file or an active graphical login using that display.
func IsDisplayAvailable(display string) bool {
	display = NormalizeX11DisplayIdentifier(display)
	if display == "" {
		return false
	}

	lockFile := fmt.Sprintf("/tmp/.X%s-lock", strings.TrimPrefix(display, ":"))
	if _, err := os.Stat(lockFile); err == nil {
		return true
	}

	session := DetectDesktopSessionFn()
	if session.Type == DesktopSessionTypeX11 && session.Display == display {
		return true
	}

	sessions, err := system.CollectUserSessionsFn()
	if err != nil {
		return false
	}
	for _, session := range sessions {
		if strings.TrimSpace(session.RemoteHost) == display {
			return true
		}
		if display == ":0" && strings.EqualFold(strings.TrimSpace(session.Terminal), "seat0") {
			return true
		}
	}
	return false
}

func AppendDetectedActiveDisplays(dst []string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, display := range dst {
		normalized := NormalizeX11DisplayIdentifier(display)
		if normalized == "" {
			continue
		}
		seen[normalized] = struct{}{}
	}

	for i := 0; i <= 99; i++ {
		display := fmt.Sprintf(":%d", i)
		if !IsDisplayAvailable(display) {
			continue
		}
		if _, ok := seen[display]; ok {
			continue
		}
		seen[display] = struct{}{}
		dst = append(dst, display)
	}
	sort.Strings(dst)
	return dst
}
