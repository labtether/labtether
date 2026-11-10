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

// Renaming or copying onto a taken name answered 400 with the agent's own
// "destination already exists", which named nothing. The answer must be a 409
// that names the taken entry and its folder on every agent OS, and any other
// refusal must still be about the source path.
func TestAgentFileNameClashNamesTheTakenEntry(t *testing.T) {
	cases := []struct {
		name, src, dst, agent string
		wantCode              int
		wantText              string
	}{
		{"linux rename", "/srv/docs/one.txt", "/srv/docs/two.txt", "destination already exists",
			http.StatusConflict, `"two.txt" already exists in /srv/docs. Choose another name.`},
		{"windows copy", `C:\docs\one.txt`, `C:\two.txt`, "destination already exists",
			http.StatusConflict, `"two.txt" already exists in C:\. Choose another name.`},
		{"folder in root", "/old", "/new/", "destination already exists",
			http.StatusConflict, `"new" already exists in /. Choose another name.`},
		{"taken after the agent's check", "/srv/a", "/srv/b", "rename a b: file exists",
			http.StatusConflict, `"b" already exists in /srv. Choose another name.`},
		{"bare name", "one.txt", "two.txt", "destination already exists",
			http.StatusConflict, `"two.txt" already exists. Choose another name.`},
		{"missing source", "/srv/gone.txt", "/srv/two.txt", "lstat gone.txt: no such file or directory",
			http.StatusNotFound, "file not found: /srv/gone.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, text := agentFileMoveErrorAnswer(tc.src, tc.dst, tc.agent)
			if code != tc.wantCode || text != tc.wantText {
				t.Fatalf("got %d %q, want %d %q", code, text, tc.wantCode, tc.wantText)
			}
		})
	}
}
