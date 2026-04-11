package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/terminal"
)

// resetTerminalDepsForTest clears the cached terminal deps and resets the
// sync.Once guard so the next ensureTerminalDeps call rebuilds from the
// current apiServer fields. Tests use this when they swap stores or policy
// mid-run and need the change reflected.
func (s *apiServer) resetTerminalDepsForTest() {
	s.terminalDeps = nil
	s.terminalDepsOnce = sync.Once{}
}

// resetDesktopDepsForTest mirrors resetTerminalDepsForTest for desktopDeps.
func (s *apiServer) resetDesktopDepsForTest() {
	s.desktopDeps = nil
	s.desktopDepsOnce = sync.Once{}
}

func mustCreateSession(t *testing.T, sut *apiServer) string {
	t.Helper()

	payload := []byte(`{"actor_id":"owner","target":"lab-host-01","mode":"interactive"}`)
	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating session, got %d", rec.Code)
	}

	var response struct {
		Session terminal.Session `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode session response: %v", err)
	}
	if response.Session.ID == "" {
		t.Fatalf("expected session ID")
	}

	return response.Session.ID
}

func newTestAPIServer(t *testing.T) *apiServer {
	t.Helper()

	terminalStore := persistence.NewMemoryTerminalStore()
	auditStore := persistence.NewMemoryAuditStore()
	assetStore := persistence.NewMemoryAssetStore()
	canonicalStore := persistence.NewMemoryCanonicalModelStore()
	telemetryStore := persistence.NewMemoryTelemetryStore()
	logStore := persistence.NewMemoryLogStore()
	actionStore := persistence.NewMemoryActionStore()
	updateStore := persistence.NewMemoryUpdateStore()
	alertStore := persistence.NewMemoryAlertStore()
	alertInstanceStore := persistence.NewMemoryAlertInstanceStore()
	incidentStore := persistence.NewMemoryIncidentStore()
	retentionStore := persistence.NewMemoryRetentionStore()
	runtimeStore := persistence.NewMemoryRuntimeSettingsStore()
	credentialStore := persistence.NewMemoryCredentialStore()
	_, _ = retentionStore.SaveRetentionSettings(retention.DefaultSettings())
	secretsManager, err := secrets.NewManagerFromEncodedKey("MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=")
	if err != nil {
		t.Fatalf("failed to initialize test secrets manager: %v", err)
	}

	groupStore := persistence.NewMemoryGroupStore()
	syntheticStore := persistence.NewMemorySyntheticStore()
	enrollmentStore := persistence.NewMemoryEnrollmentStore()
	adminResetStore := persistence.NewMemoryAdminResetStore()
	linkSuggestionStore := persistence.NewMemoryLinkSuggestionStore()
	authStore := persistence.NewMemoryAuthStore()

	sut := &apiServer{
		terminalStore:           terminalStore,
		terminalPersistentStore: terminalStore,
		auditStore:              auditStore,
		assetStore:              assetStore,
		groupStore:              groupStore,
		credentialStore:         credentialStore,
		canonicalStore:          canonicalStore,
		telemetryStore:          telemetryStore,
		logStore:                logStore,
		actionStore:             actionStore,
		updateStore:             updateStore,
		alertStore:              alertStore,
		alertInstanceStore:      alertInstanceStore,
		incidentStore:           incidentStore,
		retentionStore:          retentionStore,
		runtimeStore:            runtimeStore,
		syntheticStore:          syntheticStore,
		enrollmentStore:         enrollmentStore,
		adminResetStore:         adminResetStore,
		apiKeyStore:             persistence.NewMemoryAPIKeyStore(),
		scheduleStore:           persistence.NewMemoryScheduleStore(),
		webhookStore:            persistence.NewMemoryWebhookStore(),
		savedActionStore:        persistence.NewMemorySavedActionStore(),
		linkSuggestionStore:     linkSuggestionStore,
		authStore:               authStore,
		secretsManager:          secretsManager,
		policyState:             newPolicyRuntimeState(policy.DefaultEvaluatorConfig()),
		connectorRegistry:       connectorsdk.NewRegistry(),
		notificationDispatcher: NotificationDispatcher{
			Adapters:    defaultNotificationAdapters(),
			DispatchSem: make(chan struct{}, 32),
		},
		collectorDispatchSem: make(chan struct{}, 4),
		pendingAgents:        newPendingAgents(),
		installStateStore:    installstate.New(filepath.Join(t.TempDir(), "install")),
	}
	sut.webServiceCoordinator = webservice.NewCoordinator(persistence.NewMemoryWebServiceStore())
	// proxmoxDeps is lazy-initialized by ensureProxmoxDeps() on first use,
	// allowing tests to modify stores before the deps are built.

	// Ensure groupmaintenance created in tests are always removed during teardown, mirroring
	// smoke/integration cleanup behavior for created resources.
	t.Cleanup(func() {
		if sut.alertInstanceStore != nil {
			silences, err := sut.alertInstanceStore.ListAlertSilences(1000, false)
			if err != nil {
				t.Errorf("cleanup: failed to list alert silences: %v", err)
			} else {
				for _, silence := range silences {
					if delErr := sut.alertInstanceStore.DeleteAlertSilence(silence.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete alert silence %s: %v", silence.ID, delErr)
					}
				}
			}
		}

		if sut.alertStore != nil {
			rules, err := sut.alertStore.ListAlertRules(persistence.AlertRuleFilter{Limit: 1000})
			if err != nil {
				t.Errorf("cleanup: failed to list alert rules: %v", err)
			} else {
				for _, rule := range rules {
					if delErr := sut.alertStore.DeleteAlertRule(rule.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete alert rule %s: %v", rule.ID, delErr)
					}
				}
			}
		}

		if sut.updateStore != nil {
			runs, err := sut.updateStore.ListUpdateRuns(1000, "")
			if err != nil {
				t.Errorf("cleanup: failed to list update runs: %v", err)
			} else {
				for _, run := range runs {
					if delErr := sut.updateStore.DeleteUpdateRun(run.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete update run %s: %v", run.ID, delErr)
					}
				}
			}

			plans, err := sut.updateStore.ListUpdatePlans(1000)
			if err != nil {
				t.Errorf("cleanup: failed to list update plans: %v", err)
			} else {
				for _, plan := range plans {
					if delErr := sut.updateStore.DeleteUpdatePlan(plan.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete update plan %s: %v", plan.ID, delErr)
					}
				}
			}
		}

		if sut.actionStore != nil {
			runs, err := sut.actionStore.ListActionRuns(1000, 0, "", "")
			if err != nil {
				t.Errorf("cleanup: failed to list action runs: %v", err)
			} else {
				for _, run := range runs {
					if delErr := sut.actionStore.DeleteActionRun(run.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete action run %s: %v", run.ID, delErr)
					}
				}
			}
		}

		if sut.syntheticStore != nil {
			checks, err := sut.syntheticStore.ListSyntheticChecks(1000, false)
			if err != nil {
				t.Errorf("cleanup: failed to list synthetic checks: %v", err)
			} else {
				for _, check := range checks {
					if delErr := sut.syntheticStore.DeleteSyntheticCheck(check.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete synthetic check %s: %v", check.ID, delErr)
					}
				}
			}
		}

		if sut.credentialStore != nil {
			profiles, err := sut.credentialStore.ListCredentialProfiles(1000)
			if err != nil {
				t.Errorf("cleanup: failed to list credential profiles: %v", err)
			} else {
				for _, profile := range profiles {
					if delErr := sut.credentialStore.DeleteCredentialProfile(profile.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete credential profile %s: %v", profile.ID, delErr)
					}
				}
			}
		}

		if sut.incidentStore != nil {
			incidentList, err := sut.incidentStore.ListIncidents(persistence.IncidentFilter{Limit: 1000})
			if err != nil {
				t.Errorf("cleanup: failed to list incidents: %v", err)
			} else {
				for _, incident := range incidentList {
					if delErr := sut.incidentStore.DeleteIncident(incident.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete incident %s: %v", incident.ID, delErr)
					}
				}
			}
		}

		if sut.assetStore != nil {
			assets, err := sut.assetStore.ListAssets()
			if err != nil {
				t.Errorf("cleanup: failed to list assets: %v", err)
			} else {
				for _, asset := range assets {
					if delErr := sut.assetStore.DeleteAsset(asset.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete asset %s: %v", asset.ID, delErr)
					}
				}
			}
		}

		if sut.terminalStore != nil {
			sessions, err := sut.terminalStore.ListSessions()
			if err != nil {
				t.Errorf("cleanup: failed to list terminal sessions: %v", err)
			} else {
				for _, session := range sessions {
					if delErr := sut.terminalStore.DeleteTerminalSession(session.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete terminal session %s: %v", session.ID, delErr)
					}
				}
			}
		}

		if sut.groupStore != nil {
			groupList, err := sut.groupStore.ListGroups()
			if err != nil {
				t.Errorf("cleanup: failed to list groups: %v", err)
			} else {
				for _, group := range groupList {
					if delErr := sut.groupStore.DeleteGroup(group.ID); delErr != nil {
						t.Errorf("cleanup: failed to delete group %s: %v", group.ID, delErr)
					}
				}
			}
		}
	})

	return sut
}
