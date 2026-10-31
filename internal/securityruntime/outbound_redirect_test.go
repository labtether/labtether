package securityruntime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// A connector key header must not follow a redirect to another host, such as
// a forward-auth login page, and a request body must not be resent there.
func TestOutboundRedirectToAnotherHostDropsCredentialHeaders(t *testing.T) {
	allowTestLoopbackHTTP(t)
	withMockLookupIPAddrs(t, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	var targetHits atomic.Int32
	var gotKey, gotAccept atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		gotKey.Store(r.Header.Get("X-API-Key"))
		gotAccept.Store(r.Header.Get("Accept"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)
	authURL := "http://auth.example.test:" + targetURL.Port() + "/login"
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusFound
		if r.Method == http.MethodPost {
			status = http.StatusTemporaryRedirect
		}
		http.Redirect(w, r, authURL, status)
	}))
	defer source.Close()
	sourceURL, _ := url.Parse(source.URL)
	start := "http://portainer.example.test:" + sourceURL.Port() + "/api/endpoints"

	req, err := NewOutboundRequestWithContext(context.Background(), http.MethodGet, start, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-API-Key", "ptr_secret")
	req.Header.Set("Accept", "application/json")
	resp, err := DoOutboundRequest(&http.Client{}, req)
	if err != nil {
		t.Fatalf("follow redirect: %v", err)
	}
	_ = resp.Body.Close()
	if gotKey.Load() != "" || gotAccept.Load() != "application/json" {
		t.Fatalf("redirect target saw X-API-Key=%q Accept=%q", gotKey.Load(), gotAccept.Load())
	}

	post, err := NewOutboundRequestWithContext(context.Background(), http.MethodPost, start, strings.NewReader(`{"password":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DoOutboundRequest(&http.Client{}, post); err == nil {
		t.Fatal("body was allowed to follow a redirect to another host")
	}
	if targetHits.Load() != 1 {
		t.Fatalf("redirect target hits = %d, want 1", targetHits.Load())
	}
}
