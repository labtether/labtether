package webservice

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestAgentReportCannotClaimAnotherHostOrItsHealth(t *testing.T) {
	coord := NewCoordinator()
	now := time.Date(2026, time.October, 11, 1, 0, 0, 0, time.UTC)
	coord.nowFn = func() time.Time { return now }
	coord.HandleReport("agent-b", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-b",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "report-host", Status: "up", URL: "https://b.example/one"},
			{ID: "service-host", Status: "up", URL: "https://b.example/two"},
		},
	}))
	now = now.Add(2 * time.Minute)
	spoofed := makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-b",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "report-host", Status: "down", URL: "https://a.example/one"},
			{ID: "service-host", HostAssetID: "agent-b", Status: "down", URL: "https://a.example/two"},
		},
	})
	coord.HandleReport("agent-a", spoofed)
	for _, svc := range coord.ListAll() {
		if svc.Status == "down" && svc.HostAssetID != "agent-a" {
			t.Errorf("A's service entered another access scope: %+v", svc)
		}
	}
	if up, total := coord.SummaryByHosts(map[string]struct{}{"agent-b": {}}); up != 2 || total != 2 {
		t.Fatalf("B's summary includes another agent: up=%d total=%d", up, total)
	}
	assertBUnchanged := func() {
		t.Helper()
		services := coord.ListByHost("agent-b")
		coord.AttachHealthSummaries(services)
		if len(services) != 2 {
			t.Fatalf("B lost legitimate services: %d", len(services))
		}
		for _, svc := range services {
			if svc.HostAssetID != "agent-b" || svc.Status != "up" || svc.Health == nil || svc.Health.Checks != 1 || svc.Health.UpChecks != 1 {
				t.Errorf("B's service or health changed: %+v", svc)
			}
		}
	}
	assertBUnchanged()

	// Empty reports still clear only their real owner's services, and normal
	// host removal and ownerless input cannot mutate the other host's history.
	coord.HandleReport("agent-a", makeReportMsg(agentmgr.WebServiceReportData{HostAssetID: "agent-b"}))
	if services := coord.ListByHost("agent-a"); len(services) != 0 {
		t.Fatalf("empty report retained A's services: %d", len(services))
	}
	coord.RemoveHost("agent-a")
	coord.HandleReport("  ", spoofed)
	if all := coord.ListAll(); len(all) != 2 {
		t.Fatalf("ownerless report populated discovery: %d services", len(all))
	}
	assertBUnchanged()
}
