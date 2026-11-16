package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/notifications"
)

// TestAlertingRoutesReachHandlersThroughRouteTable sends one signed-in admin
// request to each alerting route through the real route table and auth, and
// checks the answer only that route's handler gives.
//
// The alerting handler tests live in internal/hubapi/alerting and build
// their Deps with testutil/hubdeps. This test catches a fault in cmd's own
// wiring that those tests cannot see: a route bound to the wrong handler, or
// buildAlertingDeps leaving out a store (rules, instances and silences,
// incidents, notifications, live activities). Channels and routes are put
// in the store, not created through their routes, so no channel address is
// checked or sent to.
func TestAlertingRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	// Production fills the notification store from Postgres; the test server has none.
	store := newNotificationStoreStub()
	store.channels["chan-wiring"] = notifications.Channel{ID: "chan-wiring", Name: "Wiring hook", Type: notifications.ChannelTypeWebhook, Enabled: true}
	store.routes["route-wiring"] = notifications.Route{ID: "route-wiring", Name: "Wiring route", ChannelIDs: []string{"chan-wiring"}, Enabled: true}
	store.records = append(store.records, notifications.Record{ID: "rec-wiring", ChannelID: "chan-wiring"})
	sut.notificationStore = store
	admin := createRoleSessionToken(t, sut, "admin-alerting-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/alerts/templates", http.MethodGet, "/alerts/templates", "", http.StatusOK, `"id":"starter.agent_offline"`)
	rule := call("/alerts/templates/", http.MethodPost, "/alerts/templates/starter.error_burst/enable", `{"name":"Wiring rule"}`, http.StatusCreated, `"name":"Wiring rule"`)
	var enabled struct {
		Rule struct {
			ID string `json:"id"`
		} `json:"rule"`
	}
	if err := json.Unmarshal([]byte(rule), &enabled); err != nil || enabled.Rule.ID == "" {
		t.Fatalf("no rule id in %s (err %v)", rule, err)
	}
	ruleID := enabled.Rule.ID
	call("/alerts/rules", http.MethodGet, "/alerts/rules", "", http.StatusOK, `"name":"Wiring rule"`)
	call("/alerts/rules/", http.MethodGet, "/alerts/rules/"+ruleID, "", http.StatusOK, `"name":"Wiring rule"`)

	instance, err := sut.alertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: ruleID, Fingerprint: "wiring-fp", Severity: alerts.SeverityHigh})
	if err != nil {
		t.Fatalf("seed alert instance: %v", err)
	}
	call("/alerts/instances", http.MethodGet, "/alerts/instances", "", http.StatusOK, `"fingerprint":"wiring-fp"`)
	call("/alerts/instances/", http.MethodGet, "/alerts/instances/"+instance.ID, "", http.StatusOK, `"fingerprint":"wiring-fp"`)
	silence, err := sut.alertInstanceStore.CreateAlertSilence(alerts.CreateSilenceRequest{
		Matchers: map[string]string{"env": "lab"}, Reason: "wiring window", StartsAt: time.Now().UTC(), EndsAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed silence: %v", err)
	}
	call("/alerts/silences", http.MethodGet, "/alerts/silences", "", http.StatusOK, `"reason":"wiring window"`)
	call("/alerts/silences/", http.MethodGet, "/alerts/silences/"+silence.ID, "", http.StatusOK, `"reason":"wiring window"`)

	call("/alerts/routes", http.MethodGet, "/alerts/routes", "", http.StatusOK, `"name":"Wiring route"`)
	call("/alerts/routes/", http.MethodGet, "/alerts/routes/route-wiring", "", http.StatusOK, `"name":"Wiring route"`)
	call("/notifications/channels", http.MethodGet, "/notifications/channels", "", http.StatusOK, `"name":"Wiring hook"`)
	call("/notifications/channels/", http.MethodGet, "/notifications/channels/chan-wiring", "", http.StatusOK, `"name":"Wiring hook"`)
	call("/notifications/history", http.MethodGet, "/notifications/history", "", http.StatusOK, `"id":"rec-wiring"`)

	incident := call("/incidents", http.MethodPost, "/incidents", `{"title":"Wiring incident","severity":"high"}`, http.StatusCreated, `"title":"Wiring incident"`)
	incidentID := routeWiringID(t, incident, "incident")
	call("/incidents/", http.MethodGet, "/incidents/"+incidentID, "", http.StatusOK, `"title":"Wiring incident"`)
	// The test server's live activity store is its nil Postgres store. This
	// request is refused on its path, before the store is read.
	call("/live-activities/incidents/", http.MethodPost, "/live-activities/incidents/"+incidentID, "", http.StatusNotFound, "live activity path not found")
}
