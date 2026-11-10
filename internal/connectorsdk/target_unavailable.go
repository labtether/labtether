package connectorsdk

import (
	"fmt"
	"strings"
	"time"
)

// TargetUnavailableError says an action cannot reach its target because the
// device that runs it is offline, or is connected but has not reported the
// target since it connected. Its text is meant for people as is.
type TargetUnavailableError struct {
	// Device is the device's name.
	Device string
	// LastSeen is when its agent last reported; zero when never.
	LastSeen time.Time
	// NotReported marks a connected device that has not yet sent What.
	NotReported bool
	// What names what it reports, for example "containers".
	What string
}

func (e *TargetUnavailableError) Error() string {
	device := strings.TrimSpace(e.Device)
	if device == "" {
		device = "The device"
	}
	if e.NotReported {
		what := strings.TrimSpace(e.What)
		if what == "" {
			what = "its resources"
		} else {
			what = "its " + what
		}
		return fmt.Sprintf("%s has not sent %s since it connected.", device, what)
	}
	if e.LastSeen.IsZero() {
		return device + " is offline."
	}
	return fmt.Sprintf("%s is offline. Its agent was last seen at %s.", device, e.LastSeen.UTC().Format("2006-01-02 15:04 UTC"))
}
