package backends

import "testing"

func TestParseLaunchctlListOutput(t *testing.T) {
	raw := `PID	Status	Label
123	0	com.example.running
-	0	com.example.stopped
-	78	com.example.failed
`

	services := ParseLaunchctlListOutput(raw)
	if len(services) != 3 {
		t.Fatalf("len(services)=%d, want 3", len(services))
	}

	if services[0].Name != "com.example.running" || services[0].ActiveState != "active" || services[0].SubState != "running" {
		t.Fatalf("unexpected running service parse: %+v", services[0])
	}
	if services[1].Name != "com.example.stopped" || services[1].ActiveState != "inactive" || services[1].SubState != "stopped" {
		t.Fatalf("unexpected stopped service parse: %+v", services[1])
	}
	if services[2].Name != "com.example.failed" || services[2].SubState != "failed" {
		t.Fatalf("unexpected failed service parse: %+v", services[2])
	}
}

func TestBuildLaunchctlActionCandidates(t *testing.T) {
	candidates := BuildLaunchctlActionCandidates("start", "com.example.job")
	if len(candidates) < 2 {
		t.Fatalf("expected multiple launchctl candidates, got %d", len(candidates))
	}

	last := candidates[len(candidates)-1]
	if len(last) != 2 || last[0] != "start" || last[1] != "com.example.job" {
		t.Fatalf("unexpected fallback start candidate: %v", last)
	}
}
