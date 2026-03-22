package backends

import (
	"os"
	"testing"
)

func TestParseSCQueryOutput(t *testing.T) {
	data, err := os.ReadFile("testdata/sc_query_all.txt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	services := parseSCQueryOutput(string(data))

	if len(services) != 4 {
		t.Fatalf("expected 4 services, got %d", len(services))
	}

	tests := []struct {
		name, displayName, status string
	}{
		{"Winmgmt", "Windows Management Instrumentation", "active"},
		{"wuauserv", "Windows Update", "inactive"},
		{"WinRM", "Windows Remote Management (WS-Management)", "activating"},
		{"Spooler", "Print Spooler", "deactivating"},
	}
	for i, tc := range tests {
		if services[i].Name != tc.name {
			t.Errorf("[%d] name: got %q, want %q", i, services[i].Name, tc.name)
		}
		if services[i].ActiveState != tc.status {
			t.Errorf("[%d] status: got %q, want %q", i, services[i].ActiveState, tc.status)
		}
		if services[i].Description != tc.displayName {
			t.Errorf("[%d] description: got %q, want %q", i, services[i].Description, tc.displayName)
		}
	}
}

func TestParseSCQueryOutput_Empty(t *testing.T) {
	services := parseSCQueryOutput("")
	if len(services) != 0 {
		t.Errorf("expected 0 services, got %d", len(services))
	}
}
