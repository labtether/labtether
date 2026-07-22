package webservice

import (
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestConcurrentHealthReportAndSummaryAccess(t *testing.T) {
	coord := NewCoordinator()
	report := makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "agent-01",
		Services: []agentmgr.DiscoveredWebService{{
			ID: "svc-1", Name: "Service", Status: "up", ResponseMs: 10, HostAssetID: "agent-01",
		}},
	})
	coord.HandleReport("agent-01", report)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 250 {
			coord.HandleReport("agent-01", report)
		}
	}()
	go func() {
		defer wg.Done()
		for range 250 {
			services := coord.ListAll()
			coord.AttachHealthSummaries(services)
		}
	}()
	wg.Wait()

	services := coord.ListAll()
	coord.AttachHealthSummaries(services)
	if len(services) != 1 || services[0].Health == nil || services[0].Health.Checks == 0 {
		t.Fatalf("concurrent service health snapshot = %+v", services)
	}
}
