package backends

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSchtasksCSVCount(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "schtasks_query.csv"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseSchtasksCSV(raw)
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}

	// Fixture has 5 tasks; 2 are under \Microsoft\ and must be filtered.
	// Remaining: \BackupJob, \HealthCheck, \CleanTempFiles = 3.
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after Microsoft filter, got %d", len(entries))
	}
}

func TestParseSchtasksCSVMicrosoftFiltered(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "schtasks_query.csv"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseSchtasksCSV(raw)
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}

	for _, e := range entries {
		if len(e.Command) >= len(`\Microsoft\`) && e.Command[:len(`\Microsoft\`)] == `\Microsoft\` {
			t.Errorf("entry with name starting with \\Microsoft\\ was not filtered: %+v", e)
		}
	}
}

func TestParseSchtasksCSVFields(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "schtasks_query.csv"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseSchtasksCSV(raw)
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}

	byName := make(map[string]struct {
		schedule string
		command  string
		user     string
		nextRun  string
		lastRun  string
	}, len(entries))
	for _, e := range entries {
		byName[e.Source+":"+e.Command] = struct {
			schedule string
			command  string
			user     string
			nextRun  string
			lastRun  string
		}{e.Schedule, e.Command, e.User, e.NextRun, e.LastRun}
	}

	// \BackupJob: daily at 2:00 AM, run as Administrator, last run set, next run set
	backupKey := "task-scheduler:\\BackupJob"
	backup, ok := byName[backupKey]
	if !ok {
		t.Fatalf("expected \\BackupJob entry, keys: %v", func() []string {
			ks := make([]string, 0, len(byName))
			for k := range byName {
				ks = append(ks, k)
			}
			return ks
		}())
	}
	if backup.user != "Administrator" {
		t.Errorf("\\BackupJob user=%q, want Administrator", backup.user)
	}
	if backup.nextRun == "" {
		t.Error("\\BackupJob NextRun should not be empty")
	}
	if backup.lastRun == "" {
		t.Error("\\BackupJob LastRun should not be empty")
	}
	if backup.schedule == "" {
		t.Error("\\BackupJob Schedule should not be empty")
	}
}

func TestParseSchtasksCSVSourceField(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "schtasks_query.csv"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseSchtasksCSV(raw)
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}

	for _, e := range entries {
		if e.Source != "task-scheduler" {
			t.Errorf("entry Source=%q, want task-scheduler", e.Source)
		}
	}
}

func TestParseSchtasksCSVNANextRun(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "schtasks_query.csv"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseSchtasksCSV(raw)
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}

	// \HealthCheck has "N/A" for Next Run Time — should produce empty NextRun.
	for _, e := range entries {
		if e.Command == `\HealthCheck` {
			if e.NextRun != "" {
				t.Errorf("\\HealthCheck NextRun=%q, want empty (was N/A in fixture)", e.NextRun)
			}
			return
		}
	}
	t.Fatal("\\HealthCheck entry not found")
}

func TestParseSchtasksCSVScheduleType(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "schtasks_query.csv"))
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	entries, err := parseSchtasksCSV(raw)
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}

	// \CleanTempFiles is Weekly — schedule should contain "Weekly".
	for _, e := range entries {
		if e.Command == `\CleanTempFiles` {
			if e.Schedule == "" {
				t.Error("\\CleanTempFiles Schedule should not be empty")
			}
			return
		}
	}
	t.Fatal("\\CleanTempFiles entry not found")
}

func TestParseSchtasksCSVEmpty(t *testing.T) {
	t.Parallel()

	entries, err := parseSchtasksCSV([]byte(""))
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error on empty input: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries from empty input, got %d", len(entries))
	}
}

func TestParseSchtasksCSVHeaderOnly(t *testing.T) {
	t.Parallel()

	input := `"HostName","TaskName","Next Run Time","Status","Logon Mode","Last Run Time","Last Result","Author","Task To Run","Comment","Scheduled Task State","Idle Time","Power Management","Run As User","Delete Task If Not Rescheduled","Stop Task If Runs X Hours and X Mins","Schedule","Schedule Type","Start Time","Start Date","End Date","Days","Months","Repeat: Every","Repeat: Until: Time","Repeat: Until: Duration","Repeat: Stop If Still Running"
`
	entries, err := parseSchtasksCSV([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries from header-only input, got %d", len(entries))
	}
}

func TestParseSchtasksCSVInlineData(t *testing.T) {
	t.Parallel()

	// Minimal inline CSV with one user task and one Microsoft task.
	// Backslashes here are literal single backslashes (raw string, no escaping).
	input := "\"HostName\",\"TaskName\",\"Next Run Time\",\"Status\",\"Logon Mode\",\"Last Run Time\",\"Last Result\",\"Author\",\"Task To Run\",\"Comment\",\"Scheduled Task State\",\"Idle Time\",\"Power Management\",\"Run As User\",\"Delete Task If Not Rescheduled\",\"Stop Task If Runs X Hours and X Mins\",\"Schedule\",\"Schedule Type\",\"Start Time\",\"Start Date\",\"End Date\",\"Days\",\"Months\",\"Repeat: Every\",\"Repeat: Until: Time\",\"Repeat: Until: Duration\",\"Repeat: Stop If Still Running\"\n" +
		"\"TESTHOST\",\"\\Microsoft\\Windows\\Foo\",\"N/A\",\"Ready\",\"Interactive/Background\",\"N/A\",\"0\",\"Microsoft\",\"COM handler\",\"\",\"Enabled\",\"Disabled\",\"\",\"SYSTEM\",\"Disabled\",\"72:00:00\",\"\",\"Daily\",\"12:00:00 AM\",\"1/1/2020\",\"N/A\",\"Every Day\",\"Every Month\",\"Disabled\",\"Disabled\",\"Disabled\",\"Disabled\"\n" +
		"\"TESTHOST\",\"\\MyTask\",\"3/22/2026 6:00:00 AM\",\"Ready\",\"Interactive/Background\",\"3/21/2026 6:00:00 AM\",\"0\",\"TESTHOST\\user\",\"C:\\myjob.bat\",\"My task\",\"Enabled\",\"Disabled\",\"\",\"user\",\"Disabled\",\"00:30:00\",\"\",\"Daily\",\"6:00:00 AM\",\"1/1/2025\",\"N/A\",\"Every Day\",\"Every Month\",\"Disabled\",\"Disabled\",\"Disabled\",\"Disabled\"\n"
	entries, err := parseSchtasksCSV([]byte(input))
	if err != nil {
		t.Fatalf("parseSchtasksCSV returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (Microsoft filtered), got %d", len(entries))
	}
	if entries[0].Command != `\MyTask` {
		t.Errorf("command=%q, want \\MyTask", entries[0].Command)
	}
	if entries[0].User != "user" {
		t.Errorf("user=%q, want user", entries[0].User)
	}
	if entries[0].Source != "task-scheduler" {
		t.Errorf("source=%q, want task-scheduler", entries[0].Source)
	}
}
