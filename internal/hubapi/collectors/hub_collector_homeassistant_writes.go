package collectors

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/assets"
)

// homeAssistantFullRefreshInterval forces a full write of every entity now and
// then, so a row changed or removed outside the collector is put right.
const homeAssistantFullRefreshInterval = 15 * time.Minute

// homeAssistantWrites remembers what each Home Assistant collector last wrote.
// A large install has thousands of entities and most do not change between
// runs. Unchanged entities only get their last-seen time refreshed in one bulk
// statement, instead of a full heartbeat write each every run.
var homeAssistantWrites = &homeAssistantWriteCache{collectors: make(map[string]homeAssistantWriteState)}

type homeAssistantWriteState struct {
	fingerprints map[string]string
	refreshedAt  time.Time
}

type homeAssistantWriteCache struct {
	mu         sync.Mutex
	collectors map[string]homeAssistantWriteState
}

// homeAssistantRun tracks one collector run. changed reports whether any
// entity was written in full, removed, or a full refresh is due, which is
// when the connector snapshot also needs rewriting.
type homeAssistantRun struct {
	previous map[string]string
	current  map[string]string
	full     bool
	changed  bool
}

func (c *homeAssistantWriteCache) begin(collectorID string, now time.Time) *homeAssistantRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.collectors[collectorID]
	full := state.fingerprints == nil || now.Sub(state.refreshedAt) >= homeAssistantFullRefreshInterval
	return &homeAssistantRun{previous: state.fingerprints, current: make(map[string]string), full: full, changed: full}
}

func (c *homeAssistantWriteCache) finish(collectorID string, run *homeAssistantRun, now time.Time) {
	for assetID := range run.previous {
		if _, ok := run.current[assetID]; !ok {
			run.changed = true
			break
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := homeAssistantWriteState{fingerprints: run.current, refreshedAt: c.collectors[collectorID].refreshedAt}
	if run.full {
		state.refreshedAt = now
	}
	c.collectors[collectorID] = state
}

// unchanged returns the request's fingerprint and whether it matches what this
// collector last wrote for the same asset.
func (r *homeAssistantRun) unchanged(req assets.HeartbeatRequest) (string, bool) {
	fingerprint := homeAssistantHeartbeatFingerprint(req)
	return fingerprint, fingerprint != "" && !r.full && r.previous[req.AssetID] == fingerprint
}

func homeAssistantHeartbeatFingerprint(req assets.HeartbeatRequest) string {
	// json.Marshal sorts map keys, so equal requests give equal bytes.
	payload, err := json.Marshal(struct {
		ID, Type, Name, Source, Status, GroupID, Platform string
		Metadata                                          map[string]string
	}{req.AssetID, req.Type, req.Name, req.Source, req.Status, req.GroupID, req.Platform, req.Metadata})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
