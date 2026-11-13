package collectors

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

// CollectorIntervalMetadataKey carries a connection's sync interval on every
// device it makes. A connection refreshes its devices once per interval, so
// the console and the Hub's counts (assets.Freshness) judge their freshness
// against it instead of the agent heartbeat limits (65 s / 300 s), which made
// devices of a 120 s connection flap between online and "Not responding".
const CollectorIntervalMetadataKey = assets.MetadataKeyCollectorInterval

const collectorIntervalCacheTTL = 30 * time.Second

type collectorIntervalEntry struct {
	seconds int
	readAt  time.Time
}

// A Home Assistant run can store hundreds of devices; the interval is read
// from the connection at most once per TTL per connection.
var collectorIntervalCache = struct {
	sync.Mutex
	entries map[string]collectorIntervalEntry
}{entries: map[string]collectorIntervalEntry{}}

func effectiveCollectorInterval(seconds int) int {
	if seconds <= 0 {
		return hubcollector.DefaultIntervalSeconds
	}
	return seconds
}

// withCollectorInterval returns metadata with the interval set. The input
// map is not changed.
func withCollectorInterval(metadata map[string]string, seconds int) map[string]string {
	out := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		out[key] = value
	}
	out[CollectorIntervalMetadataKey] = strconv.Itoa(effectiveCollectorInterval(seconds))
	return out
}

// collectorIntervalSeconds returns the saved interval of a connection, or 0
// when it cannot be read (the device then keeps the default limits).
func (d *Deps) collectorIntervalSeconds(collectorID string) int {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID == "" || d.HubCollectorStore == nil {
		return 0
	}
	now := time.Now()
	collectorIntervalCache.Lock()
	entry, ok := collectorIntervalCache.entries[collectorID]
	collectorIntervalCache.Unlock()
	if ok && now.Sub(entry.readAt) < collectorIntervalCacheTTL {
		return entry.seconds
	}
	collector, found, err := d.HubCollectorStore.GetHubCollector(collectorID)
	if err != nil || !found {
		return 0
	}
	seconds := effectiveCollectorInterval(collector.IntervalSeconds)
	collectorIntervalCache.Lock()
	collectorIntervalCache.entries[collectorID] = collectorIntervalEntry{seconds: seconds, readAt: now}
	collectorIntervalCache.Unlock()
	return seconds
}
