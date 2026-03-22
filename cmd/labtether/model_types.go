package main

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/retention"
)

// Thin type aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

type telemetrySnapshot = shared.TelemetrySnapshot
type assetTelemetryOverview = shared.AssetTelemetryOverview
type retentionSettingsRequest = shared.RetentionSettingsRequest
type retentionSettingsResponse = shared.RetentionSettingsResponse
type retentionPresetResponse = shared.RetentionPresetResponse
type runtimeSettingsUpdateRequest = shared.RuntimeSettingsUpdateRequest
type runtimeSettingsResetRequest = shared.RuntimeSettingsResetRequest
type runtimeSettingEntry = shared.RuntimeSettingEntry
type runtimeSettingsPayload = shared.RuntimeSettingsPayload

func formatRetentionSettings(settings retention.Settings) retentionSettingsResponse {
	return shared.FormatRetentionSettings(settings)
}

func formatRetentionPresets() []retentionPresetResponse {
	return shared.FormatRetentionPresets()
}
