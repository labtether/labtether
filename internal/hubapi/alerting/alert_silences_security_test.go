package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apiv2"
)

func TestValidateSilenceRequestBoundsMatchers(t *testing.T) {
	now := time.Now().UTC()
	valid := alerts.CreateSilenceRequest{
		Matchers: map[string]string{"severity": "critical"},
		StartsAt: now,
		EndsAt:   now.Add(time.Hour),
	}
	if err := ValidateSilenceRequest(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	tests := []struct {
		name     string
		matchers map[string]string
		want     string
	}{
		{name: "blank key", matchers: map[string]string{"  ": "critical"}, want: "labels cannot be empty"},
		{name: "long key", matchers: map[string]string{strings.Repeat("k", MaxSilenceMatcherKeyLen+1): "critical"}, want: "labels must be"},
		{name: "long value", matchers: map[string]string{"severity": strings.Repeat("v", MaxSilenceMatcherValLen+1)}, want: "values must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			req.Matchers = tt.matchers
			err := ValidateSilenceRequest(req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q validation error, got %v", tt.want, err)
			}
		})
	}

	tooMany := make(map[string]string, MaxSilenceMatcherCount+1)
	for i := 0; i <= MaxSilenceMatcherCount; i++ {
		tooMany[string(rune('a'+i))] = "value"
	}
	req := valid
	req.Matchers = tooMany
	if err := ValidateSilenceRequest(req); err == nil || !strings.Contains(err.Error(), "no more than") {
		t.Fatalf("expected matcher-count validation error, got %v", err)
	}
}

func TestCreateAlertSilenceUsesAuthenticatedActor(t *testing.T) {
	deps := newTestAlertingDeps(t)
	now := time.Now().UTC()
	payload, err := json.Marshal(alerts.CreateSilenceRequest{
		Matchers:  map[string]string{"severity": "critical"},
		Reason:    "planned maintenance",
		CreatedBy: "spoofed-actor",
		StartsAt:  now,
		EndsAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/alerts/silences", bytes.NewReader(payload))
	req = req.WithContext(apiv2.ContextWithPrincipal(context.Background(), "operator-1", "operator"))
	rec := httptest.NewRecorder()
	deps.HandleAlertSilences(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Silence alerts.AlertSilence `json:"silence"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Silence.CreatedBy != "operator-1" {
		t.Fatalf("created_by=%q, want authenticated actor", response.Silence.CreatedBy)
	}
}

func TestCreateAlertRuleUsesAuthenticatedActor(t *testing.T) {
	deps := newTestAlertingDeps(t)
	payload := []byte(`{
		"name":"CPU high",
		"kind":"metric_threshold",
		"severity":"high",
		"target_scope":"global",
		"condition":{"metric":"cpu_usage","operator":">","threshold":90},
		"created_by":"spoofed-actor"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/alerts/rules", bytes.NewReader(payload))
	req = req.WithContext(apiv2.ContextWithPrincipal(context.Background(), "operator-1", "operator"))
	rec := httptest.NewRecorder()
	deps.HandleAlertRules(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Rule.CreatedBy != "operator-1" {
		t.Fatalf("created_by=%q, want authenticated actor", response.Rule.CreatedBy)
	}
}
