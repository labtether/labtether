package agentmgr

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// A message literal with fields; a slice of messages or the zero value
	// builds nothing.
	rawMessageBuild = regexp.MustCompile(`(?:^|[^\]])agentmgr\.Message\{[^}]`)
	// A payload decoded by hand from some value's Data field.
	rawMessageDecode = regexp.MustCompile(`(?:Unmarshal|DecodeData)\(\s*(\w+)\.Data\b`)
)

// rawMessageAllowed lists the matches that are not agent messages built or
// decoded by hand, by file and matched text.
var rawMessageAllowed = map[string]string{
	"internal/connectors/proxmox/client.go:wrapped.Data":               "Proxmox API response, not an agent message",
	"internal/connectors/pbs/client.go:wrapped.Data":                   "PBS API response, not an agent message",
	"internal/hubapi/desktop/webrtc_stream_handlers.go:sigMsg.Data":    "browser signalling frame, not an agent message",
	"internal/hubapi/testutil/agent_reply_freeze.go:agentmgr.Message{": "test support: builds raw replies to pin the wire shape",
	"internal/hubapi/testutil/agent_reply_freeze.go:sent.Data":         "test support: reads the request ID the Hub sent",
}

// Only agentmgr builds and decodes agent messages: every other package uses
// NewMessage and Decode, so the envelope and decode rules live in one place.
// Test files may build raw messages to check the wire shape.
func TestOnlyAgentmgrBuildsAndDecodesAgentMessages(t *testing.T) {
	root := filepath.Join("..", "..")
	seen := map[string]bool{}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/agentmgr/") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(src), "\n") {
				var found []string
				if rawMessageBuild.MatchString(line) {
					found = append(found, "agentmgr.Message{")
				}
				for _, m := range rawMessageDecode.FindAllStringSubmatch(line, -1) {
					found = append(found, m[1]+".Data")
				}
				for _, what := range found {
					key := rel + ":" + what
					if _, ok := rawMessageAllowed[key]; ok {
						seen[key] = true
						continue
					}
					t.Errorf("%s:%d builds or decodes an agent message by hand (%s); use agentmgr.NewMessage or agentmgr.Decode", rel, i+1, what)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for key := range rawMessageAllowed {
		if !seen[key] {
			t.Errorf("allow-list entry %q matches nothing; remove it", key)
		}
	}
}
