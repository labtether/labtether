package apiv2

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// File uploads on v2 routes run inside WrapV1Handler. They keep a long upload
// alive by moving the connection deadlines, which must reach the real
// connection instead of stopping at the buffering recorder.
func TestWrapV1HandlerPassesConnectionDeadlinesThrough(t *testing.T) {
	results := make(chan [2]error, 1)
	server := httptest.NewServer(WrapV1Handler(func(w http.ResponseWriter, _ *http.Request) {
		controller := http.NewResponseController(w)
		deadline := time.Now().Add(time.Minute)
		results <- [2]error{controller.SetReadDeadline(deadline), controller.SetWriteDeadline(deadline)}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := <-results; got[0] != nil || got[1] != nil {
		t.Fatalf("deadline errors read=%v write=%v", got[0], got[1])
	}
}
