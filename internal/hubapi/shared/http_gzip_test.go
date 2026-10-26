package shared

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestGzipMiddlewareDiscardsHandlerContentLengthAtCommit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		payload  string
		explicit bool
		flush    bool
	}{
		{name: "explicit status", payload: "download marker", explicit: true},
		{name: "implicit write", payload: "download marker"},
		{name: "flush before write", payload: "download marker", flush: true},
		{name: "empty close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantStatus := http.StatusOK
			if tc.explicit {
				wantStatus = http.StatusCreated
			}
			server := httptest.NewServer(GzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// This is the download handler's late header: middleware has already run.
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Length", strconv.Itoa(len(tc.payload)))
				if tc.explicit {
					w.WriteHeader(wantStatus)
				}
				if tc.flush {
					w.(http.Flusher).Flush()
				}
				if tc.payload != "" {
					if _, err := io.WriteString(w, tc.payload); err != nil {
						t.Errorf("write response: %v", err)
					}
				}
			})))
			defer server.Close()
			transport := &http.Transport{DisableCompression: true}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
			req, err := http.NewRequest(http.MethodGet, server.URL+"/files/owned/download", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", "gzip")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			encoded, err := io.ReadAll(io.LimitReader(response.Body, 4096))
			if err != nil {
				t.Fatalf("read framed gzip response: %v", err)
			}
			if response.StatusCode != wantStatus || response.Header.Get("Content-Encoding") != "gzip" {
				t.Fatalf("status=%d encoding=%q", response.StatusCode, response.Header.Get("Content-Encoding"))
			}
			if response.ContentLength >= 0 && response.ContentLength != int64(len(encoded)) {
				t.Fatalf("content length=%d encoded bytes=%d", response.ContentLength, len(encoded))
			}
			decoder, err := gzip.NewReader(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			defer decoder.Close()
			decoded, err := io.ReadAll(io.LimitReader(decoder, 4096))
			if err != nil {
				t.Fatalf("decode complete gzip response: %v", err)
			}
			if string(decoded) != tc.payload {
				t.Fatalf("decoded=%q, want %q", decoded, tc.payload)
			}
		})
	}
}
