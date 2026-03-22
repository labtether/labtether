package agentcore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stubProvider implements TelemetryProvider for testing.
type stubProvider struct {
	sample TelemetrySample
}

func (s stubProvider) Collect(_ time.Time) (TelemetrySample, error) {
	return s.sample, nil
}

func (s stubProvider) StaticMetadata() map[string]string {
	return map[string]string{"os": "darwin"}
}

func (s stubProvider) AgentInfo() AgentInfo {
	return AgentInfo{OS: "darwin", Mode: "agent", Status: "ok"}
}

func TestStatusEndpoint(t *testing.T) {
	t.Parallel()

	provider := stubProvider{
		sample: TelemetrySample{
			AssetID:       "node-42",
			CPUPercent:    55.5,
			MemoryPercent: 72.3,
			DiskPercent:   40.1,
			CollectedAt:   time.Now().UTC(),
		},
	}

	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		GroupID:           "group-1",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})
	// Pre-populate telemetry so status returns non-zero metrics.
	rt.collectOnce(time.Now().UTC())

	handler := rt.statusHandler()
	req := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var resp StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.AgentName != "test-agent" {
		t.Fatalf("expected agent_name %q, got %q", "test-agent", resp.AgentName)
	}
	if resp.AssetID != "node-42" {
		t.Fatalf("expected asset_id %q, got %q", "node-42", resp.AssetID)
	}
	if resp.GroupID != "group-1" {
		t.Fatalf("expected group_id %q, got %q", "group-1", resp.GroupID)
	}
	if resp.Port != "9100" {
		t.Fatalf("expected port %q, got %q", "9100", resp.Port)
	}
	if resp.Connected {
		t.Fatal("expected connected=false when transport is nil")
	}
	if resp.Uptime == "" {
		t.Fatal("expected non-empty uptime")
	}
	if resp.StartedAt.IsZero() {
		t.Fatal("expected non-zero started_at")
	}
	if resp.Metrics.CPUPercent != 55.5 {
		t.Fatalf("expected cpu_percent 55.5, got %f", resp.Metrics.CPUPercent)
	}
	if resp.Metrics.MemoryPercent != 72.3 {
		t.Fatalf("expected memory_percent 72.3, got %f", resp.Metrics.MemoryPercent)
	}
	if resp.Alerts == nil {
		t.Fatal("expected non-nil alerts slice")
	}
	if len(resp.Alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(resp.Alerts))
	}
}

func TestStatusEndpointWithAlerts(t *testing.T) {
	t.Parallel()

	provider := stubProvider{}
	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})

	rt.pushAlert(AlertSnapshot{
		ID:       "alert-1",
		Severity: "critical",
		Title:    "High CPU",
		Summary:  "CPU usage above 90%",
		State:    "firing",
	})

	handler := rt.statusHandler()
	req := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(resp.Alerts))
	}
	if resp.Alerts[0].ID != "alert-1" {
		t.Fatalf("expected alert ID %q, got %q", "alert-1", resp.Alerts[0].ID)
	}
	if resp.Alerts[0].Severity != "critical" {
		t.Fatalf("expected severity %q, got %q", "critical", resp.Alerts[0].Severity)
	}
}

func TestAlertRelay(t *testing.T) {
	t.Parallel()

	provider := stubProvider{}
	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})

	// getAlerts on fresh runtime should return empty slice, not nil.
	alerts := rt.getAlerts()
	if alerts == nil {
		t.Fatal("expected non-nil empty slice from getAlerts")
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(alerts))
	}

	// Push some alerts.
	rt.pushAlert(AlertSnapshot{ID: "a1", Severity: "warning", Title: "Disk space low"})
	rt.pushAlert(AlertSnapshot{ID: "a2", Severity: "critical", Title: "CPU high"})

	alerts = rt.getAlerts()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	if alerts[0].ID != "a1" {
		t.Fatalf("expected first alert ID %q, got %q", "a1", alerts[0].ID)
	}
	if alerts[1].ID != "a2" {
		t.Fatalf("expected second alert ID %q, got %q", "a2", alerts[1].ID)
	}

	// Upsert: update existing alert by ID.
	rt.pushAlert(AlertSnapshot{ID: "a1", Severity: "critical", Title: "Disk space critical"})
	alerts = rt.getAlerts()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts after upsert, got %d", len(alerts))
	}
	if alerts[0].Title != "Disk space critical" {
		t.Fatalf("expected upserted title %q, got %q", "Disk space critical", alerts[0].Title)
	}

	// Returned slice should be a copy; modifying it should not affect the runtime.
	alerts[0].Title = "mutated"
	original := rt.getAlerts()
	if original[0].Title == "mutated" {
		t.Fatal("getAlerts returned a reference instead of a copy")
	}
}

func TestAlertRelayMaxCap(t *testing.T) {
	t.Parallel()

	provider := stubProvider{}
	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})

	// Push 25 distinct alerts.
	for i := 0; i < 25; i++ {
		rt.pushAlert(AlertSnapshot{
			ID:    fmt.Sprintf("alert-%d", i),
			Title: fmt.Sprintf("Alert %d", i),
		})
	}

	alerts := rt.getAlerts()
	if len(alerts) != maxCachedAlerts {
		t.Fatalf("expected %d alerts (max cap), got %d", maxCachedAlerts, len(alerts))
	}

	// The oldest 5 should have been dropped (alert-0 through alert-4).
	if alerts[0].ID != "alert-5" {
		t.Fatalf("expected first alert after cap to be %q, got %q", "alert-5", alerts[0].ID)
	}
	if alerts[maxCachedAlerts-1].ID != "alert-24" {
		t.Fatalf("expected last alert to be %q, got %q", "alert-24", alerts[maxCachedAlerts-1].ID)
	}
}

func TestStatusEndpointConnectionState(t *testing.T) {
	t.Parallel()

	provider := stubProvider{}
	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})

	handler := rt.statusHandler()
	req := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// No transport → disconnected state.
	if resp.ConnectionState != "disconnected" {
		t.Fatalf("expected connection_state %q, got %q", "disconnected", resp.ConnectionState)
	}
	if resp.DisconnectedAt != nil {
		t.Fatalf("expected disconnected_at to be nil when no transport, got %v", resp.DisconnectedAt)
	}
	if resp.LastError != "" {
		t.Fatalf("expected last_error to be empty, got %q", resp.LastError)
	}
}

func TestStatusEndpointWithTransport(t *testing.T) {
	t.Parallel()

	provider := stubProvider{}
	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})

	// Create a transport and set its state to simulate different connection states.
	transport := newWSTransport("wss://hub.example.com/ws/agent", "tok", "node-42", "darwin", "v1.2.3", nil, "", nil)
	rt.transport = transport

	// Case 1: transport exists but not connected → "disconnected"
	handler := rt.statusHandler()
	req := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("case 1: failed to decode: %v", err)
	}
	if resp.Connected {
		t.Fatal("case 1: expected connected=false")
	}
	if resp.ConnectionState != "disconnected" {
		t.Fatalf("case 1: expected connection_state %q, got %q", "disconnected", resp.ConnectionState)
	}

	// Case 2: simulate connected state
	transport.mu.Lock()
	transport.connected = true
	transport.mu.Unlock()

	req = httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("case 2: failed to decode: %v", err)
	}
	if !resp.Connected {
		t.Fatal("case 2: expected connected=true")
	}
	if resp.ConnectionState != "connected" {
		t.Fatalf("case 2: expected connection_state %q, got %q", "connected", resp.ConnectionState)
	}

	// Case 3: simulate auth failure
	discTime := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	transport.mu.Lock()
	transport.connected = false
	transport.lastError = "auth_failed"
	transport.disconnectedAt = discTime
	transport.mu.Unlock()

	req = httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("case 3: failed to decode: %v", err)
	}
	if resp.Connected {
		t.Fatal("case 3: expected connected=false")
	}
	if resp.ConnectionState != "auth_failed" {
		t.Fatalf("case 3: expected connection_state %q, got %q", "auth_failed", resp.ConnectionState)
	}
	if resp.LastError != "auth_failed" {
		t.Fatalf("case 3: expected last_error %q, got %q", "auth_failed", resp.LastError)
	}
	if resp.DisconnectedAt == nil {
		t.Fatal("case 3: expected disconnected_at to be set")
	}
	if !resp.DisconnectedAt.Equal(discTime) {
		t.Fatalf("case 3: expected disconnected_at %v, got %v", discTime, *resp.DisconnectedAt)
	}

	// Case 4: simulate reconnecting (transient error)
	transport.mu.Lock()
	transport.connected = false
	transport.lastError = "transient"
	transport.disconnectedAt = discTime
	transport.mu.Unlock()

	req = httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("case 4: failed to decode: %v", err)
	}
	if resp.ConnectionState != "connecting" {
		t.Fatalf("case 4: expected connection_state %q, got %q", "connecting", resp.ConnectionState)
	}
}

func TestStatusEndpointSupportsConditionalRequests(t *testing.T) {
	t.Parallel()

	provider := stubProvider{
		sample: TelemetrySample{
			AssetID:       "node-42",
			CPUPercent:    22.2,
			MemoryPercent: 44.4,
			DiskPercent:   55.5,
			CollectedAt:   time.Now().UTC(),
		},
	}
	cfg := RuntimeConfig{
		Name:              "test-agent",
		Port:              "9100",
		AssetID:           "node-42",
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
	rt := NewRuntime(cfg, provider, noopHeartbeatPublisher{})
	rt.collectOnce(time.Now().UTC())

	handler := rt.statusHandler()

	firstReq := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected warm request status 200, got %d", firstRec.Code)
	}
	etag := firstRec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected status response to include ETag")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	secondReq.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for matching If-None-Match, got %d", secondRec.Code)
	}
	if got := secondRec.Header().Get("ETag"); got == "" {
		t.Fatal("expected 304 response to include ETag")
	}

	rt.pushAlert(AlertSnapshot{
		ID:        "alert-1",
		Severity:  "critical",
		Title:     "High CPU",
		Summary:   "CPU usage above threshold",
		State:     "firing",
		Timestamp: time.Now().UTC(),
	})

	thirdReq := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	thirdReq.Header.Set("If-None-Match", etag)
	thirdRec := httptest.NewRecorder()
	handler.ServeHTTP(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusOK {
		t.Fatalf("expected 200 after payload change, got %d", thirdRec.Code)
	}
	if got := thirdRec.Header().Get("ETag"); got == "" || got == etag {
		t.Fatalf("expected changed ETag after payload mutation, got %q", got)
	}
}

func TestStatusETagMatches(t *testing.T) {
	t.Parallel()

	const etag = `"abc123"`
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "exact", header: `"abc123"`, want: true},
		{name: "weak", header: `W/"abc123"`, want: true},
		{name: "multiple", header: `"other", W/"abc123"`, want: true},
		{name: "wildcard", header: `*`, want: true},
		{name: "mismatch", header: `"other"`, want: false},
		{name: "empty", header: ``, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := statusETagMatches(tc.header, etag)
			if got != tc.want {
				t.Fatalf("statusETagMatches(%q, %q)=%v, want %v", tc.header, etag, got, tc.want)
			}
		})
	}
}
