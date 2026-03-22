package collectors

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/hubcollector"
)

const hubCollectorScanInterval = 5 * time.Second
const hubCollectorMaxRunDuration = 2 * time.Minute
const linkSuggestionScanInterval = 2 * time.Minute

type collectorExecutor func(*Deps, context.Context, hubcollector.Collector)

var CollectorExecutorRegistry = map[string]collectorExecutor{
	hubcollector.CollectorTypeSSH:           (*Deps).executeSSHCollector,
	hubcollector.CollectorTypeWinRM:         (*Deps).executeWinRMCollector,
	hubcollector.CollectorTypeAPI:           (*Deps).executeAPICollector,
	hubcollector.CollectorTypeProxmox:       (*Deps).executeProxmoxCollector,
	hubcollector.CollectorTypePBS:           (*Deps).executePBSCollector,
	hubcollector.CollectorTypeTrueNAS:       (*Deps).ExecuteTrueNASCollector,
	hubcollector.CollectorTypePortainer:     (*Deps).ExecutePortainerCollector,
	hubcollector.CollectorTypeDocker:        (*Deps).ExecuteDockerCollector,
	hubcollector.CollectorTypeHomeAssistant: (*Deps).executeHomeAssistantCollector,
}

func (d *Deps) RunHubCollectorLoop(ctx context.Context) {
	ticker := time.NewTicker(hubCollectorScanInterval)
	defer ticker.Stop()
	log.Printf("hub collector runner started (scan_interval=%s)", hubCollectorScanInterval)

	emptyRuns := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("hub collector runner stopped")
			return
		case <-ticker.C:
			found := d.RunPendingCollectors(ctx)
			if found == 0 {
				emptyRuns++
				if emptyRuns >= 12 {
					ticker.Reset(30 * time.Second)
				}
			} else {
				emptyRuns = 0
				ticker.Reset(hubCollectorScanInterval)
			}
		}
	}
}

func (d *Deps) RunPendingCollectors(ctx context.Context) int {
	if d.HubCollectorStore == nil {
		return 0
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(100, true)
	if err != nil {
		log.Printf("hub collector: failed to list collectors: %v", err)
		return 0
	}

	// Process the stalest collectors first so one recently-updated collector
	// does not monopolize every scan pass.
	sort.SliceStable(collectors, func(i, j int) bool {
		var left, right time.Time
		if collectors[i].LastCollectedAt != nil {
			left = collectors[i].LastCollectedAt.UTC()
		}
		if collectors[j].LastCollectedAt != nil {
			right = collectors[j].LastCollectedAt.UTC()
		}
		if left.Equal(right) {
			return collectors[i].ID < collectors[j].ID
		}
		if left.IsZero() {
			return true
		}
		if right.IsZero() {
			return false
		}
		return left.Before(right)
	})

	started := 0
	for _, collector := range collectors {
		select {
		case <-ctx.Done():
			return len(collectors)
		default:
		}

		// Check if enough time has elapsed since last collection
		if collector.LastCollectedAt != nil {
			now := time.Now().UTC()
			interval := time.Duration(collector.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 60 * time.Second
			}
			if now.Sub(*collector.LastCollectedAt) < interval {
				continue
			}
		}

		if d.startCollectorRun(ctx, collector, false) {
			started++
		}
	}

	return started
}

func (d *Deps) ExecuteCollector(ctx context.Context, collector hubcollector.Collector) {
	if !d.TryBeginCollectorRun(collector.ID) {
		log.Printf("hub collector: collector %s already running, skipping duplicate trigger", collector.ID)
		return
	}
	defer d.FinishCollectorRun(collector.ID)

	d.executeCollectorBody(ctx, collector)
}

func (d *Deps) startCollectorRun(ctx context.Context, collector hubcollector.Collector, waitForSlot bool) bool {
	if !d.TryBeginCollectorRun(collector.ID) {
		log.Printf("hub collector: collector %s already running, skipping duplicate trigger", collector.ID)
		return false
	}

	dispatchSem := d.CollectorDispatchSem
	if dispatchSem == nil {
		go func() {
			defer d.FinishCollectorRun(collector.ID)
			runCtx, cancel := context.WithTimeout(ctx, hubCollectorMaxRunDuration)
			defer cancel()
			d.executeCollectorBody(runCtx, collector)
		}()
		return true
	}

	if !waitForSlot {
		select {
		case dispatchSem <- struct{}{}:
		default:
			d.FinishCollectorRun(collector.ID)
			return false
		}

		go func() {
			defer func() { <-dispatchSem }()
			defer d.FinishCollectorRun(collector.ID)

			runCtx, cancel := context.WithTimeout(ctx, hubCollectorMaxRunDuration)
			defer cancel()
			d.executeCollectorBody(runCtx, collector)
		}()
		return true
	}

	go func() {
		acquired := false
		defer func() {
			if acquired {
				<-dispatchSem
			}
			d.FinishCollectorRun(collector.ID)
		}()

		select {
		case dispatchSem <- struct{}{}:
			acquired = true
		case <-ctx.Done():
			return
		}

		runCtx, cancel := context.WithTimeout(ctx, hubCollectorMaxRunDuration)
		defer cancel()
		d.executeCollectorBody(runCtx, collector)
	}()
	return true
}

func (d *Deps) executeCollectorBody(ctx context.Context, collector hubcollector.Collector) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("hub collector: panic in %s collector %s: %v", collector.CollectorType, collector.ID, r) // #nosec G706 -- Collector type and ID are bounded persisted identifiers; panic value is local runtime state.
			d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("panic: %v", r))
		}
	}()

	executor, ok := CollectorExecutorRegistry[collector.CollectorType]
	if !ok {
		log.Printf("hub collector: unsupported type %q for collector %s", collector.CollectorType, collector.ID)
		d.UpdateCollectorStatus(collector.ID, "error", fmt.Sprintf("unsupported type: %s", collector.CollectorType))
		return
	}

	executor(d, ctx, collector)

	// After each successful collection cycle, schedule a throttled link
	// suggestion pass. This keeps the detector off the collector hot path.
	d.scheduleLinkSuggestionDetection()
}

func (d *Deps) TryBeginCollectorRun(collectorID string) bool {
	key := strings.TrimSpace(collectorID)
	if key == "" {
		return false
	}
	state, _ := d.CollectorRunState.LoadOrStore(key, &atomic.Bool{})
	flag, ok := state.(*atomic.Bool)
	if !ok {
		return false
	}
	return flag.CompareAndSwap(false, true)
}

func (d *Deps) FinishCollectorRun(collectorID string) {
	key := strings.TrimSpace(collectorID)
	if key == "" {
		return
	}
	state, ok := d.CollectorRunState.Load(key)
	if !ok {
		return
	}
	flag, ok := state.(*atomic.Bool)
	if !ok {
		return
	}
	flag.Store(false)
}

func (d *Deps) scheduleLinkSuggestionDetection() {
	now := time.Now().UTC()

	d.LinkSuggestionScanMu.Lock()
	if d.LinkSuggestionScanRunning.Load() || (!d.LinkSuggestionScanLastStarted.IsZero() && now.Sub(d.LinkSuggestionScanLastStarted) < linkSuggestionScanInterval) {
		d.LinkSuggestionScanMu.Unlock()
		return
	}
	d.LinkSuggestionScanLastStarted = now
	d.LinkSuggestionScanRunning.Store(true)
	d.LinkSuggestionScanMu.Unlock()

	go func() {
		defer d.LinkSuggestionScanRunning.Store(false)
		if err := d.DetectLinkSuggestions(); err != nil {
			log.Printf("hub collector: link suggestion detection failed: %v", err)
		}
	}()
}
