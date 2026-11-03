package connectorsdk

import (
	"fmt"
	"strings"
	"time"
)

// TargetOfflineError says an action cannot reach its target because the
// device that runs it is offline. Its text is meant for people as is.
type TargetOfflineError struct {
	// Device is the offline device's name.
	Device string
	// LastSeen is when its agent last reported; zero when never.
	LastSeen time.Time
}

func (e *TargetOfflineError) Error() string {
	device := strings.TrimSpace(e.Device)
	if device == "" {
		device = "The device"
	}
	if e.LastSeen.IsZero() {
		return device + " is offline."
	}
	return fmt.Sprintf("%s is offline. Its agent was last seen at %s.", device, e.LastSeen.UTC().Format("2006-01-02 15:04 UTC"))
}
