package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

// prometheusRemoteWriteRuntime owns exactly one worker. Apply builds a complete
// replacement before canceling the active worker, then waits for the canceled
// request to finish before starting the replacement. This prevents overlapping
// deliveries during live settings changes.
type prometheusRemoteWriteRuntime struct {
	rootCtx context.Context
	store   persistence.RuntimeSettingsStore
	secrets *secrets.Manager
	source  remotewrite.SampleSource
	cursors remotewrite.CursorStore

	applyMu      sync.Mutex
	mu           sync.Mutex
	activeConfig *remotewrite.Config
	cancel       context.CancelFunc
	done         chan struct{}
	stopped      bool
}

func newPrometheusRemoteWriteRuntime(ctx context.Context, store persistence.RuntimeSettingsStore, manager *secrets.Manager, source remotewrite.SampleSource, cursors remotewrite.CursorStore) *prometheusRemoteWriteRuntime {
	return &prometheusRemoteWriteRuntime{
		rootCtx: ctx,
		store:   store,
		secrets: manager,
		source:  source,
		cursors: cursors,
	}
}

func (r *prometheusRemoteWriteRuntime) Apply() error {
	if r == nil || r.rootCtx == nil || r.store == nil || r.source == nil || r.cursors == nil {
		return fmt.Errorf("prometheus remote write runtime is unavailable")
	}
	// Serialize settings resolution and replacement so a slower, older apply
	// cannot overwrite a newer runtime configuration.
	r.applyMu.Lock()
	defer r.applyMu.Unlock()
	values, _, err := shared.ResolveRuntimeSettingEffectiveValues(r.store, r.secrets)
	if err != nil {
		return fmt.Errorf("load prometheus remote write settings: %w", err)
	}
	config, err := remotewrite.ConfigFromRuntimeValues(values)
	if err != nil {
		return err
	}
	var worker *remotewrite.Worker
	if config.Enabled {
		worker, err = remotewrite.NewWorker(config, r.source, r.cursors)
		if err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped || r.rootCtx.Err() != nil {
		return fmt.Errorf("prometheus remote write runtime is stopping")
	}
	if r.activeConfig != nil && *r.activeConfig == config {
		return nil
	}
	r.stopActiveLocked()
	r.activeConfig = &config
	if worker == nil {
		return nil
	}
	workerCtx, cancel := context.WithCancel(r.rootCtx)
	done := make(chan struct{})
	r.cancel = cancel
	r.done = done
	var workerWG sync.WaitGroup
	servicehttp.SafeGo(workerCtx, &workerWG, "prometheus-remote-write-worker", func(ctx context.Context) {
		worker.Run(ctx)
	})
	go func() {
		workerWG.Wait()
		defer close(done)
	}()
	return nil
}

func (r *prometheusRemoteWriteRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	r.stopActiveLocked()
}

func (r *prometheusRemoteWriteRuntime) stopActiveLocked() {
	if r.cancel != nil {
		r.cancel()
	}
	if r.done != nil {
		<-r.done
	}
	r.cancel = nil
	r.done = nil
}

func startPrometheusRemoteWrite(ctx context.Context, srv *apiServer, pgStore *persistence.PostgresStore) {
	if srv == nil || pgStore == nil {
		return
	}
	runtime := newPrometheusRemoteWriteRuntime(ctx, srv.runtimeStore, srv.secretsManager, pgStore, pgStore)
	srv.prometheusRemoteWriteRuntime = runtime
	if err := runtime.Apply(); err != nil {
		// Config errors and transport identities are intentionally not expanded
		// with the URL or secret-bearing settings map.
		log.Printf("labtether: prometheus remote write runtime not started: %v", err)
	}
	servicehttp.SafeGo(ctx, &srv.backgroundWG, "prometheus-remote-write", func(ctx context.Context) {
		<-ctx.Done()
		runtime.Stop()
	})
}
