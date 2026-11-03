package agents

import (
	"strings"
	"sync"
	"time"
)

// maxRejectedPendingDevices bounds the memory of rejected devices.
const maxRejectedPendingDevices = 1000

// refusalRejectedPending answers a pending request from a rejected device.
const refusalRejectedPending = "An admin rejected this device in LabTether. To add it, run the install command with an enrollment token from Devices, Add device."

// rejectedPendingDevices remembers devices an admin rejected in Pending. An
// agent asked again within seconds and was back in the list, so Reject did
// nothing. A rejected device's pending requests are refused until it enrolls
// with a token. The memory lasts until the Hub restarts.
type rejectedPendingDevices struct {
	mu   sync.Mutex
	keys map[string]time.Time
}

// rejectionKey is the device key fingerprint, or the hostname for an agent
// that sent none.
func rejectionKey(fingerprint, hostname string) string {
	if fp := strings.TrimSpace(fingerprint); fp != "" {
		return "fp:" + fp
	}
	if host := strings.ToLower(strings.TrimSpace(hostname)); host != "" {
		return "host:" + host
	}
	return ""
}

func (r *rejectedPendingDevices) remember(fingerprint, hostname string) {
	key := rejectionKey(fingerprint, hostname)
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.keys == nil {
		r.keys = make(map[string]time.Time)
	}
	if len(r.keys) >= maxRejectedPendingDevices {
		oldestKey, oldest := "", time.Time{}
		for k, at := range r.keys {
			if oldestKey == "" || at.Before(oldest) {
				oldestKey, oldest = k, at
			}
		}
		delete(r.keys, oldestKey)
	}
	r.keys[key] = time.Now()
}

func (r *rejectedPendingDevices) has(fingerprint, hostname string) bool {
	key := rejectionKey(fingerprint, hostname)
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.keys[key]
	return key != "" && ok
}

func (r *rejectedPendingDevices) forget(fingerprint, hostname string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.keys, rejectionKey(fingerprint, hostname))
	delete(r.keys, rejectionKey("", hostname))
}
