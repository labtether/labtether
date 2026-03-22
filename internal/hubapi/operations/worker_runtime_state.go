package operations

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
)

// WorkerRuntimeState holds atomic worker runtime configuration that can be
// updated from database overrides without restarting the worker.
type WorkerRuntimeState struct {
	baseMaxDeliveries      uint64
	baseRetentionApply     time.Duration
	maxDeliveries          atomic.Uint64
	retentionIntervalNanos atomic.Int64
}

// NewWorkerRuntimeState creates a WorkerRuntimeState with the given defaults.
func NewWorkerRuntimeState(maxDeliveries uint64, retentionInterval time.Duration) *WorkerRuntimeState {
	if maxDeliveries == 0 {
		maxDeliveries = 5
	}
	if retentionInterval <= 0 {
		retentionInterval = 5 * time.Minute
	}

	state := &WorkerRuntimeState{
		baseMaxDeliveries:  maxDeliveries,
		baseRetentionApply: retentionInterval,
	}
	state.maxDeliveries.Store(maxDeliveries)
	state.retentionIntervalNanos.Store(retentionInterval.Nanoseconds())
	return state
}

// MaxDeliveries returns the current max delivery attempts.
func (s *WorkerRuntimeState) MaxDeliveries() uint64 {
	if s == nil {
		return 5
	}
	value := s.maxDeliveries.Load()
	if value == 0 {
		return 5
	}
	return value
}

// RetentionInterval returns the current retention loop interval.
func (s *WorkerRuntimeState) RetentionInterval() time.Duration {
	if s == nil {
		return 5 * time.Minute
	}
	value := time.Duration(s.retentionIntervalNanos.Load())
	if value <= 0 {
		return 5 * time.Minute
	}
	return value
}

// ApplyOverrides applies runtime setting overrides from the database.
func (s *WorkerRuntimeState) ApplyOverrides(overrides map[string]string) {
	if s == nil {
		return
	}

	nextMax := s.baseMaxDeliveries
	nextInterval := s.baseRetentionApply

	if raw := strings.TrimSpace(overrides[runtimesettings.KeyWorkerQueueMaxDeliveries]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 {
			nextMax = uint64(parsed)
		}
	}

	if raw := strings.TrimSpace(overrides[runtimesettings.KeyWorkerRetentionApply]); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err == nil && parsed > 0 {
			nextInterval = parsed
		}
	}

	s.maxDeliveries.Store(nextMax)
	s.retentionIntervalNanos.Store(nextInterval.Nanoseconds())
}

// RefreshWorkerRuntimeSettingsDirect reads overrides from the database directly
// instead of polling over HTTP, since the worker is now in-process.
func RefreshWorkerRuntimeSettingsDirect(
	ctx context.Context,
	store persistence.RuntimeSettingsStore,
	state *WorkerRuntimeState,
	onApplied func(*WorkerRuntimeState),
) {
	if state == nil || store == nil {
		return
	}

	apply := func() {
		overrides, err := store.ListRuntimeSettingOverrides()
		if err != nil {
			log.Printf("labtether worker runtime settings refresh failed: %v", err)
			return
		}
		state.ApplyOverrides(overrides)
		if onApplied != nil {
			onApplied(state)
		}
	}

	apply()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("labtether worker runtime settings refresh stopped")
			return
		case <-ticker.C:
			apply()
		}
	}
}
