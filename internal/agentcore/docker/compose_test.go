package docker

import "testing"

func TestDockerComposeDetectCLI(t *testing.T) {
	// Just verifies the function runs without panic — actual availability depends on host
	version, available := isDockerComposeCLIAvailable()
	t.Logf("docker compose CLI: version=%d available=%v", version, available)
	// No assertion — this is a host-dependent check
}

func TestDockerComposeBuildCommandV2(t *testing.T) {
	cmd, args := composeCommandArgs(2, "/opt/stacks/web", "up", "-d")
	if cmd != "docker" {
		t.Errorf("cmd = %q, want %q", cmd, "docker")
	}
	// Expected: ["compose", "--project-directory", "/opt/stacks/web", "up", "-d"]
	if len(args) < 5 {
		t.Fatalf("expected at least 5 args, got %d: %v", len(args), args)
	}
	if args[0] != "compose" {
		t.Errorf("args[0] = %q, want %q", args[0], "compose")
	}
	if args[1] != "--project-directory" {
		t.Errorf("args[1] = %q, want %q", args[1], "--project-directory")
	}
	if args[2] != "/opt/stacks/web" {
		t.Errorf("args[2] = %q, want %q", args[2], "/opt/stacks/web")
	}
	if args[3] != "up" {
		t.Errorf("args[3] = %q, want %q", args[3], "up")
	}
	if args[4] != "-d" {
		t.Errorf("args[4] = %q, want %q", args[4], "-d")
	}
}

func TestDockerComposeBuildCommandV1(t *testing.T) {
	cmd, args := composeCommandArgs(1, "/opt/stacks/db", "down")
	if cmd != "docker-compose" {
		t.Errorf("cmd = %q, want %q", cmd, "docker-compose")
	}
	if len(args) < 3 {
		t.Fatalf("expected at least 3 args, got %d: %v", len(args), args)
	}
	if args[0] != "--project-directory" {
		t.Errorf("args[0] = %q, want %q", args[0], "--project-directory")
	}
	if args[2] != "down" {
		t.Errorf("args[2] = %q, want %q", args[2], "down")
	}
}

func TestDockerComposeBuildCommandActions(t *testing.T) {
	tests := []struct {
		action   string
		wantArgs int // minimum args count
		wantLast string
	}{
		{"up", 5, "-d"},
		{"down", 4, "down"},
		{"restart", 4, "restart"},
		{"pull", 4, "pull"},
	}
	for _, tt := range tests {
		extraArgs := []string{}
		if tt.action == "up" {
			extraArgs = []string{"-d"}
		}
		_, args := composeCommandArgs(2, "/opt/stacks/test", tt.action, extraArgs...)
		if len(args) < tt.wantArgs {
			t.Errorf("action %q: expected at least %d args, got %d", tt.action, tt.wantArgs, len(args))
		}
		if args[len(args)-1] != tt.wantLast {
			t.Errorf("action %q: last arg = %q, want %q", tt.action, args[len(args)-1], tt.wantLast)
		}
	}
}
