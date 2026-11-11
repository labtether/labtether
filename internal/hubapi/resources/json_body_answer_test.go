package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// An SFTP rename sent with query parameters instead of the JSON body answered
// an empty 200 and renamed nothing, so the caller believed it had worked. A
// body that cannot be read now gets a 400 that says why.
func TestFileConnectionRenameRefusesABodyItCannotRead(t *testing.T) {
	d := &Deps{DecodeJSONBody: shared.DecodeJSONBody}
	for body, want := range map[string]string{
		"":                               "The request needs a JSON body.",
		"old_path=/a":                    "The request body is not a valid JSON object.",
		`{"old_path":7,"new_path":"/b"}`: "old_path must be text.",
	} {
		req := httptest.NewRequest(http.MethodPost, "/file-connections/conn-1/rename?old_path=/a&new_path=/b", strings.NewReader(body))
		rec := httptest.NewRecorder()
		d.dispatchFileProtoOp(rec, req, "conn-1", "rename")
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body %q: rename = %d %q, want 400 %q", body, rec.Code, rec.Body.String(), want)
		}
	}
}
