package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"golang.org/x/sync/singleflight"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	agentTelemetryRequestTimeout = 3 * time.Second
	agentTelemetryConcurrency    = 8
	networkCounterMaxAge         = 10 * time.Minute
)

type processMetricsConfigResolver func() (enabled bool, topN int)

// agentTelemetryAdapter backs the process, interface, and mount bridges using
// the existing authenticated agent request channels. Each method has its own
// single-flight key and rotating fleet cursor so concurrent callers share work
// and fleets larger than one sweep cap are covered over subsequent intervals.
type agentTelemetryAdapter struct {
	agentMgr       *agentmgr.AgentManager
	processBridges *sync.Map
	networkBridges *sync.Map
	diskBridges    *sync.Map
	resolveProcess processMetricsConfigResolver
	requestTimeout time.Duration
	requestSem     chan struct{}

	flight singleflight.Group

	cursorMu sync.Mutex
	cursors  map[string]int

	networkMu       sync.Mutex
	networkCounters map[string]networkCounterSnapshot
}

type networkCounterSnapshot struct {
	rxBytes uint64
	txBytes uint64
	at      time.Time
}

func newAgentTelemetryAdapter(srv *apiServer) *agentTelemetryAdapter {
	adapter := &agentTelemetryAdapter{
		agentMgr:        srv.agentMgr,
		processBridges:  &srv.processBridges,
		networkBridges:  &srv.networkBridges,
		diskBridges:     &srv.diskBridges,
		requestTimeout:  agentTelemetryRequestTimeout,
		requestSem:      make(chan struct{}, agentTelemetryConcurrency),
		cursors:         make(map[string]int),
		networkCounters: make(map[string]networkCounterSnapshot),
	}
	adapter.resolveProcess = func() (bool, int) {
		if srv.runtimeStore == nil {
			return false, 0
		}
		values, _, err := shared.ResolveRuntimeSettingEffectiveValues(srv.runtimeStore, srv.secretsManager)
		if err != nil {
			return false, 0
		}
		enabled, err := strconv.ParseBool(strings.TrimSpace(values[runtimesettings.KeyProcessMetricsEnabled]))
		if err != nil || !enabled {
			return false, 0
		}
		topN, err := strconv.Atoi(strings.TrimSpace(values[runtimesettings.KeyProcessMetricsTopN]))
		if err != nil || topN < 1 {
			return false, 0
		}
		if topN > telemetry.MaxBridgeProcessesPerAsset {
			topN = telemetry.MaxBridgeProcessesPerAsset
		}
		return true, topN
	}
	return adapter
}

func (a *agentTelemetryAdapter) timeout() time.Duration {
	if a.requestTimeout <= 0 {
		return agentTelemetryRequestTimeout
	}
	return a.requestTimeout
}

func (a *agentTelemetryAdapter) connectedAssetBatch(kind string) []string {
	if a == nil || a.agentMgr == nil {
		return nil
	}
	ids := a.agentMgr.ConnectedAssets()
	sort.Strings(ids)
	if len(ids) <= telemetry.MaxBridgeAgentAssets {
		return ids
	}
	a.cursorMu.Lock()
	start := a.cursors[kind] % len(ids)
	a.cursors[kind] = (start + telemetry.MaxBridgeAgentAssets) % len(ids)
	a.cursorMu.Unlock()
	out := make([]string, 0, telemetry.MaxBridgeAgentAssets)
	for offset := 0; offset < telemetry.MaxBridgeAgentAssets; offset++ {
		out = append(out, ids[(start+offset)%len(ids)])
	}
	return out
}

func collectAgentAssets[T any](assetIDs []string, sem chan struct{}, collect func(string) []T) []T {
	if len(assetIDs) == 0 {
		return nil
	}
	if cap(sem) == 0 {
		sem = make(chan struct{}, agentTelemetryConcurrency)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var out []T
	for _, assetID := range assetIDs {
		assetID := assetID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			entries := func() (entries []T) {
				defer func() {
					<-sem
					if recover() != nil {
						entries = nil
					}
				}()
				return collect(assetID)
			}()
			if len(entries) == 0 {
				return
			}
			mu.Lock()
			out = append(out, entries...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func (a *agentTelemetryAdapter) AllProcessMetrics() []bridge.ProcessMetricEntry {
	if a == nil || a.resolveProcess == nil {
		return nil
	}
	enabled, topN := a.resolveProcess()
	if !enabled || topN < 1 {
		return nil
	}
	value, _, _ := a.flight.Do("process", func() (any, error) {
		entries := collectAgentAssets(a.connectedAssetBatch("process"), a.requestSem, func(assetID string) []bridge.ProcessMetricEntry {
			return a.collectProcesses(assetID, topN)
		})
		return entries, nil
	})
	entries, _ := value.([]bridge.ProcessMetricEntry)
	return entries
}

func (a *agentTelemetryAdapter) collectProcesses(assetID string, topN int) []bridge.ProcessMetricEntry {
	conn, ok := a.agentMgr.Get(assetID)
	if !ok || conn == nil || a.processBridges == nil {
		return nil
	}
	requestID := generateRequestID()
	waiter := &processBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	a.processBridges.Store(requestID, waiter)
	defer a.processBridges.Delete(requestID)
	payload, _ := json.Marshal(agentmgr.ProcessListData{RequestID: requestID, SortBy: "cpu", Limit: topN})
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgProcessList, ID: requestID, Data: payload}); err != nil {
		return nil
	}
	timer := time.NewTimer(a.timeout())
	defer timer.Stop()
	var response agentmgr.ProcessListedData
	select {
	case msg := <-waiter.Ch:
		if json.Unmarshal(msg.Data, &response) != nil || strings.TrimSpace(response.Error) != "" {
			return nil
		}
	case <-timer.C:
		return nil
	}
	maxRawProcesses := telemetry.MaxBridgeProcessesPerAsset * 10
	if len(response.Processes) > maxRawProcesses {
		response.Processes = response.Processes[:maxRawProcesses]
	}
	sort.Slice(response.Processes, func(i, j int) bool {
		if response.Processes[i].CPUPct != response.Processes[j].CPUPct {
			return response.Processes[i].CPUPct > response.Processes[j].CPUPct
		}
		return response.Processes[i].PID < response.Processes[j].PID
	})
	out := make([]bridge.ProcessMetricEntry, 0, min(len(response.Processes), topN))
	seenPIDs := make(map[int]struct{}, topN)
	for _, process := range response.Processes {
		if len(out) >= topN {
			break
		}
		name, validName := boundedMetricLabel(process.Name, 256)
		if process.PID <= 0 || !validName || !finiteRange(process.CPUPct, 0, 10000) ||
			!finiteRange(process.MemPct, 0, 100) || process.MemRSS < 0 {
			continue
		}
		if _, duplicate := seenPIDs[process.PID]; duplicate {
			continue
		}
		seenPIDs[process.PID] = struct{}{}
		out = append(out, bridge.ProcessMetricEntry{
			AssetID: assetID, CPUPercent: process.CPUPct, MemPercent: process.MemPct, MemRSS: float64(process.MemRSS),
			Labels: map[string]string{"process_name": name, "process_pid": strconv.Itoa(process.PID)},
		})
	}
	return out
}

func (a *agentTelemetryAdapter) AllNetworkInterfaces() []bridge.NetworkInterfaceEntry {
	if a == nil {
		return nil
	}
	value, _, _ := a.flight.Do("network", func() (any, error) {
		entries := collectAgentAssets(a.connectedAssetBatch("network"), a.requestSem, a.collectNetworkInterfaces)
		return entries, nil
	})
	entries, _ := value.([]bridge.NetworkInterfaceEntry)
	return entries
}

func (a *agentTelemetryAdapter) collectNetworkInterfaces(assetID string) []bridge.NetworkInterfaceEntry {
	conn, ok := a.agentMgr.Get(assetID)
	if !ok || conn == nil || a.networkBridges == nil {
		return nil
	}
	requestID := generateRequestID()
	waiter := &networkBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	a.networkBridges.Store(requestID, waiter)
	defer a.networkBridges.Delete(requestID)
	payload, _ := json.Marshal(agentmgr.NetworkListData{RequestID: requestID})
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgNetworkList, ID: requestID, Data: payload}); err != nil {
		return nil
	}
	timer := time.NewTimer(a.timeout())
	defer timer.Stop()
	var response agentmgr.NetworkListedData
	select {
	case msg := <-waiter.Ch:
		if json.Unmarshal(msg.Data, &response) != nil || strings.TrimSpace(response.Error) != "" {
			return nil
		}
	case <-timer.C:
		return nil
	}
	maxRawInterfaces := telemetry.MaxBridgeInterfacesPerAsset * 10
	if len(response.Interfaces) > maxRawInterfaces {
		response.Interfaces = response.Interfaces[:maxRawInterfaces]
	}
	sort.Slice(response.Interfaces, func(i, j int) bool { return response.Interfaces[i].Name < response.Interfaces[j].Name })
	now := time.Now().UTC()
	out := make([]bridge.NetworkInterfaceEntry, 0, telemetry.MaxBridgeInterfacesPerAsset)
	a.networkMu.Lock()
	for key, previous := range a.networkCounters {
		if now.Sub(previous.at) > networkCounterMaxAge {
			delete(a.networkCounters, key)
		}
	}
	seenInterfaces := make(map[string]struct{}, telemetry.MaxBridgeInterfacesPerAsset)
	for _, iface := range response.Interfaces {
		name, validName := boundedMetricLabel(iface.Name, 128)
		if !validName {
			continue
		}
		if _, duplicate := seenInterfaces[name]; duplicate {
			continue
		}
		if len(seenInterfaces) >= telemetry.MaxBridgeInterfacesPerAsset {
			break
		}
		seenInterfaces[name] = struct{}{}
		key := assetID + "\x00" + name
		previous, hasPrevious := a.networkCounters[key]
		a.networkCounters[key] = networkCounterSnapshot{rxBytes: iface.RXBytes, txBytes: iface.TXBytes, at: now}
		if !hasPrevious || !now.After(previous.at) || iface.RXBytes < previous.rxBytes || iface.TXBytes < previous.txBytes {
			continue
		}
		seconds := now.Sub(previous.at).Seconds()
		rxRate := float64(iface.RXBytes-previous.rxBytes) / seconds
		txRate := float64(iface.TXBytes-previous.txBytes) / seconds
		if !finiteNonNegative(rxRate) || !finiteNonNegative(txRate) {
			continue
		}
		out = append(out, bridge.NetworkInterfaceEntry{
			AssetID: assetID, RXBytes: rxRate, TXBytes: txRate,
			RXPackets: float64(iface.RXPackets), TXPackets: float64(iface.TXPackets),
			Labels: map[string]string{"interface": name},
		})
	}
	a.networkMu.Unlock()
	return out
}

func (a *agentTelemetryAdapter) AllDiskMounts() []bridge.DiskMountEntry {
	if a == nil {
		return nil
	}
	value, _, _ := a.flight.Do("disk", func() (any, error) {
		entries := collectAgentAssets(a.connectedAssetBatch("disk"), a.requestSem, a.collectDiskMounts)
		return entries, nil
	})
	entries, _ := value.([]bridge.DiskMountEntry)
	return entries
}

func (a *agentTelemetryAdapter) collectDiskMounts(assetID string) []bridge.DiskMountEntry {
	conn, ok := a.agentMgr.Get(assetID)
	if !ok || conn == nil || a.diskBridges == nil {
		return nil
	}
	requestID := generateRequestID()
	waiter := &diskBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	a.diskBridges.Store(requestID, waiter)
	defer a.diskBridges.Delete(requestID)
	payload, _ := json.Marshal(agentmgr.DiskListData{RequestID: requestID})
	if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgDiskList, ID: requestID, Data: payload}); err != nil {
		return nil
	}
	timer := time.NewTimer(a.timeout())
	defer timer.Stop()
	var response agentmgr.DiskListedData
	select {
	case msg := <-waiter.Ch:
		if json.Unmarshal(msg.Data, &response) != nil || strings.TrimSpace(response.Error) != "" {
			return nil
		}
	case <-timer.C:
		return nil
	}
	maxRawMounts := telemetry.MaxBridgeMountsPerAsset * 10
	if len(response.Mounts) > maxRawMounts {
		response.Mounts = response.Mounts[:maxRawMounts]
	}
	sort.Slice(response.Mounts, func(i, j int) bool { return response.Mounts[i].MountPoint < response.Mounts[j].MountPoint })
	out := make([]bridge.DiskMountEntry, 0, telemetry.MaxBridgeMountsPerAsset)
	seenMounts := make(map[string]struct{}, telemetry.MaxBridgeMountsPerAsset)
	for _, mount := range response.Mounts {
		mountPoint, validMount := boundedMetricLabel(mount.MountPoint, 512)
		if !validMount || mount.Used > mount.Total || mount.Available > mount.Total ||
			mount.Used > mount.Total-mount.Available || !finiteRange(mount.UsePct, 0, 100) {
			continue
		}
		if _, duplicate := seenMounts[mountPoint]; duplicate {
			continue
		}
		if len(seenMounts) >= telemetry.MaxBridgeMountsPerAsset {
			break
		}
		seenMounts[mountPoint] = struct{}{}
		out = append(out, bridge.DiskMountEntry{
			AssetID: assetID, Total: float64(mount.Total), Used: float64(mount.Used),
			Available: float64(mount.Available), UsePct: mount.UsePct,
			Labels: map[string]string{"mount_point": mountPoint},
		})
	}
	return out
}
