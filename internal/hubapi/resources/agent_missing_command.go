package resources

import (
	"net/http"
	"regexp"

	"github.com/labtether/labtether/internal/servicehttp"
)

// A command the device does not have reaches the Hub as Go's exec error:
// exec: "systemctl": executable file not found in $PATH (%PATH% on Windows).
var missingCommandPattern = regexp.MustCompile(`exec: "([^"]+)": executable file not found in [$%]PATH%?`)

// missingCommand names the command an agent error says is not installed.
func missingCommand(agentError string) (string, bool) {
	match := missingCommandPattern.FindStringSubmatch(agentError)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// writeMissingCommand answers a request the device cannot serve because it
// lacks the command behind it, in plain words instead of Go's exec text. what
// finishes the sentence, for example "services can't be listed". It reports
// false, writing nothing, for any other agent error.
func writeMissingCommand(w http.ResponseWriter, agentError, what string) bool {
	command, ok := missingCommand(agentError)
	if !ok {
		return false
	}
	have := command + " command"
	if command == "systemctl" || command == "journalctl" {
		have = "systemd"
	}
	servicehttp.WriteError(w, http.StatusUnprocessableEntity, "This device has no "+have+", so "+what+".")
	return true
}
