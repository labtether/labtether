package hubdeps

import (
	"github.com/labtether/labtether/internal/hubapi/pbs"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// BuildPBSDeps mirrors cmd's buildPBSDeps field by field. Stand-ins replace
// only cmd's middleware (route auth and the admin check); see
// testutil/standins.go for what each one still refuses.
func (h *Hub) BuildPBSDeps() *pbs.Deps {
	return &pbs.Deps{
		AssetStore:        h.AssetStore,
		HubCollectorStore: h.HubCollectorStore,
		CredentialStore:   h.CredentialStore,
		SecretsManager:    h.SecretsManager,

		RequireAdminAuth:            testutil.RequireAdminRole,
		EvaluateConnectorGuardrails: h.ConnectorMaintenanceEvaluator(),

		WrapAuth:  testutil.RouteAuthNotTested(h.t),
		WrapAdmin: testutil.RouteAuthNotTested(h.t),
	}
}

// PBSDeps mirrors cmd's ensurePBSDeps: one Deps for the life of the Hub (so
// its runtime cache is kept between calls), with the same fields refreshed
// from the Hub's current stores on every call. As in cmd, the maintenance
// evaluator is not refreshed: it keeps the stores of the first call.
func (h *Hub) PBSDeps() *pbs.Deps {
	h.pbsOnce.Do(func() { h.pbsDeps = h.BuildPBSDeps() })
	d := h.pbsDeps
	d.AssetStore = h.AssetStore
	d.HubCollectorStore = h.HubCollectorStore
	d.CredentialStore = h.CredentialStore
	d.SecretsManager = h.SecretsManager
	d.RequireAdminAuth = testutil.RequireAdminRole
	return d
}
