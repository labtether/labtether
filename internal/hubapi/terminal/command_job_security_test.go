package terminal

import (
	"encoding/json"
	"strings"
	"testing"

	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestMarshalTerminalCommandJobExcludesResolvedSSHCredentials(t *testing.T) {
	job := terminalmodel.CommandJob{
		JobID:     "job-1",
		SessionID: "session-1",
		CommandID: "command-1",
		Target:    "asset-1",
		Command:   "uptime",
		SSHConfig: &terminalmodel.SSHConfig{
			Host:                 "host.internal",
			User:                 "operator",
			Password:             "queue-password-secret",
			PrivateKey:           "queue-private-key-secret",
			PrivateKeyPassphrase: "queue-passphrase-secret",
		},
	}

	payload, err := marshalTerminalCommandJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"queue-password-secret",
		"queue-private-key-secret",
		"queue-passphrase-secret",
		"ssh_config",
	} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("durable command payload contains %q: %s", secret, payload)
		}
	}
	if !strings.Contains(string(payload), `"command":"uptime"`) {
		t.Fatalf("durable command payload lost command metadata: %s", payload)
	}

	directPayload, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(directPayload), "ssh_config") || strings.Contains(string(directPayload), "queue-password-secret") {
		t.Fatalf("CommandJob JSON contract exposed execution-only SSH config: %s", directPayload)
	}
}
