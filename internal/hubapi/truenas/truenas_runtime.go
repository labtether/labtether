package truenas

import (
	"sync"
	"time"
)

var (
	SubscriptionInitialBackoff = 3 * time.Second
	SubscriptionMaxBackoff     = time.Minute
	SubscriptionBackoffMu      sync.RWMutex
)

func SubscriptionBackoffBounds() (initial, max time.Duration) {
	SubscriptionBackoffMu.RLock()
	defer SubscriptionBackoffMu.RUnlock()
	return SubscriptionInitialBackoff, SubscriptionMaxBackoff
}
