package backends

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// parseGetVMOutput — fixture-driven tests
// ---------------------------------------------------------------------------

func TestParseGetVMOutputFromFixture(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "get_vm.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	vms, err := parseGetVMOutput(raw)
	if err != nil {
		t.Fatalf("parseGetVMOutput returned error: %v", err)
	}

	if len(vms) != 4 {
		t.Fatalf("expected 4 VMs, got %d", len(vms))
	}
}

func TestParseGetVMOutputFields(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "get_vm.json"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	vms, err := parseGetVMOutput(raw)
	if err != nil {
		t.Fatalf("parseGetVMOutput returned error: %v", err)
	}

	byName := make(map[string]HyperVVM, len(vms))
	for _, v := range vms {
		byName[v.Name] = v
	}

	tests := []struct {
		name             string
		state            string
		cpuUsage         int
		memoryAssignedMB int64
		generation       int
		vmID             string
	}{
		{
			name:             "dc01",
			state:            "Running",
			cpuUsage:         12,
			memoryAssignedMB: 4096, // 4294967296 bytes / (1024*1024)
			generation:       2,
			vmID:             "a1b2c3d4-0001-0001-0001-000000000001",
		},
		{
			name:             "web-server",
			state:            "Running",
			cpuUsage:         4,
			memoryAssignedMB: 2048,
			generation:       2,
			vmID:             "a1b2c3d4-0002-0002-0002-000000000002",
		},
		{
			name:             "test-lab",
			state:            "Off",
			cpuUsage:         0,
			memoryAssignedMB: 0, // MemoryAssigned is 0 when VM is off
			generation:       1,
			vmID:             "a1b2c3d4-0003-0003-0003-000000000003",
		},
		{
			name:             "backup-vm",
			state:            "Saved",
			cpuUsage:         0,
			memoryAssignedMB: 1024,
			generation:       2,
			vmID:             "a1b2c3d4-0004-0004-0004-000000000004",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v, ok := byName[tc.name]
			if !ok {
				t.Fatalf("VM %q not found in output", tc.name)
			}
			if v.State != tc.state {
				t.Errorf("state=%q, want %q", v.State, tc.state)
			}
			if v.CPUUsage != tc.cpuUsage {
				t.Errorf("cpu_usage=%d, want %d", v.CPUUsage, tc.cpuUsage)
			}
			if v.MemoryAssignedMB != tc.memoryAssignedMB {
				t.Errorf("memory_assigned_mb=%d, want %d", v.MemoryAssignedMB, tc.memoryAssignedMB)
			}
			if v.Generation != tc.generation {
				t.Errorf("generation=%d, want %d", v.Generation, tc.generation)
			}
			if v.VMId != tc.vmID {
				t.Errorf("vm_id=%q, want %q", v.VMId, tc.vmID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseGetVMOutput — edge cases
// ---------------------------------------------------------------------------

func TestParseGetVMOutputEmpty(t *testing.T) {
	t.Parallel()

	vms, err := parseGetVMOutput([]byte(""))
	if err != nil {
		t.Fatalf("parseGetVMOutput returned error on empty input: %v", err)
	}
	if len(vms) != 0 {
		t.Fatalf("expected 0 VMs from empty input, got %d", len(vms))
	}
}

func TestParseGetVMOutputWhitespaceOnly(t *testing.T) {
	t.Parallel()

	vms, err := parseGetVMOutput([]byte("   \n\t  "))
	if err != nil {
		t.Fatalf("unexpected error on whitespace-only input: %v", err)
	}
	if len(vms) != 0 {
		t.Fatalf("expected 0 VMs from whitespace-only input, got %d", len(vms))
	}
}

func TestParseGetVMOutputSingleObject(t *testing.T) {
	t.Parallel()

	// PowerShell emits a bare object (not an array) when only one VM exists.
	input := `{
		"Name": "solo-vm",
		"State": 2,
		"CPUUsage": 8,
		"MemoryAssigned": 2147483648,
		"MemoryStartup": 2147483648,
		"Generation": 2,
		"Uptime": {"Days": 0, "Hours": 1, "Minutes": 30, "Seconds": 0, "TotalSeconds": 5400.0},
		"Status": "Operating normally",
		"Path": "C:\\VMs",
		"VMId": "aaaabbbb-cccc-dddd-eeee-ffffffffffff"
	}`

	vms, err := parseGetVMOutput([]byte(input))
	if err != nil {
		t.Fatalf("parseGetVMOutput returned error for single object: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("expected 1 VM, got %d", len(vms))
	}
	if vms[0].Name != "solo-vm" {
		t.Errorf("name=%q, want solo-vm", vms[0].Name)
	}
	if vms[0].State != "Running" {
		t.Errorf("state=%q, want Running", vms[0].State)
	}
	if vms[0].MemoryAssignedMB != 2048 {
		t.Errorf("memory_assigned_mb=%d, want 2048", vms[0].MemoryAssignedMB)
	}
}

func TestParseGetVMOutputBOM(t *testing.T) {
	t.Parallel()

	bom := "\xef\xbb\xbf"
	input := bom + `[{"Name":"bom-vm","State":3,"CPUUsage":0,"MemoryAssigned":0,"MemoryStartup":1073741824,"Generation":2,"Uptime":{"Days":0,"Hours":0,"Minutes":0,"Seconds":0,"TotalSeconds":0},"Status":"","Path":"C:\\VMs","VMId":"00000000-0000-0000-0000-000000000001"}]`

	vms, err := parseGetVMOutput([]byte(input))
	if err != nil {
		t.Fatalf("parseGetVMOutput returned error with BOM: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("expected 1 VM with BOM input, got %d", len(vms))
	}
	if vms[0].Name != "bom-vm" {
		t.Errorf("name=%q, want bom-vm", vms[0].Name)
	}
}

func TestParseGetVMOutputSkipsEmptyName(t *testing.T) {
	t.Parallel()

	input := `[
		{"Name":"valid-vm","State":2,"CPUUsage":0,"MemoryAssigned":1073741824,"MemoryStartup":1073741824,"Generation":2,"Uptime":{"Days":0,"Hours":0,"Minutes":0,"Seconds":0,"TotalSeconds":0},"Status":"","Path":"","VMId":"aaa"},
		{"Name":"","State":3,"CPUUsage":0,"MemoryAssigned":0,"MemoryStartup":0,"Generation":1,"Uptime":{"Days":0,"Hours":0,"Minutes":0,"Seconds":0,"TotalSeconds":0},"Status":"","Path":"","VMId":"bbb"}
	]`

	vms, err := parseGetVMOutput([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("expected 1 VM (entry with empty Name skipped), got %d", len(vms))
	}
	if vms[0].Name != "valid-vm" {
		t.Errorf("name=%q, want valid-vm", vms[0].Name)
	}
}

// ---------------------------------------------------------------------------
// hyperVStateString — state mapping
// ---------------------------------------------------------------------------

func TestHyperVStateString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state int
		want  string
	}{
		{2, "Running"},
		{3, "Off"},
		{6, "Saved"},
		{9, "Paused"},
		{0, "Unknown(0)"},
		{99, "Unknown(99)"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := hyperVStateString(tc.state)
			if got != tc.want {
				t.Errorf("hyperVStateString(%d) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatVMUptime
// ---------------------------------------------------------------------------

func TestFormatVMUptime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		uptime vmUptimeJSON
		want   string
	}{
		{
			uptime: vmUptimeJSON{Days: 3, Hours: 14, Minutes: 22, Seconds: 7, TotalSeconds: 310927},
			want:   "3d 14h 22m 7s",
		},
		{
			uptime: vmUptimeJSON{Days: 0, Hours: 5, Minutes: 41, Seconds: 33, TotalSeconds: 20493},
			want:   "5h 41m 33s",
		},
		{
			uptime: vmUptimeJSON{Days: 0, Hours: 0, Minutes: 0, Seconds: 0, TotalSeconds: 0},
			want:   "0s",
		},
		{
			uptime: vmUptimeJSON{Days: 1, Hours: 0, Minutes: 0, Seconds: 0, TotalSeconds: 86400},
			want:   "1d",
		},
		{
			uptime: vmUptimeJSON{Days: 0, Hours: 0, Minutes: 5, Seconds: 0, TotalSeconds: 300},
			want:   "5m",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := formatVMUptime(tc.uptime)
			if got != tc.want {
				t.Errorf("formatVMUptime(%+v) = %q, want %q", tc.uptime, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PerformVMAction — input validation (no PowerShell required)
// ---------------------------------------------------------------------------

func TestPerformVMActionUnsupportedAction(t *testing.T) {
	t.Parallel()

	b := HyperVBackend{}
	err := b.PerformVMAction("migrate", "some-vm")
	if err == nil {
		t.Fatal("expected error for unsupported action, got nil")
	}
}

func TestPerformVMActionEmptyVMName(t *testing.T) {
	t.Parallel()

	b := HyperVBackend{}
	err := b.PerformVMAction("start", "")
	if err == nil {
		t.Fatal("expected error for empty vmName, got nil")
	}
}

func TestPerformVMActionEmptyVMNameWhitespace(t *testing.T) {
	t.Parallel()

	b := HyperVBackend{}
	err := b.PerformVMAction("stop", "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only vmName, got nil")
	}
}
