package resources

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A host without systemd answered `services list` with Go's text:
// systemctl list-units: exec: "systemctl": executable file not found in $PATH.
// Users must read what is missing and what that means, in plain words; other
// agent errors stay as they are.
func TestMissingCommandIsSaidInPlainWords(t *testing.T) {
	cases := []struct{ agent, what, want string }{
		{`systemctl list-units: exec: "systemctl": executable file not found in $PATH`, "services can't be listed",
			"This device has no systemd, so services can't be listed."},
		{`exec: "ps": executable file not found in $PATH`, "processes can't be listed",
			"This device has no ps command, so processes can't be listed."},
		{`exec: "who": executable file not found in %PATH%`, "signed-in users can't be listed",
			"This device has no who command, so signed-in users can't be listed."},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		if !writeMissingCommand(rec, tc.agent, tc.what) || rec.Code != 422 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%q: got %d %s, want 422 %q", tc.agent, rec.Code, rec.Body.String(), tc.want)
		}
	}
	rec := httptest.NewRecorder()
	if writeMissingCommand(rec, "Failed to start x.service: Unit x.service not found.", "services can't be listed") || rec.Body.Len() != 0 {
		t.Fatalf("an unrelated agent error was rewritten: %s", rec.Body.String())
	}
}
