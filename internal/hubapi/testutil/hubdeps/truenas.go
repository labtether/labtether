package hubdeps

import (
	"time"

	"github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/truenas"
)

// BuildTrueNASDeps mirrors cmd's buildTruenasDeps field by field. Stand-ins
// replace only cmd's middleware (route auth, the admin check and the
// WebSocket origin check); see testutil/standins.go for what each one still
// refuses. The connector log callbacks are the real collectors ones.
func (h *Hub) BuildTrueNASDeps() *truenas.Deps {
	return &truenas.Deps{
		AssetStore:        h.AssetStore,
		HubCollectorStore: h.HubCollectorStore,
		CredentialStore:   h.CredentialStore,
		SecretsManager:    h.SecretsManager,
		LogStore:          h.LogStore,

		TruenasSubs: make(map[string]truenas.TruenasSubscriptionHandle, 8),

		RequireAdminAuth:            testutil.RequireAdminRole,
		EvaluateConnectorGuardrails: h.ConnectorMaintenanceEvaluator(),

		WrapAuth:  testutil.RouteAuthNotTested(h.t),
		WrapAdmin: testutil.RouteAuthNotTested(h.t),

		CheckSameOrigin: testutil.CheckNoBrowserOrigin,

		AppendConnectorLogEvent:       h.appendConnectorLogEvent,
		AppendConnectorLogEventWithID: h.appendConnectorLogEventWithID,

		StartBrowserWebSocketKeepalive:    shared.StartBrowserWebSocketKeepalive,
		TouchBrowserWebSocketReadDeadline: shared.TouchBrowserWebSocketReadDeadline,
	}
}

// TrueNASDeps mirrors cmd's ensureTruenasDeps: one Deps for the life of the
// Hub (so its runtime, read caches and subscription workers are kept between
// calls), with the same fields refreshed from the Hub's current stores on
// every call.
func (h *Hub) TrueNASDeps() *truenas.Deps {
	h.truenasOnce.Do(func() { h.truenasDeps = h.BuildTrueNASDeps() })
	d := h.truenasDeps
	d.AssetStore = h.AssetStore
	d.HubCollectorStore = h.HubCollectorStore
	d.CredentialStore = h.CredentialStore
	d.SecretsManager = h.SecretsManager
	d.LogStore = h.LogStore
	d.RequireAdminAuth = testutil.RequireAdminRole
	d.EvaluateConnectorGuardrails = h.ConnectorMaintenanceEvaluator()
	return d
}

// appendConnectorLogEvent and appendConnectorLogEventWithID call the real
// collectors code, as cmd's methods of the same names do. cmd's collectors
// Deps keeps the log store it had when first built; these read the Hub's
// log store at each call.
func (h *Hub) appendConnectorLogEvent(assetID, source, level, message string, fields map[string]string, at time.Time) {
	(&collectors.Deps{LogStore: h.LogStore}).AppendConnectorLogEvent(assetID, source, level, message, fields, at)
}

func (h *Hub) appendConnectorLogEventWithID(eventID, assetID, source, level, message string, fields map[string]string, at time.Time) {
	(&collectors.Deps{LogStore: h.LogStore}).AppendConnectorLogEventWithID(eventID, assetID, source, level, message, fields, at)
}
