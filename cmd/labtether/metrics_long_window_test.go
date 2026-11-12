package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A device chart may ask for up to 30 days. Before this, any window past 24h
// fell back to one hour without saying so, and a fine step on a long window
// would have returned tens of thousands of points per metric.
func TestAssetMetricsLongWindows(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-long","type":"host","name":"Lab Host Long","source":"agent","status":"online","platform":"linux","metadata":{"cpu_percent":"17.2"}}`)
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload)))
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("heartbeat: expected 202, got %d", createRec.Code)
	}

	cases := []struct {
		name       string
		query      string
		wantWindow time.Duration
		wantStep   time.Duration
	}{
		{"7 days", "window=168h", 7 * 24 * time.Hour, 10 * time.Minute},
		{"30 days", "window=720h", 30 * 24 * time.Hour, 30 * time.Minute},
		{"30 days with a fine step is coarsened", "window=720h&step=10s", 30 * 24 * time.Hour, 21*time.Minute + 36*time.Second},
		{"24 hours keeps the step it was given", "window=24h&step=10s", 24 * time.Hour, 10 * time.Second},
		{"past 30 days falls back to one hour", "window=721h", time.Hour, 15 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			sut.handleAssetMetrics(rec, httptest.NewRequest(http.MethodGet, "/metrics/assets/lab-host-long?"+tc.query, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Window string    `json:"window"`
				Step   string    `json:"step"`
				From   time.Time `json:"from"`
				To     time.Time `json:"to"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Window != tc.wantWindow.String() || body.Step != tc.wantStep.String() {
				t.Fatalf("window/step = %s/%s, want %s/%s", body.Window, body.Step, tc.wantWindow, tc.wantStep)
			}
			if got := body.To.Sub(body.From); got != tc.wantWindow {
				t.Fatalf("queried range = %s, want %s", got, tc.wantWindow)
			}
			if points := int(tc.wantWindow / tc.wantStep); tc.wantWindow > 24*time.Hour && points > 2000 {
				t.Fatalf("%d points per series, want at most 2000", points)
			}
		})
	}
}
