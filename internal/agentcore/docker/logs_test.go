package docker

import (
	"context"
	"net/http"
	"testing"
)

func TestDockerContainerLogs(t *testing.T) {
	srv, client := newSecureDockerClientFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/abc123/logs" {
			q := r.URL.Query()
			if q.Get("tail") != "100" {
				t.Errorf("tail = %q, want 100", q.Get("tail"))
			}
			if q.Get("follow") != "true" {
				t.Errorf("follow = %q, want true", q.Get("follow"))
			}
			w.Write([]byte("line 1\nline 2\n"))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	body, err := client.containerLogs(context.Background(), "abc123", 100, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()

	buf := make([]byte, 1024)
	n, _ := body.Read(buf)
	if n == 0 {
		t.Error("expected log data")
	}
}

func TestDockerLogManagerMaxStreams(t *testing.T) {
	client := NewDockerClient("http://localhost:1")
	lm := NewDockerLogManager(client)

	// Fill up streams to the max
	lm.mu.Lock()
	for i := 0; i < maxLogStreams; i++ {
		lm.streams[string(rune('a'+i))] = &dockerLogStream{
			sessionID: string(rune('a' + i)),
			cancel:    func() {},
		}
	}
	lm.mu.Unlock()

	if len(lm.streams) != maxLogStreams {
		t.Fatalf("expected %d streams, got %d", maxLogStreams, len(lm.streams))
	}
}

func TestDockerLogManagerCloseAll(t *testing.T) {
	client := NewDockerClient("http://localhost:1")
	lm := NewDockerLogManager(client)
	lm.CloseAll() // should not panic on empty

	if len(lm.streams) != 0 {
		t.Errorf("expected 0 streams after closeAll, got %d", len(lm.streams))
	}
}
