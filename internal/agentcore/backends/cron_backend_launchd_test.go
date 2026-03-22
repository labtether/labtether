package backends

import "testing"

func TestBuildLaunchdSchedule(t *testing.T) {
	payload := map[string]any{
		"StartInterval": float64(300),
		"RunAtLoad":     true,
	}

	schedule := BuildLaunchdSchedule(payload)
	if schedule != "every 300s | @reboot" {
		t.Fatalf("schedule=%q, want %q", schedule, "every 300s | @reboot")
	}
}

func TestBuildLaunchdScheduleCalendar(t *testing.T) {
	payload := map[string]any{
		"StartCalendarInterval": map[string]any{
			"Minute":  float64(15),
			"Hour":    float64(3),
			"Weekday": float64(1),
		},
	}

	schedule := BuildLaunchdSchedule(payload)
	if schedule != "15 3 * * 1" {
		t.Fatalf("schedule=%q, want %q", schedule, "15 3 * * 1")
	}
}

func TestBuildLaunchdCronEntry(t *testing.T) {
	payload := map[string]any{
		"Label": "com.labtether.test-job",
		"ProgramArguments": []any{
			"/usr/bin/env",
			"bash",
			"-lc",
			"echo test",
		},
		"StartInterval": float64(60),
	}

	entry, ok := BuildLaunchdCronEntry(payload, "michael")
	if !ok {
		t.Fatal("expected launchd entry to be built")
	}
	if entry.Source != "launchd" {
		t.Fatalf("source=%q, want launchd", entry.Source)
	}
	if entry.User != "michael" {
		t.Fatalf("user=%q, want michael", entry.User)
	}
	if entry.Command != "/usr/bin/env bash -lc echo test" {
		t.Fatalf("command=%q", entry.Command)
	}
	if entry.Schedule != "every 60s" {
		t.Fatalf("schedule=%q, want every 60s", entry.Schedule)
	}
}
