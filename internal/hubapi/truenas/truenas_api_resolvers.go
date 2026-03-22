package truenas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func WriteTrueNASResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTrueNASAssetNotFound), errors.Is(err, ErrAssetNotTrueNAS):
		servicehttp.WriteError(w, http.StatusNotFound, err.Error())
	default:
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
	}
}

func (d *Deps) ResolveTrueNASAsset(assetID string) (assets.Asset, error) {
	if d.AssetStore == nil {
		return assets.Asset{}, ErrTrueNASAssetNotFound
	}

	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return assets.Asset{}, ErrTrueNASAssetNotFound
	}

	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		return assets.Asset{}, fmt.Errorf("failed to load asset: %w", err)
	}
	if !ok {
		return assets.Asset{}, ErrTrueNASAssetNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(asset.Source), "truenas") {
		return assets.Asset{}, ErrAssetNotTrueNAS
	}
	return asset, nil
}

func (d *Deps) ResolveTrueNASAssetRuntime(assetID string) (assets.Asset, *TruenasRuntime, error) {
	asset, err := d.ResolveTrueNASAsset(assetID)
	if err != nil {
		return assets.Asset{}, nil, err
	}

	preferredCollectorID := strings.TrimSpace(asset.Metadata["collector_id"])
	runtime, loadErr := d.LoadTrueNASRuntime(preferredCollectorID)
	if loadErr != nil && preferredCollectorID != "" {
		runtime, loadErr = d.LoadTrueNASRuntime("")
	}
	if loadErr != nil {
		return assets.Asset{}, nil, fmt.Errorf("failed to load truenas runtime: %w", loadErr)
	}
	return asset, runtime, nil
}

func ParseTrueNASEventsWindow(raw string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 24 * time.Hour
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 24 * time.Hour
	}
	if parsed < time.Hour {
		return time.Hour
	}
	if parsed > 30*24*time.Hour {
		return 30 * 24 * time.Hour
	}
	return parsed
}

func CallTrueNASMethodWithRetries(ctx context.Context, client *tnconnector.Client, method string, params []any, dest any) error {
	var err error
	for attempt := 0; attempt < TrueNASMethodCallRetryAttempts; attempt++ {
		err = client.Call(ctx, method, params, dest)
		if err == nil || !tnconnector.IsMethodCallError(err) {
			return err
		}
		if !WaitForTrueNASMethodRetry(ctx, attempt) {
			break
		}
	}
	return err
}

func StaleTrueNASReadWarning(message, fetchedAt string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "live truenas data unavailable"
	}
	fetchedAt = strings.TrimSpace(fetchedAt)
	if fetchedAt == "" {
		return message + " (showing cached data)"
	}
	return message + " (showing cached data from " + fetchedAt + ")"
}

func AppendTrueNASWarning(existing []string, warning string) []string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return existing
	}
	for _, current := range existing {
		if strings.EqualFold(strings.TrimSpace(current), warning) {
			return existing
		}
	}
	return append(existing, warning)
}

func WaitForTrueNASMethodRetry(ctx context.Context, attempt int) bool {
	if attempt >= TrueNASMethodCallRetryAttempts-1 {
		return false
	}
	delay := TrueNASMethodCallRetryBackoff * time.Duration(attempt+1)
	if delay <= 0 {
		return true
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func CallTrueNASQueryCompat(ctx context.Context, client *tnconnector.Client, method string, dest any) error {
	err := client.Call(ctx, method, nil, dest)
	if err == nil || !tnconnector.IsMethodCallError(err) {
		return err
	}

	retryParams := [][]any{
		{[]any{}, map[string]any{}},
		{[]any{}},
	}
	retryErr := err
	for _, params := range retryParams {
		retryErr = client.Call(ctx, method, params, dest)
		if retryErr == nil {
			return nil
		}
		if !tnconnector.IsMethodCallError(retryErr) {
			return retryErr
		}
	}
	return retryErr
}

func CallTrueNASQueryWithRetries(ctx context.Context, client *tnconnector.Client, method string, dest any) error {
	var err error
	for attempt := 0; attempt < TrueNASMethodCallRetryAttempts; attempt++ {
		err = CallTrueNASQueryCompat(ctx, client, method, dest)
		if err == nil || !tnconnector.IsMethodCallError(err) {
			return err
		}
		if !WaitForTrueNASMethodRetry(ctx, attempt) {
			break
		}
	}
	return err
}

func LatestSmartResultsByDisk(results []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(results))
	for _, entry := range results {
		diskName := ""
		switch diskValue := entry["disk"].(type) {
		case map[string]any:
			diskName = strings.TrimSpace(shared.CollectorAnyString(diskValue["name"]))
		default:
			diskName = strings.TrimSpace(shared.CollectorAnyString(diskValue))
		}
		if diskName == "" {
			continue
		}

		current, exists := out[diskName]
		if !exists {
			out[diskName] = entry
			continue
		}
		entryTime, entryOK := shared.ParseAnyTimestamp(entry["created_at"])
		if !entryOK {
			entryTime, entryOK = shared.ParseAnyTimestamp(entry["end_time"])
		}
		currentTime, currentOK := shared.ParseAnyTimestamp(current["created_at"])
		if !currentOK {
			currentTime, currentOK = shared.ParseAnyTimestamp(current["end_time"])
		}

		if entryOK && (!currentOK || entryTime.After(currentTime)) {
			out[diskName] = entry
		}
	}
	return out
}

func DeriveTrueNASDiskHealthStatus(view TrueNASDiskHealth) string {
	health := strings.ToLower(strings.TrimSpace(view.SmartHealth))
	testStatus := strings.ToLower(strings.TrimSpace(view.LastTestStatus))

	if strings.Contains(testStatus, "fail") || strings.Contains(testStatus, "error") || strings.Contains(testStatus, "abort") {
		return "critical"
	}
	if strings.Contains(health, "fail") || strings.Contains(health, "critical") || strings.Contains(health, "fault") {
		return "critical"
	}
	if view.TemperatureCelsius != nil && *view.TemperatureCelsius >= 60 {
		return "critical"
	}
	if strings.Contains(health, "warn") || strings.Contains(health, "degrad") || strings.Contains(testStatus, "warn") {
		return "warning"
	}
	if view.TemperatureCelsius != nil && *view.TemperatureCelsius >= 50 {
		return "warning"
	}
	if view.SmartEnabled != nil && !*view.SmartEnabled {
		return "warning"
	}
	if health != "" || testStatus != "" || view.TemperatureCelsius != nil {
		return "healthy"
	}
	return "unknown"
}

func TrueNASDiskHealthSeverity(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "critical":
		return 4
	case "warning":
		return 3
	case "unknown":
		return 2
	case "healthy":
		return 1
	default:
		return 0
	}
}
