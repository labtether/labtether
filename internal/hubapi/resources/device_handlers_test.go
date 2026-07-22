package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

type pushDeviceStoreStub struct {
	upserted  []persistence.PushDevice
	deleted   [][2]string
	upsertErr error
}

func (s *pushDeviceStoreStub) UpsertPushDevice(_ context.Context, device persistence.PushDevice) error {
	s.upserted = append(s.upserted, device)
	return s.upsertErr
}

func (s *pushDeviceStoreStub) DeletePushDevice(_ context.Context, userID, deviceID string) error {
	s.deleted = append(s.deleted, [2]string{userID, deviceID})
	return nil
}

func newPushDeviceHandlerDeps(store PushDeviceStore) *Deps {
	return &Deps{
		DB: store,
		DecodeJSONBody: func(_ http.ResponseWriter, r *http.Request, dst any) error {
			return json.NewDecoder(r.Body).Decode(dst)
		},
		UserIDFromContext: func(context.Context) string { return "user-1" },
	}
}

func TestHandleDeviceRegisterPersistsAPNsPreferences(t *testing.T) {
	store := &pushDeviceStoreStub{}
	deps := newPushDeviceHandlerDeps(store)
	body := `{
		"device_id":"device-1",
		"platform":"iOS",
		"push_token":"token-1",
		"bundle_id":"com.labtether.mobile.debug",
		"environment":"sandbox",
		"time_zone":"Australia/Sydney",
		"notify_critical_alerts":false,
		"notify_node_offline":true,
		"notify_service_down":false,
		"push_category":"alerts_and_incidents",
		"minimum_severity":"medium",
		"quiet_hours_enabled":true,
		"quiet_hours_start_minutes":1260,
		"quiet_hours_end_minutes":360,
		"digest_window_seconds":300
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/register", strings.NewReader(body))
	rec := httptest.NewRecorder()

	deps.HandleDeviceRegister(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.upserted) != 1 {
		t.Fatalf("upsert count = %d, want 1", len(store.upserted))
	}
	got := store.upserted[0]
	if got.UserID != "user-1" || got.Platform != "ios" || got.Environment != "sandbox" {
		t.Fatalf("unexpected identity/routing fields: %+v", got)
	}
	if got.TimeZone != "Australia/Sydney" {
		t.Fatalf("unexpected device timezone: %q", got.TimeZone)
	}
	if got.BundleID != "com.labtether.mobile.debug" || got.PushToken != "token-1" {
		t.Fatalf("unexpected APNs token fields: %+v", got)
	}
	if got.NotifyCriticalAlerts || !got.NotifyNodeOffline || got.NotifyServiceDown {
		t.Fatalf("unexpected notification toggles: %+v", got)
	}
	if got.PushCategory != "alerts_and_incidents" || got.MinimumSeverity != "warning" {
		t.Fatalf("unexpected category/severity normalization: %+v", got)
	}
	if !got.QuietHoursEnabled || got.QuietHoursStartMinutes != 1260 || got.QuietHoursEndMinutes != 360 || got.DigestWindowSeconds != 300 {
		t.Fatalf("unexpected quiet-hours/digest preferences: %+v", got)
	}
}

func TestHandleDeviceRegisterMapsPerUserCapacityToRateLimit(t *testing.T) {
	store := &pushDeviceStoreStub{upsertErr: persistence.ErrPushDeviceRegistrationLimit}
	deps := newPushDeviceHandlerDeps(store)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/devices/register",
		strings.NewReader(`{"device_id":"device-over-cap","push_token":"token-over-cap"}`),
	)
	rec := httptest.NewRecorder()

	deps.HandleDeviceRegister(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeviceRegisterAppliesBackwardCompatibleDefaults(t *testing.T) {
	store := &pushDeviceStoreStub{}
	deps := newPushDeviceHandlerDeps(store)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/devices/register",
		strings.NewReader(`{"device_id":"legacy-device","push_token":"legacy-token"}`),
	)
	rec := httptest.NewRecorder()

	deps.HandleDeviceRegister(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := store.upserted[0]
	if got.Platform != "ios" || got.Environment != "" {
		t.Fatalf("unexpected routing defaults: %+v", got)
	}
	if got.TimeZone != "" {
		t.Fatalf("legacy registration timezone = %q, want empty fail-open value", got.TimeZone)
	}
	if !got.NotifyCriticalAlerts || !got.NotifyNodeOffline || !got.NotifyServiceDown {
		t.Fatalf("expected notification toggles enabled by default: %+v", got)
	}
	if got.PushCategory != "critical_only" || got.MinimumSeverity != "warning" {
		t.Fatalf("unexpected preference defaults: %+v", got)
	}
	if got.QuietHoursStartMinutes != 1320 || got.QuietHoursEndMinutes != 420 || got.DigestWindowSeconds != 180 {
		t.Fatalf("unexpected quiet-hours/digest defaults: %+v", got)
	}
}

func TestHandleDeviceRegisterRejectsInvalidEnvironment(t *testing.T) {
	store := &pushDeviceStoreStub{}
	deps := newPushDeviceHandlerDeps(store)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/devices/register",
		strings.NewReader(`{"device_id":"device-1","push_token":"token-1","environment":"beta"}`),
	)
	rec := httptest.NewRecorder()

	deps.HandleDeviceRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(store.upserted) != 0 {
		t.Fatalf("invalid registration was persisted: %+v", store.upserted)
	}
}

func TestHandleDeviceRegisterRejectsInvalidTimeZone(t *testing.T) {
	store := &pushDeviceStoreStub{}
	deps := newPushDeviceHandlerDeps(store)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/devices/register",
		strings.NewReader(`{"device_id":"device-1","push_token":"token-1","time_zone":"Mars/Olympus_Mons"}`),
	)
	rec := httptest.NewRecorder()

	deps.HandleDeviceRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if len(store.upserted) != 0 {
		t.Fatalf("invalid timezone registration was persisted: %+v", store.upserted)
	}
}

func TestHandleDeviceRegisterRejectsOversizedOrControlCharacterIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "oversized device id",
			body: `{"device_id":"` + strings.Repeat("d", 129) + `","push_token":"token-1"}`,
		},
		{
			name: "control character token",
			body: `{"device_id":"device-1","push_token":"token\u0000value"}`,
		},
		{
			name: "unsupported platform",
			body: `{"device_id":"device-1","push_token":"token-1","platform":"browser"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &pushDeviceStoreStub{}
			deps := newPushDeviceHandlerDeps(store)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/register", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			deps.HandleDeviceRegister(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if len(store.upserted) != 0 {
				t.Fatalf("invalid registration was persisted: %+v", store.upserted)
			}
		})
	}
}

func TestHandleDeviceDeregisterSupportsLegacyPostBody(t *testing.T) {
	store := &pushDeviceStoreStub{}
	deps := newPushDeviceHandlerDeps(store)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/devices/deregister",
		strings.NewReader(`{"device_id":"device-legacy"}`),
	)
	rec := httptest.NewRecorder()

	deps.HandleDeviceDeregister(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != [2]string{"user-1", "device-legacy"} {
		t.Fatalf("unexpected deletes: %+v", store.deleted)
	}
}

func TestHandleDeviceRegisterSupportsCanonicalDelete(t *testing.T) {
	store := &pushDeviceStoreStub{}
	deps := newPushDeviceHandlerDeps(store)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/register?device_id=device-current", nil)
	rec := httptest.NewRecorder()

	deps.HandleDeviceRegister(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != [2]string{"user-1", "device-current"} {
		t.Fatalf("unexpected deletes: %+v", store.deleted)
	}
}
