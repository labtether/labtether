package resources

import (
	"strings"
	"unicode/utf8"
)

const maxAgentFailureMessageBytes = 400

// agentFailureMessage is what a user reads when a service or package action
// fails on the device. The agent sends the command's error ("exit status 5")
// and its output; the output says what went wrong ("Unit x.service not
// found.", "E: Unable to locate package x"), so its error lines are used, or
// its last line, and the bare exit status only when there is no output.
func agentFailureMessage(errText, output string) string {
	lines := make([]string, 0, 8)
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return errText
	}
	picked := make([]string, 0, 3)
	for i := len(lines) - 1; i >= 0 && len(picked) < 3; i-- {
		if looksLikeCommandError(lines[i]) {
			picked = append([]string{lines[i]}, picked...)
		}
	}
	if len(picked) == 0 {
		picked = []string{lines[len(lines)-1]}
	}
	message := strings.Join(picked, " ")
	if len(message) > maxAgentFailureMessageBytes {
		message = message[:maxAgentFailureMessageBytes]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		message += "…"
	}
	return message
}

func looksLikeCommandError(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(line, "E: ") || strings.HasPrefix(lower, "error") ||
		strings.Contains(lower, "failed") || strings.Contains(lower, "not found") ||
		strings.Contains(lower, "unable to") || strings.Contains(lower, "no such")
}
