package alerting

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
)

func newTestAlertingDeps(t *testing.T) *Deps {
	t.Helper()
	return &Deps{
		AlertStore:           testutil.NewAlertStore(),
		AlertInstanceStore:   testutil.NewAlertInstanceStore(),
		IncidentStore:        testutil.NewIncidentStore(),
		GroupStore:           testutil.NewGroupStore(),
		AssetStore:           testutil.NewAssetStore(),
		CanonicalStore:       testutil.NewCanonicalStore(),
		TelemetryStore:       testutil.NewTelemetryStore(),
		LogStore:             testutil.NewLogStore(),
		ActionStore:          testutil.NewActionStore(),
		UpdateStore:          testutil.NewUpdateStore(),
		AuditStore:           testutil.NewAuditStore(),
		NotificationAdapters: make(map[string]notifications.Adapter),
		NotificationSem:      make(chan struct{}, 32),
		NotificationWG:       &sync.WaitGroup{},
		EnforceRateLimit:     testutil.NoopRateLimit,
		WrapAuth:             testutil.NoopAuth,
		WrapAdmin:            testutil.NoopAuth,
	}
}

func TestHandleNotificationChannelActionsRejectsExtraPathSegments(t *testing.T) {
	deps := newTestAlertingDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/notifications/channels/channel-1/extra", nil)
	rec := httptest.NewRecorder()
	deps.HandleNotificationChannelActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for channel action path with extra segments, got %d", rec.Code)
	}
}

func TestValidateCreateRouteRequestRejectsUnsupportedGroupingAndRepeatSettings(t *testing.T) {
	err := ValidateCreateRouteRequest(notifications.CreateRouteRequest{
		Name:                  "Critical route",
		ChannelIDs:            []string{"chan-1"},
		GroupBy:               []string{"severity"},
		GroupWaitSeconds:      30,
		GroupIntervalSeconds:  300,
		RepeatIntervalSeconds: 3600,
	})
	if err == nil {
		t.Fatal("expected unsupported grouping settings to be rejected")
	}
}

func TestNotificationChannelTestIsRateLimitedBeforeDelivery(t *testing.T) {
	deps := newTestAlertingDeps(t)
	var gotBucket string
	var gotLimit int
	var gotWindow time.Duration
	deps.EnforceRateLimit = func(w http.ResponseWriter, _ *http.Request, bucket string, limit int, window time.Duration) bool {
		gotBucket = bucket
		gotLimit = limit
		gotWindow = window
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return false
	}

	req := httptest.NewRequest(http.MethodPost, "/notifications/channels/channel-1/test", nil)
	rec := httptest.NewRecorder()
	deps.HandleNotificationChannelTest(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("test status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if gotBucket != "notifications.channels.test" || gotLimit != 30 || gotWindow != time.Minute {
		t.Fatalf("rate limit = %q %d %s, want notifications.channels.test 30 1m", gotBucket, gotLimit, gotWindow)
	}
}
