package truenas

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) EnsureTrueNASSubscriptionWorker(ctx context.Context, collector hubcollector.Collector, runtime *TruenasRuntime) {
	if runtime == nil || runtime.Client == nil {
		return
	}

	collectorID := strings.TrimSpace(runtime.CollectorID)
	if collectorID == "" {
		collectorID = strings.TrimSpace(collector.ID)
	}
	if collectorID == "" {
		return
	}

	configKey := strings.TrimSpace(runtime.ConfigKey)
	if configKey == "" {
		configKey = strings.TrimSpace(runtime.BaseURL)
	}

	d.TruenasSubMu.Lock()
	if d.TruenasSubs == nil {
		d.TruenasSubs = make(map[string]TruenasSubscriptionHandle)
	}
	if existing, ok := d.TruenasSubs[collectorID]; ok {
		if existing.ConfigKey == configKey {
			d.TruenasSubMu.Unlock()
			return
		}
		existing.Cancel()
		delete(d.TruenasSubs, collectorID)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	d.TruenasSubs[collectorID] = TruenasSubscriptionHandle{
		ConfigKey: configKey,
		Cancel:    cancel,
	}
	d.TruenasSubMu.Unlock()

	go d.RunTrueNASSubscriptionWorker(workerCtx, collector, runtime)
}

func (d *Deps) RunTrueNASSubscriptionWorker(ctx context.Context, collector hubcollector.Collector, runtime *TruenasRuntime) {
	collectorID := strings.TrimSpace(runtime.CollectorID)
	if collectorID == "" {
		collectorID = strings.TrimSpace(collector.ID)
	}
	configKey := strings.TrimSpace(runtime.ConfigKey)
	defer d.UnregisterTrueNASSubscriptionWorker(collectorID, configKey)

	logFields := map[string]string{
		"collector_id":   collectorID,
		"collector_type": hubcollector.CollectorTypeTrueNAS,
		"subscription":   "alert.list",
	}
	d.AppendConnectorLogEvent(
		collector.AssetID,
		"truenas",
		"info",
		"subscription worker started (collection=alert.list)",
		logFields,
		time.Now().UTC(),
	)

	initialBackoff, maxBackoff := SubscriptionBackoffBounds()
	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		if !d.IsTrueNASCollectorActive(collectorID) {
			log.Printf("hub collector truenas: stopping subscription worker for inactive collector %s", collectorID)
			return
		}

		err := runtime.Client.Subscribe(ctx, "alert.list", func(event tnconnector.SubscriptionEvent) error {
			d.IngestTrueNASSubscriptionEvent(collector, collectorID, event)
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("hub collector truenas: subscription stream error for %s: %v", collectorID, err)
			d.AppendConnectorLogEvent(
				collector.AssetID,
				"truenas",
				"warn",
				fmt.Sprintf("subscription stream interrupted: %v", err),
				logFields,
				time.Now().UTC(),
			)
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (d *Deps) UnregisterTrueNASSubscriptionWorker(collectorID, configKey string) {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID == "" {
		return
	}

	d.TruenasSubMu.Lock()
	defer d.TruenasSubMu.Unlock()

	current, ok := d.TruenasSubs[collectorID]
	if !ok {
		return
	}
	if configKey != "" && current.ConfigKey != configKey {
		return
	}
	delete(d.TruenasSubs, collectorID)
}

func (d *Deps) IsTrueNASCollectorActive(collectorID string) bool {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID == "" || d.HubCollectorStore == nil {
		return false
	}

	collector, ok, err := d.HubCollectorStore.GetHubCollector(collectorID)
	if err != nil {
		// Keep existing worker alive when the store is temporarily unavailable.
		log.Printf("hub collector truenas: failed to load collector %s status: %v", collectorID, err)
		return true
	}
	if !ok {
		return false
	}
	if !collector.Enabled {
		return false
	}
	return collector.CollectorType == hubcollector.CollectorTypeTrueNAS
}
