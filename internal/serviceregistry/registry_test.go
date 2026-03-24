package serviceregistry

import (
	"testing"
)

// ── LookupByKey ─────────────────────────────────────────────────────────────

func TestLookupByKey_Known(t *testing.T) {
	svc, ok := LookupByKey("plex")
	if !ok {
		t.Fatal("expected to find 'plex' by key")
	}
	if svc.Name != "Plex" {
		t.Errorf("Name = %q, want %q", svc.Name, "Plex")
	}
	if svc.Category != CatMedia {
		t.Errorf("Category = %q, want %q", svc.Category, CatMedia)
	}
}

func TestLookupByKey_Unknown(t *testing.T) {
	_, ok := LookupByKey("does-not-exist")
	if ok {
		t.Error("expected not found for unknown key")
	}
}

// ── LookupByDockerImage ──────────────────────────────────────────────────────

func TestLookupByDockerImage_Exact(t *testing.T) {
	svc, ok := LookupByDockerImage("jellyfin/jellyfin")
	if !ok {
		t.Fatal("expected match for jellyfin/jellyfin")
	}
	if svc.Key != "jellyfin" {
		t.Errorf("Key = %q, want jellyfin", svc.Key)
	}
}

func TestLookupByDockerImage_WithTag(t *testing.T) {
	svc, ok := LookupByDockerImage("linuxserver/plex:latest")
	if !ok {
		t.Fatal("expected match for linuxserver/plex:latest")
	}
	if svc.Key != "plex" {
		t.Errorf("Key = %q, want plex", svc.Key)
	}
}

func TestLookupByDockerImage_DockerHubPrefix(t *testing.T) {
	svc, ok := LookupByDockerImage("docker.io/jellyfin/jellyfin:10")
	if !ok {
		t.Fatal("expected match after stripping docker.io prefix")
	}
	if svc.Key != "jellyfin" {
		t.Errorf("Key = %q, want jellyfin", svc.Key)
	}
}

func TestLookupByDockerImage_WithDigest(t *testing.T) {
	svc, ok := LookupByDockerImage("linuxserver/sonarr@sha256:abcdef1234567890")
	if !ok {
		t.Fatal("expected match after stripping digest")
	}
	if svc.Key != "sonarr" {
		t.Errorf("Key = %q, want sonarr", svc.Key)
	}
}

func TestLookupByDockerImage_Unknown(t *testing.T) {
	_, ok := LookupByDockerImage("nobody/nothing:v1")
	if ok {
		t.Error("expected no match for unknown image")
	}
}

func TestLookupByDockerImage_Empty(t *testing.T) {
	_, ok := LookupByDockerImage("")
	if ok {
		t.Error("expected no match for empty image")
	}
}

// ── LookupByHint ─────────────────────────────────────────────────────────────

func TestLookupByHint_Name(t *testing.T) {
	svc, ok := LookupByHint("Grafana")
	if !ok {
		t.Fatal("expected hint match for 'Grafana'")
	}
	if svc.Key != "grafana" {
		t.Errorf("Key = %q, want grafana", svc.Key)
	}
}

func TestLookupByHint_Domain(t *testing.T) {
	svc, ok := LookupByHint("http://plex.local:32400")
	if !ok {
		t.Fatal("expected hint match for plex URL")
	}
	if svc.Key != "plex" {
		t.Errorf("Key = %q, want plex", svc.Key)
	}
}

func TestLookupByHint_ContainerName(t *testing.T) {
	// Container names like "homelab-sonarr-1" should resolve via token matching
	svc, ok := LookupByHint("sonarr")
	if !ok {
		t.Fatal("expected hint match for container name 'sonarr'")
	}
	if svc.Key != "sonarr" {
		t.Errorf("Key = %q, want sonarr", svc.Key)
	}
}

func TestLookupByHint_Unknown(t *testing.T) {
	_, ok := LookupByHint("completelyunknownservice12345")
	if ok {
		t.Error("expected no hint match for unknown service")
	}
}

// ── LookupByPort ─────────────────────────────────────────────────────────────

func TestLookupByPort_Known(t *testing.T) {
	// Port 32400 is unique to Plex
	svc, ok := LookupByPort(32400)
	if !ok {
		t.Fatal("expected port match for 32400")
	}
	if svc.Key != "plex" {
		t.Errorf("Key = %q, want plex", svc.Key)
	}
}

func TestLookupByPort_Unknown(t *testing.T) {
	_, ok := LookupByPort(1)
	if ok {
		t.Error("expected no match for port 1")
	}
}

func TestLookupUniqueByPort_Unique(t *testing.T) {
	svc, ok := LookupUniqueByPort(32400)
	if !ok {
		t.Fatal("port 32400 should be unique to Plex")
	}
	if svc.Key != "plex" {
		t.Errorf("Key = %q, want plex", svc.Key)
	}
}

// ── AllCategories ─────────────────────────────────────────────────────────────

func TestAllCategories(t *testing.T) {
	cats := AllCategories()
	if len(cats) == 0 {
		t.Fatal("AllCategories should not be empty")
	}

	// Result should be sorted
	for i := 1; i < len(cats); i++ {
		if cats[i] < cats[i-1] {
			t.Errorf("categories not sorted at index %d: %q < %q", i, cats[i], cats[i-1])
		}
	}

	// All well-known categories should be present
	expected := []string{CatMedia, CatMonitoring, CatNetworking, CatDatabases}
	catSet := make(map[string]bool, len(cats))
	for _, c := range cats {
		catSet[c] = true
	}
	for _, e := range expected {
		if !catSet[e] {
			t.Errorf("expected category %q not found in AllCategories()", e)
		}
	}
}

// ── normalizeDockerImage (internal, tested via white-box) ───────────────────

func TestNormalizeDockerImage(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"traefik:v3.0", "traefik"},
		{"linuxserver/plex:latest", "linuxserver/plex"},
		{"docker.io/library/nginx:1.25", "nginx"},
		{"registry-1.docker.io/library/redis", "redis"},
		{"ghcr.io/jellyfin/jellyfin:10.8", "jellyfin/jellyfin"},
		{"linuxserver/sonarr@sha256:abc123", "linuxserver/sonarr"},
		{"UPPERCASE/Image:Tag", "uppercase/image"},
	}
	for _, tc := range cases {
		got := normalizeDockerImage(tc.input)
		if got != tc.want {
			t.Errorf("normalizeDockerImage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
