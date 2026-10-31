package desktop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

type recordingPrincipalKey struct{}

type recordingPrincipal struct{ id, role string }

func recordingDownloadDeps(t *testing.T, rows map[string]RecordingMetadata) *Deps {
	t.Helper()
	principal := func(ctx context.Context) recordingPrincipal {
		p, _ := ctx.Value(recordingPrincipalKey{}).(recordingPrincipal)
		return p
	}
	return &Deps{
		RecordingsDir:       t.TempDir(),
		UserRoleFromContext: func(ctx context.Context) string { return principal(ctx).role },
		PrincipalActorID:    func(ctx context.Context) string { return principal(ctx).id },
		LookupRecording: func(_ context.Context, id string) (RecordingMetadata, bool, error) {
			meta, ok := rows[id]
			return meta, ok, nil
		},
	}
}

func downloadRecording(d *Deps, path, actorID, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), recordingPrincipalKey{}, recordingPrincipal{id: actorID, role: role}))
	rec := httptest.NewRecorder()
	d.HandleRecordingActions(rec, req)
	return rec
}

func TestRecordingDownloadStreamsAttachmentFromDataDir(t *testing.T) {
	const fileName = "rec_1_20261026T120000.bin"
	d := recordingDownloadDeps(t, map[string]RecordingMetadata{
		// Rows written before 1.7.1 store the old relative path; the file is
		// still resolved by base name inside the recordings directory.
		"rec_1": {ID: "rec_1", SessionID: "sess-1", AssetID: "srv1", ActorID: "op-1", FilePath: "data/recordings/" + fileName, Status: "completed"},
	})
	if err := os.WriteFile(filepath.Join(d.RecordingsDir, fileName), []byte("vnc-frames"), 0o600); err != nil {
		t.Fatal(err)
	}

	rec := downloadRecording(d, RecordingDownloadPath("rec_1"), "op-1", auth.RoleOperator)
	if rec.Code != http.StatusOK || rec.Body.String() != "vnc-frames" {
		t.Fatalf("download status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") || !strings.Contains(got, fileName) {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestRecordingDownloadRefusesViewersUnknownAndForeignRecordings(t *testing.T) {
	const fileName = "rec_2_20261026T120000.bin"
	d := recordingDownloadDeps(t, map[string]RecordingMetadata{
		"rec_2":      {ID: "rec_2", SessionID: "sess-2", AssetID: "srv1", ActorID: "op-2", FilePath: fileName, Status: "completed"},
		"rec_escape": {ID: "rec_escape", SessionID: "sess-3", AssetID: "srv1", ActorID: "op-2", FilePath: "../../etc/passwd", Status: "completed"},
		"rec_live":   {ID: "rec_live", SessionID: "sess-4", AssetID: "srv1", ActorID: "op-2", FilePath: fileName, Status: "recording"},
	})
	if err := os.WriteFile(filepath.Join(d.RecordingsDir, fileName), []byte("frames"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, path, actor, role string
		want                    int
	}{
		{"viewer", RecordingDownloadPath("rec_2"), "op-2", auth.RoleViewer, http.StatusForbidden},
		{"unknown", RecordingDownloadPath("rec_missing"), "op-2", auth.RoleOperator, http.StatusNotFound},
		{"other operator", RecordingDownloadPath("rec_2"), "op-other", auth.RoleOperator, http.StatusNotFound},
		{"stored path traversal", RecordingDownloadPath("rec_escape"), "op-2", auth.RoleOperator, http.StatusNotFound},
		{"id traversal", "/recordings/..%2f..%2fetc/download", "op-2", auth.RoleAdmin, http.StatusNotFound},
		{"in progress", RecordingDownloadPath("rec_live"), "op-2", auth.RoleOperator, http.StatusConflict},
		{"admin sees any", RecordingDownloadPath("rec_2"), "admin-1", auth.RoleAdmin, http.StatusOK},
	}
	for _, tc := range cases {
		if rec := downloadRecording(d, tc.path, tc.actor, tc.role); rec.Code != tc.want {
			t.Errorf("%s: status=%d body=%s, want %d", tc.name, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func TestRecordingDownloadRefusesSymlinkedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	const fileName = "rec_3_20261026T120000.bin"
	d := recordingDownloadDeps(t, map[string]RecordingMetadata{
		"rec_3": {ID: "rec_3", SessionID: "sess-3", AssetID: "srv1", ActorID: "op-3", FilePath: fileName, Status: "completed"},
	})
	outside := filepath.Join(t.TempDir(), "secret.bin")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(d.RecordingsDir, fileName)); err != nil {
		t.Fatal(err)
	}
	if rec := downloadRecording(d, RecordingDownloadPath("rec_3"), "op-3", auth.RoleOperator); rec.Code != http.StatusNotFound {
		t.Fatalf("symlinked recording status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRecordingsDirLivesUnderDataDir(t *testing.T) {
	if got, want := RecordingsDirForDataDir("/data"), filepath.Join("/data", "recordings"); got != want {
		t.Fatalf("recordings dir = %q, want %q", got, want)
	}
}
