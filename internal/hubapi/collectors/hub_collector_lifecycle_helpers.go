package collectors

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

type CollectorLifecycle struct {
	deps      *Deps
	collector hubcollector.Collector
	source    string
	logFields map[string]string
}

func NewCollectorLifecycle(d *Deps, collector hubcollector.Collector, source, collectorType string) CollectorLifecycle {
	return CollectorLifecycle{
		deps:      d,
		collector: collector,
		source:    strings.TrimSpace(source),
		logFields: map[string]string{
			"collector_id":   collector.ID,
			"collector_type": strings.TrimSpace(collectorType),
		},
	}
}

func (l CollectorLifecycle) Fail(message string) {
	errMsg := strings.TrimSpace(message)
	if errMsg == "" {
		return
	}
	l.deps.AppendConnectorLogEvent(
		l.collector.AssetID,
		l.source,
		"error",
		"collector run failed: "+errMsg,
		l.logFields,
		time.Now().UTC(),
	)
	l.deps.UpdateCollectorStatus(l.collector.ID, "error", errMsg)
}

func (l CollectorLifecycle) Failf(format string, args ...any) {
	l.Fail(fmt.Sprintf(format, args...))
}

func (l CollectorLifecycle) Partial(message string) {
	warnMsg := strings.TrimSpace(message)
	if warnMsg == "" {
		return
	}
	l.deps.AppendConnectorLogEvent(
		l.collector.AssetID,
		l.source,
		"warning",
		"collector run partial: "+warnMsg,
		l.logFields,
		time.Now().UTC(),
	)
	l.deps.UpdateCollectorStatus(l.collector.ID, "partial", warnMsg)
}

func (l CollectorLifecycle) Partialf(format string, args ...any) {
	l.Partial(fmt.Sprintf(format, args...))
}

type CollectorParentAssetRefreshOptions struct {
	Collector      hubcollector.Collector
	Source         string
	AssetType      string
	Name           string
	Status         string
	Metadata       map[string]string
	LogPrefix      string
	FailureSubject string
}

func (d *Deps) KeepConnectorClusterAssetAlive(
	collector hubcollector.Collector,
	source string,
	discovered int,
	logPrefix string,
) (connectorsdk.Asset, bool) {
	trimmedSource := strings.TrimSpace(source)
	// Derive display name from collector config (cluster_name or display_name).
	name := CollectorConfigString(collector.Config, "cluster_name")
	if name == "" {
		name = CollectorConfigString(collector.Config, "display_name")
	}
	return d.RefreshCollectorParentAsset(CollectorParentAssetRefreshOptions{
		Collector: collector,
		Source:    trimmedSource,
		AssetType: "connector-cluster",
		Name:      name,
		Status:    "online",
		Metadata: map[string]string{
			"connector_type": trimmedSource,
			"discovered":     strconv.Itoa(discovered),
		},
		LogPrefix:      logPrefix,
		FailureSubject: "cluster asset",
	})
}

func (d *Deps) RefreshCollectorParentAsset(opts CollectorParentAssetRefreshOptions) (connectorsdk.Asset, bool) {
	assetID := strings.TrimSpace(opts.Collector.AssetID)
	source := strings.TrimSpace(opts.Source)
	assetType := strings.TrimSpace(opts.AssetType)
	if assetID == "" || source == "" || assetType == "" {
		return connectorsdk.Asset{}, false
	}

	status := strings.TrimSpace(opts.Status)
	if status == "" {
		status = "online"
	}
	metadata := cloneStringMap(opts.Metadata)
	if collectorID := strings.TrimSpace(opts.Collector.ID); collectorID != "" {
		metadata["collector_id"] = collectorID
	}
	_, metadata = WithCanonicalResourceMetadata(source, assetType, metadata)

	req := assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     assetType,
		Name:     strings.TrimSpace(opts.Name),
		Source:   source,
		Status:   status,
		Metadata: metadata,
	}

	if _, err := d.ProcessHeartbeatRequest(req); err != nil {
		logPrefix := strings.TrimSpace(opts.LogPrefix)
		if logPrefix == "" {
			logPrefix = "hub collector " + source
		}
		subject := strings.TrimSpace(opts.FailureSubject)
		if subject == "" {
			subject = "collector parent asset"
		}
		log.Printf("%s: failed to refresh %s %s: %v", logPrefix, subject, assetID, err)
		return connectorsdk.Asset{}, false
	}

	return ConnectorSnapshotAssetFromHeartbeat(req, ""), true
}

func ConnectorSnapshotAssetFromHeartbeat(req assets.HeartbeatRequest, kind string) connectorsdk.Asset {
	return connectorsdk.Asset{
		ID:       req.AssetID,
		Type:     req.Type,
		Name:     req.Name,
		Source:   req.Source,
		Kind:     strings.TrimSpace(kind),
		Metadata: cloneStringMap(req.Metadata),
	}
}
