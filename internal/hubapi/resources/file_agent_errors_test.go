package resources

import (
	"net/http"
	"testing"
)

// Agents send Go's error text for file requests, and clients showed it as is:
// "openat docs/none.txt: no such file or directory" (400). A missing path must
// be a 404 in plain words on every agent OS, and other errors must lose Go's
// operation prefix but keep the agent's own reason.
func TestAgentFileErrorsArePlainWords(t *testing.T) {
	cases := []struct {
		name, missing, path, agent string
		wantCode                   int
		wantText                   string
	}{
		{"linux missing file", missingFile, "docs/none.txt", "openat docs/none.txt: no such file or directory",
			http.StatusNotFound, "file not found: docs/none.txt"},
		{"windows missing folder", missingFolder, `C:\none`, `open C:\none: The system cannot find the path specified.`,
			http.StatusNotFound, `folder not found: C:\none`},
		{"permission", missingFile, "/root/x", "openat /root/x: permission denied",
			http.StatusBadRequest, "permission denied: /root/x"},
		{"other reason", missingFile, "docs", "unlinkat docs: directory not empty",
			http.StatusBadRequest, "directory not empty: docs"},
		{"agent words kept", missingFolder, "/etc", `path "/etc" is outside the allowed base directory`,
			http.StatusBadRequest, `path "/etc" is outside the allowed base directory`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, text := agentFileErrorAnswer(tc.missing, tc.path, tc.agent)
			if code != tc.wantCode || text != tc.wantText {
				t.Fatalf("got %d %q, want %d %q", code, text, tc.wantCode, tc.wantText)
			}
		})
	}
}
