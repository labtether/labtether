package webservice

import (
	"encoding/json"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

// A report that does not decode is dropped and the host keeps its services.
// Taking it as an empty report would clear the host's row on the Services
// page until the next good report.
func TestHandleReportIgnoresAReportThatDoesNotDecode(t *testing.T) {
	coord := NewCoordinator()
	coord.HandleReport("nas01", makeReportMsg(agentmgr.WebServiceReportData{
		HostAssetID: "nas01",
		Services: []agentmgr.DiscoveredWebService{
			{ID: "svc-1", Name: "Grafana", Category: "monitoring", URL: "http://192.0.2.10:3000", Status: "up"},
		},
	}))
	coord.HandleReport("nas01", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: json.RawMessage(`{"services":`)})
	if all := coord.ListAll(); len(all) != 1 || all[0].Name != "Grafana" {
		t.Fatalf("services after a broken report = %+v, want the earlier Grafana entry", all)
	}
}
