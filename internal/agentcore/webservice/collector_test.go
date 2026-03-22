package webservice

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/agentmgr"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type mockCollectorTransport struct {
	mu        sync.Mutex
	connected bool
	sends     int
}

func (m *mockCollectorTransport) Send(agentmgr.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends++
	return nil
}

func (m *mockCollectorTransport) Connect(context.Context) error { return nil }

func (m *mockCollectorTransport) Receive() (agentmgr.Message, error) {
	return agentmgr.Message{}, nil
}

func (m *mockCollectorTransport) Connected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockCollectorTransport) Close() {}

func TestMakeServiceID(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		id1 := makeServiceID("192.168.1.10", "docker", "abc123")
		id2 := makeServiceID("192.168.1.10", "docker", "abc123")
		if id1 != id2 {
			t.Errorf("expected deterministic IDs, got %q vs %q", id1, id2)
		}
	})

	t.Run("16 hex chars", func(t *testing.T) {
		id := makeServiceID("host", "source", "id")
		if len(id) != 16 {
			t.Errorf("expected 16-char hex ID, got %d chars: %q", len(id), id)
		}
	})

	t.Run("different inputs produce different IDs", func(t *testing.T) {
		id1 := makeServiceID("192.168.1.10", "docker", "container-a")
		id2 := makeServiceID("192.168.1.10", "docker", "container-b")
		id3 := makeServiceID("192.168.1.20", "docker", "container-a")
		id4 := makeServiceID("192.168.1.10", "systemd", "container-a")

		seen := map[string]bool{id1: true}
		for _, id := range []string{id2, id3, id4} {
			if seen[id] {
				t.Errorf("collision detected: %q appeared more than once", id)
			}
			seen[id] = true
		}
	})
}

func TestBuildServiceURL(t *testing.T) {
	tests := []struct {
		name   string
		hostIP string
		port   int
		want   string
	}{
		{"http standard", "192.168.1.10", 8080, "http://192.168.1.10:8080"},
		{"https 443", "192.168.1.10", 443, "https://192.168.1.10:443"},
		{"https 8443", "10.0.0.1", 8443, "https://10.0.0.1:8443"},
		{"https 9443", "10.0.0.1", 9443, "https://10.0.0.1:9443"},
		{"https 10443", "10.0.0.1", 10443, "https://10.0.0.1:10443"},
		{"https 8006", "10.0.0.1", 8006, "https://10.0.0.1:8006"},
		{"https 8007", "10.0.0.1", 8007, "https://10.0.0.1:8007"},
		{"empty hostIP defaults to localhost", "", 3000, "http://localhost:3000"},
		{"plex port", "192.168.1.5", 32400, "http://192.168.1.5:32400"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildServiceURL(tt.hostIP, tt.port)
			if got != tt.want {
				t.Errorf("buildServiceURL(%q, %d) = %q, want %q", tt.hostIP, tt.port, got, tt.want)
			}
		})
	}
}

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		name  string
		ports []dockerpkg.DockerPort
		want  int
	}{
		{
			"public port mapped",
			[]dockerpkg.DockerPort{{IP: "0.0.0.0", PrivatePort: 32400, PublicPort: 32400, Type: "tcp"}},
			32400,
		},
		{
			"multiple ports returns first public",
			[]dockerpkg.DockerPort{
				{IP: "", PrivatePort: 8080, PublicPort: 0, Type: "tcp"},
				{IP: "0.0.0.0", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
			},
			8443,
		},
		{
			"no public port",
			[]dockerpkg.DockerPort{{IP: "", PrivatePort: 8080, PublicPort: 0, Type: "tcp"}},
			0,
		},
		{
			"empty ports",
			nil,
			0,
		},
		{
			"different host and container port",
			[]dockerpkg.DockerPort{{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 9090, Type: "tcp"}},
			9090,
		},
		{
			"udp port ignored",
			[]dockerpkg.DockerPort{{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 53, Type: "udp"}},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHostPort(tt.ports)
			if got != tt.want {
				t.Errorf("extractHostPort(%v) = %d, want %d", tt.ports, got, tt.want)
			}
		})
	}
}

func TestExtractHostPortForService(t *testing.T) {
	t.Run("known non-web default prefers web port", func(t *testing.T) {
		ports := []dockerpkg.DockerPort{
			{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 53, Type: "tcp"},
			{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		}
		known := &KnownService{DefaultPort: 53}
		got := extractHostPortForService(ports, known)
		if got != 8080 {
			t.Fatalf("extractHostPortForService() = %d, want %d", got, 8080)
		}
	})

	t.Run("known web default uses mapped port", func(t *testing.T) {
		ports := []dockerpkg.DockerPort{
			{IP: "0.0.0.0", PrivatePort: 8080, PublicPort: 18080, Type: "tcp"},
			{IP: "0.0.0.0", PrivatePort: 9000, PublicPort: 9000, Type: "tcp"},
		}
		known := &KnownService{DefaultPort: 8080}
		got := extractHostPortForService(ports, known)
		if got != 18080 {
			t.Fatalf("extractHostPortForService() = %d, want %d", got, 18080)
		}
	})

	t.Run("unknown service prefers likely web port", func(t *testing.T) {
		ports := []dockerpkg.DockerPort{
			{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 53, Type: "tcp"},
			{IP: "0.0.0.0", PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
		}
		got := extractHostPortForService(ports, nil)
		if got != 3000 {
			t.Fatalf("extractHostPortForService() = %d, want %d", got, 3000)
		}
	})
}

func TestParsePortList(t *testing.T) {
	got := parsePortList(" 8080,443;8080 bad 65536 0 80 ")
	want := []int{80, 443, 8080}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePortList() = %v, want %v", got, want)
	}
}

func TestCountDiscoveredServicesBySource(t *testing.T) {
	services := []agentmgr.DiscoveredWebService{
		{Source: "docker"},
		{Source: "Docker"},
		{Source: "proxy"},
		{Source: "scan"},
		{Source: "scan"},
		{Source: ""},
	}

	got := countDiscoveredServicesBySource(services)
	want := map[string]int{
		"docker":  2,
		"proxy":   1,
		"scan":    2,
		"unknown": 1,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("countDiscoveredServicesBySource() = %#v, want %#v", got, want)
	}
}

func TestParseProcNetListeningPorts(t *testing.T) {
	raw := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000   100        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:0568 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 23456 1 0000000000000000 100 0 0 10 0
   2: 0100007F:0035 00000000:0000 01 00000000:00000000 00:00000000 00000000   100        0 34567 1 0000000000000000 100 0 0 10 0
`

	got := parseProcNetListeningPorts(raw)
	want := []int{8080, 1384}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseProcNetListeningPorts() = %v, want %v", got, want)
	}
}

func TestBuildServicesFromContainersIncludesImageMetadata(t *testing.T) {
	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "abc123def4567890",
			Names: []string{"/grafana"},
			Image: "grafana/grafana:latest",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{IP: "0.0.0.0", PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
			},
		},
	}

	services := wsc.buildServicesFromContainers(containers)
	if len(services) != 1 {
		t.Fatalf("buildServicesFromContainers() returned %d services, want 1", len(services))
	}

	if services[0].Source != "docker" {
		t.Fatalf("source = %q, want docker", services[0].Source)
	}
	if services[0].Metadata == nil {
		t.Fatal("expected metadata to be present")
	}
	if services[0].Metadata["image"] != "grafana/grafana:latest" {
		t.Fatalf("image metadata = %q, want %q", services[0].Metadata["image"], "grafana/grafana:latest")
	}
}

func TestBuildServicesFromContainersClassifiesByContainerNameHint(t *testing.T) {
	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "abc123def4567890",
			Names: []string{"/my_grafana_1"},
			Image: "registry.local/custom/grafana-build:latest",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{IP: "0.0.0.0", PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
			},
		},
	}

	services := wsc.buildServicesFromContainers(containers)
	if len(services) != 1 {
		t.Fatalf("buildServicesFromContainers() returned %d services, want 1", len(services))
	}
	if services[0].ServiceKey != "grafana" {
		t.Fatalf("service key = %q, want %q", services[0].ServiceKey, "grafana")
	}
	if services[0].Name != "Grafana" {
		t.Fatalf("name = %q, want %q", services[0].Name, "Grafana")
	}
	if services[0].Category != CatMonitoring {
		t.Fatalf("category = %q, want %q", services[0].Category, CatMonitoring)
	}
}

func TestScanCandidatePortsCanDisableListeningAugment(t *testing.T) {
	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_PORTS", "9010,9011")
	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_INCLUDE_LISTENING", "false")

	got := scanCandidatePorts()
	want := []int{9010, 9011}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scanCandidatePorts() = %v, want %v", got, want)
	}
}

func TestDiscoverPortScannedServices(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	parsed, err := neturl.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_DISABLED", "false")
	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_PORTS", strconv.Itoa(port))

	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	services := wsc.discoverPortScannedServices(context.Background(), nil)
	if len(services) != 1 {
		t.Fatalf("discoverPortScannedServices() returned %d services, want 1", len(services))
	}
	if services[0].Source != "scan" {
		t.Fatalf("service source = %q, want scan", services[0].Source)
	}
	if services[0].Metadata["public_port"] != strconv.Itoa(port) {
		t.Fatalf("public_port metadata = %q, want %d", services[0].Metadata["public_port"], port)
	}

	alreadyKnown := []agentmgr.DiscoveredWebService{
		{
			URL: buildServiceURL("127.0.0.1", port),
		},
	}
	services = wsc.discoverPortScannedServices(context.Background(), alreadyKnown)
	if len(services) != 0 {
		t.Fatalf("discoverPortScannedServices() returned %d services with existing port, want 0", len(services))
	}
}

func TestDiscoverPortScannedServicesSkipsPortsFromServiceMetadata(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	parsed, err := neturl.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_DISABLED", "false")
	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_PORTS", strconv.Itoa(port))
	t.Setenv("LABTETHER_WEBSVC_PORTSCAN_INCLUDE_LISTENING", "false")

	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	alreadyKnown := []agentmgr.DiscoveredWebService{
		{
			URL: "https://plex.home.lab",
			Metadata: map[string]string{
				"backend_url": "http://127.0.0.1:" + strconv.Itoa(port),
				"public_port": strconv.Itoa(port),
			},
		},
	}

	services := wsc.discoverPortScannedServices(context.Background(), alreadyKnown)
	if len(services) != 0 {
		t.Fatalf("discoverPortScannedServices() returned %d services with metadata-known port, want 0", len(services))
	}
}

func TestDiscoverLANScannedServices(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	parsed, err := neturl.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
		discoveryCfg: WebServiceDiscoveryConfig{
			LANScanEnabled:  true,
			LANScanCIDRs:    "127.0.0.1/32",
			LANScanPorts:    strconv.Itoa(port),
			LANScanMaxHosts: 8,
		},
	}

	services := wsc.discoverLANScannedServices(context.Background(), nil)
	if len(services) != 1 {
		t.Fatalf("discoverLANScannedServices() returned %d services, want 1", len(services))
	}
	if services[0].Source != "scan" {
		t.Fatalf("service source = %q, want scan", services[0].Source)
	}
	if services[0].Metadata["scan_scope"] != "lan" {
		t.Fatalf("scan_scope metadata = %q, want lan", services[0].Metadata["scan_scope"])
	}
	if services[0].Metadata["scan_target_host"] != "127.0.0.1" {
		t.Fatalf("scan_target_host metadata = %q, want 127.0.0.1", services[0].Metadata["scan_target_host"])
	}

	alreadyKnown := []agentmgr.DiscoveredWebService{
		{
			URL: buildServiceURL("127.0.0.1", port),
		},
	}
	services = wsc.discoverLANScannedServices(context.Background(), alreadyKnown)
	if len(services) != 0 {
		t.Fatalf("discoverLANScannedServices() returned %d services with existing endpoint, want 0", len(services))
	}
}

func TestScannedPortMetadataAmbiguousPortUsesGenericValues(t *testing.T) {
	if _, found := LookupByPort(3000); !found {
		t.Fatal("expected legacy LookupByPort(3000) match for ambiguous-port precondition")
	}
	if _, found := LookupUniqueByPort(3000); found {
		t.Fatal("expected unique lookup for port 3000 to be unresolved")
	}

	name, category, iconKey, serviceKey, healthPath, known := scannedPortMetadata(3000)
	if known {
		t.Fatal("expected ambiguous port metadata to be treated as unknown")
	}
	if name != "Port 3000" {
		t.Fatalf("name = %q, want %q", name, "Port 3000")
	}
	if category != CatOther {
		t.Fatalf("category = %q, want %q", category, CatOther)
	}
	if iconKey != "" {
		t.Fatalf("iconKey = %q, want empty", iconKey)
	}
	if serviceKey != "" {
		t.Fatalf("serviceKey = %q, want empty", serviceKey)
	}
	if healthPath != "" {
		t.Fatalf("healthPath = %q, want empty", healthPath)
	}
}

func TestApplyFingerprintMetadataLabTetherBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"labtether","status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name:     "Port 8443",
		Category: CatOther,
		URL:      server.URL,
		Source:   "scan",
	}

	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "labtether" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "labtether")
	}
	if svc.Name != "LabTether" {
		t.Fatalf("name = %q, want %q", svc.Name, "LabTether")
	}
	if svc.Category != CatManagement {
		t.Fatalf("category = %q, want %q", svc.Category, CatManagement)
	}
	if svc.Metadata["health_path"] != "/healthz" {
		t.Fatalf("health_path = %q, want %q", svc.Metadata["health_path"], "/healthz")
	}
}

func TestApplyFingerprintMetadataLabTetherFrontend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/login":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>LabTether Console</title></head><body>LabTether</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name:     "Port 3000",
		Category: CatOther,
		URL:      server.URL,
		Source:   "scan",
	}

	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "labtether" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "labtether")
	}
	if svc.Name != "LabTether" {
		t.Fatalf("name = %q, want %q", svc.Name, "LabTether")
	}
	if svc.Category != CatManagement {
		t.Fatalf("category = %q, want %q", svc.Category, CatManagement)
	}
}

func TestApplyFingerprintMetadataLabTetherFrontendWithProtectedHealthRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		case "/api/auth/login":
			w.WriteHeader(http.StatusMethodNotAllowed)
		case "/login":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>LabTether Console</title></head><body>LabTether</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name:     "Port 3000",
		Category: CatOther,
		URL:      server.URL,
		Source:   "scan",
	}

	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "labtether" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "labtether")
	}
	if svc.Name != "LabTether" {
		t.Fatalf("name = %q, want %q", svc.Name, "LabTether")
	}
	if svc.Category != CatManagement {
		t.Fatalf("category = %q, want %q", svc.Category, CatManagement)
	}
}

func TestApplyFingerprintMetadataDoesNotMisclassifyGrafanaLikeHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"database":"ok","version":"11.0.0"}`))
		case "/login":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>Grafana</title></head><body>grafana</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name:     "Port 3000",
		Category: CatOther,
		URL:      server.URL,
		Source:   "scan",
	}

	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "" {
		t.Fatalf("serviceKey = %q, want empty", svc.ServiceKey)
	}
	if svc.Name != "Port 3000" {
		t.Fatalf("name = %q, want %q", svc.Name, "Port 3000")
	}
}

func TestApplyFingerprintMetadataAppliesCompatibilityFromServiceKey(t *testing.T) {
	wsc := &WebServiceCollector{}
	svc := agentmgr.DiscoveredWebService{
		Name:       "Portainer",
		Category:   CatManagement,
		URL:        "https://10.0.0.5:9443",
		Source:     "docker",
		ServiceKey: "portainer",
	}

	wsc.applyFingerprintMetadata(&svc)

	if svc.Metadata == nil {
		t.Fatal("expected metadata to be initialized")
	}
	if svc.Metadata["compat_connector"] != "portainer" {
		t.Fatalf("compat_connector = %q, want %q", svc.Metadata["compat_connector"], "portainer")
	}
	if svc.Metadata["compat_confidence"] == "" {
		t.Fatal("expected compat_confidence metadata")
	}
	if svc.Metadata["compat_auth_hint"] == "" {
		t.Fatal("expected compat_auth_hint metadata")
	}
}

func TestApplyFingerprintMetadataDetectsHomeAssistantCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"API running."}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name:     "Port 8123",
		Category: CatOther,
		URL:      server.URL,
		Source:   "scan",
	}

	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "homeassistant" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "homeassistant")
	}
	if svc.Name != "Home Assistant" {
		t.Fatalf("name = %q, want %q", svc.Name, "Home Assistant")
	}
	if svc.Metadata == nil {
		t.Fatal("expected metadata to be initialized")
	}
	if svc.Metadata["compat_connector"] != "homeassistant" {
		t.Fatalf("compat_connector = %q, want %q", svc.Metadata["compat_connector"], "homeassistant")
	}
	if svc.Metadata["compat_profile"] != "homeassistant.api.root" {
		t.Fatalf("compat_profile = %q, want %q", svc.Metadata["compat_profile"], "homeassistant.api.root")
	}
}

func TestNormalizeLabTetherServicesHidesAPIWhenConsoleExists(t *testing.T) {
	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-console",
			ServiceKey:  "labtether",
			Name:        "LabTether",
			Category:    CatManagement,
			URL:         "https://10.0.0.5:3000",
			Source:      "scan",
			HostAssetID: "host-a",
			Metadata: map[string]string{
				"public_port": "3000",
			},
		},
		{
			ID:          "svc-api",
			ServiceKey:  "labtether",
			Name:        "LabTether",
			Category:    CatManagement,
			URL:         "https://10.0.0.5:8443",
			Source:      "scan",
			HostAssetID: "host-a",
			Metadata: map[string]string{
				"public_port": "8443",
			},
		},
	}

	normalizeLabTetherServices(services)

	if services[0].Name != "LabTether Console" {
		t.Fatalf("console name = %q, want %q", services[0].Name, "LabTether Console")
	}
	if services[0].Metadata["labtether_component"] != labtetherConsole {
		t.Fatalf("console component = %q, want %q", services[0].Metadata["labtether_component"], labtetherConsole)
	}
	if services[0].Metadata["hidden"] == "true" {
		t.Fatalf("console hidden = %q, want not hidden", services[0].Metadata["hidden"])
	}

	if services[1].Name != "LabTether API" {
		t.Fatalf("api name = %q, want %q", services[1].Name, "LabTether API")
	}
	if services[1].Metadata["labtether_component"] != labtetherAPI {
		t.Fatalf("api component = %q, want %q", services[1].Metadata["labtether_component"], labtetherAPI)
	}
	if services[1].Metadata["hidden"] != "true" {
		t.Fatalf("api hidden = %q, want %q", services[1].Metadata["hidden"], "true")
	}
}

func TestNormalizeLabTetherServicesShowsAPIWithoutConsole(t *testing.T) {
	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-api",
			ServiceKey:  "labtether",
			Name:        "LabTether",
			Category:    CatManagement,
			URL:         "https://10.0.0.5:8443",
			Source:      "scan",
			HostAssetID: "host-a",
			Metadata: map[string]string{
				"public_port": "8443",
			},
		},
	}

	normalizeLabTetherServices(services)

	if services[0].Name != "LabTether API" {
		t.Fatalf("api name = %q, want %q", services[0].Name, "LabTether API")
	}
	if services[0].Metadata["labtether_component"] != labtetherAPI {
		t.Fatalf("api component = %q, want %q", services[0].Metadata["labtether_component"], labtetherAPI)
	}
	if services[0].Metadata["hidden"] == "true" {
		t.Fatalf("api hidden = %q, want visible", services[0].Metadata["hidden"])
	}
}

func TestAlternateSchemeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"http to https", "http://10.0.0.10:8080", "https://10.0.0.10:8080"},
		{"https to http", "https://10.0.0.10:8443", "http://10.0.0.10:8443"},
		{"unsupported scheme", "tcp://10.0.0.10:22", ""},
		{"invalid url", "://bad", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alternateSchemeURL(tt.in)
			if got != tt.want {
				t.Fatalf("alternateSchemeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestProbeHealthURLSkipsAlternateWhenPrimaryResponds(t *testing.T) {
	var (
		mu      sync.Mutex
		schemes []string
	)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			schemes = append(schemes, req.URL.Scheme)
			mu.Unlock()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		}),
	}
	wsc := &WebServiceCollector{client: client}

	result := wsc.probeHealthURL("https://example.local:8443", "/healthz")
	if !result.responded {
		t.Fatal("expected primary probe to respond")
	}
	if result.baseURL != "https://example.local:8443" {
		t.Fatalf("baseURL = %q, want primary URL", result.baseURL)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(schemes) != 1 {
		t.Fatalf("expected one probe request, got %d (%v)", len(schemes), schemes)
	}
	if schemes[0] != "https" {
		t.Fatalf("expected https primary probe, got %q", schemes[0])
	}
}

func TestDoHealthRequestPrefersInsecureClientForHTTPS(t *testing.T) {
	var secureCalls int
	var insecureCalls int

	wsc := &WebServiceCollector{
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				secureCalls++
				return nil, io.ErrUnexpectedEOF
			}),
		},
		insecureClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				insecureCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("ok")),
					Request:    req,
				}, nil
			}),
		},
	}

	status, ok := wsc.doHealthRequest(http.MethodGet, "https://example.local:8443/healthz")
	if !ok {
		t.Fatal("expected successful health request")
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if insecureCalls != 1 {
		t.Fatalf("insecure calls = %d, want 1", insecureCalls)
	}
	if secureCalls != 0 {
		t.Fatalf("secure calls = %d, want 0", secureCalls)
	}
}

func TestDoBodyRequestPrefersInsecureClientForHTTPS(t *testing.T) {
	var secureCalls int
	var insecureCalls int

	wsc := &WebServiceCollector{
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				secureCalls++
				return nil, io.ErrUnexpectedEOF
			}),
		},
		insecureClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				insecureCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Request:    req,
				}, nil
			}),
		},
	}

	body, status, ok := wsc.doBodyRequest(http.MethodGet, "https://example.local:8443/api/health")
	if !ok {
		t.Fatal("expected successful body request")
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if strings.TrimSpace(string(body)) != `{"ok":true}` {
		t.Fatalf("body = %q, want %q", string(body), `{"ok":true}`)
	}
	if insecureCalls != 1 {
		t.Fatalf("insecure calls = %d, want 1", insecureCalls)
	}
	if secureCalls != 0 {
		t.Fatalf("secure calls = %d, want 0", secureCalls)
	}
}

func TestFingerprintKnownServiceSkipsAlternateWhenPrimaryResponds(t *testing.T) {
	var (
		mu      sync.Mutex
		schemes []string
	)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			schemes = append(schemes, req.URL.Scheme)
			mu.Unlock()
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("not found")),
				Request:    req,
			}, nil
		}),
	}
	wsc := &WebServiceCollector{client: client}

	if _, ok := wsc.fingerprintKnownService("https://example.local:8443"); ok {
		t.Fatal("expected unknown service fingerprint")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, scheme := range schemes {
		if scheme != "https" {
			t.Fatalf("unexpected alternate probe scheme %q (all requests: %v)", scheme, schemes)
		}
	}
}

func TestFingerprintKnownServiceFallsBackToAlternateWhenPrimaryUnreachable(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Scheme == "https" {
				return nil, io.ErrUnexpectedEOF
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<html><body>Proxmox Virtual Environment</body></html>`)),
				Request:    req,
			}, nil
		}),
	}
	wsc := &WebServiceCollector{client: client}

	known, ok := wsc.fingerprintKnownService("https://example.local:8443")
	if !ok {
		t.Fatal("expected alternate-scheme fallback to classify service")
	}
	if known.Key != "proxmox" {
		t.Fatalf("known key = %q, want proxmox", known.Key)
	}
}

func TestHealthCheckSwitchesToHTTPS(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer tlsServer.Close()

	parsed, err := neturl.Parse(tlsServer.URL)
	if err != nil {
		t.Fatalf("parse tls server url: %v", err)
	}

	svc := agentmgr.DiscoveredWebService{
		URL: "http://" + parsed.Host,
	}
	wsc := &WebServiceCollector{
		client: &http.Client{
			Timeout:   2 * time.Second,
			Transport: tlsServer.Client().Transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}

	wsc.healthCheck(&svc)

	if svc.Status != "up" {
		t.Fatalf("status = %q, want up", svc.Status)
	}
	if !strings.HasPrefix(svc.URL, "https://") {
		t.Fatalf("url = %q, want https://...", svc.URL)
	}
}

func TestHealthCheckSwitchesToHTTP(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	parsed, err := neturl.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse http server url: %v", err)
	}

	svc := agentmgr.DiscoveredWebService{
		URL: "https://" + parsed.Host,
	}
	wsc := &WebServiceCollector{
		client: &http.Client{
			Timeout: 2 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}

	wsc.healthCheck(&svc)

	if svc.Status != "up" {
		t.Fatalf("status = %q, want up", svc.Status)
	}
	if !strings.HasPrefix(svc.URL, "http://") {
		t.Fatalf("url = %q, want http://...", svc.URL)
	}
}

func TestApplyHealthCheckWithCacheReusesRecentProbe(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		interval:       time.Minute,
		client:         server.Client(),
		insecureClient: server.Client(),
		healthCache:    make(map[string]healthCacheEntry),
	}
	svc := agentmgr.DiscoveredWebService{
		ID:  "svc-1",
		URL: server.URL,
	}

	now := time.Now().UTC()
	wsc.applyHealthCheckWithCache(&svc, now)
	if svc.Status != "up" {
		t.Fatalf("first status = %q, want up", svc.Status)
	}

	wsc.applyHealthCheckWithCache(&svc, now.Add(30*time.Second))
	if svc.Status != "up" {
		t.Fatalf("second status = %q, want up", svc.Status)
	}

	mu.Lock()
	got := requests
	mu.Unlock()
	if got != 1 {
		t.Fatalf("probe requests = %d, want 1", got)
	}
}

func TestApplyHealthCheckWithCacheReprobesAfterTTL(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		interval:       time.Minute,
		client:         server.Client(),
		insecureClient: server.Client(),
		healthCache:    make(map[string]healthCacheEntry),
	}
	svc := agentmgr.DiscoveredWebService{
		ID:  "svc-ttl",
		URL: server.URL,
	}

	now := time.Now().UTC()
	wsc.applyHealthCheckWithCache(&svc, now)
	wsc.applyHealthCheckWithCache(&svc, now.Add(4*time.Minute))

	mu.Lock()
	got := requests
	mu.Unlock()
	if got != 2 {
		t.Fatalf("probe requests = %d, want 2", got)
	}
}

func TestRunCycleSkipsWhenTransportDisconnected(t *testing.T) {
	transport := &mockCollectorTransport{connected: false}
	wsc := &WebServiceCollector{
		transport:    transport,
		assetID:      "asset-1",
		interval:     time.Minute,
		discoveryCfg: WebServiceDiscoveryConfig{},
	}

	wsc.RunCycle(context.Background())
	if transport.sends != 0 {
		t.Fatalf("disconnected sends = %d, want 0", transport.sends)
	}

	transport.connected = true
	wsc.RunCycle(context.Background())
	if transport.sends != 1 {
		t.Fatalf("connected sends = %d, want 1", transport.sends)
	}
}

func TestCleanContainerName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips leading slash", "/plex", "plex"},
		{"no slash unchanged", "grafana", "grafana"},
		{"empty string", "", ""},
		{"only slash", "/", ""},
		{"nested path preserved", "/compose/service", "compose/service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanContainerName(tt.input)
			if got != tt.want {
				t.Errorf("cleanContainerName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildServicesFromContainersReadsLabTetherLabels(t *testing.T) {
	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "abc123def4567890",
			Names: []string{"/custom-app"},
			Image: "my-private/custom-app:latest",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{IP: "0.0.0.0", PrivatePort: 8080, PublicPort: 8080, Type: "tcp"},
			},
			Labels: map[string]string{
				"labtether.category": "Productivity",
				"labtether.icon":     "custom-app",
				"labtether.name":     "My Custom App",
			},
		},
	}

	services := wsc.buildServicesFromContainers(containers)
	if len(services) != 1 {
		t.Fatalf("got %d services, want 1", len(services))
	}

	svc := services[0]
	if svc.Name != "My Custom App" {
		t.Errorf("name = %q, want %q", svc.Name, "My Custom App")
	}
	if svc.Category != "Productivity" {
		t.Errorf("category = %q, want %q", svc.Category, "Productivity")
	}
	if svc.IconKey != "custom-app" {
		t.Errorf("icon_key = %q, want %q", svc.IconKey, "custom-app")
	}
}

func TestBuildServicesFromContainersLabTetherHiddenLabel(t *testing.T) {
	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "abc123def4567890",
			Names: []string{"/hidden-app"},
			Image: "some/app:latest",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{IP: "0.0.0.0", PrivatePort: 9090, PublicPort: 9090, Type: "tcp"},
			},
			Labels: map[string]string{
				"labtether.hidden": "true",
			},
		},
	}

	services := wsc.buildServicesFromContainers(containers)
	if len(services) != 1 {
		t.Fatalf("got %d services, want 1", len(services))
	}
	if services[0].Metadata == nil || services[0].Metadata["hidden"] != "true" {
		t.Errorf("expected hidden metadata to be 'true', got %v", services[0].Metadata)
	}
}

func TestBuildServicesFromContainersLabTetherLabelsOverrideAutoDetection(t *testing.T) {
	wsc := &WebServiceCollector{
		assetID: "asset-1",
		hostIP:  "127.0.0.1",
	}

	// Grafana is auto-detected as Monitoring, but label overrides to Development
	containers := []dockerpkg.DockerContainer{
		{
			ID:    "abc123def4567890",
			Names: []string{"/grafana"},
			Image: "grafana/grafana:latest",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{IP: "0.0.0.0", PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
			},
			Labels: map[string]string{
				"labtether.category": "Development",
				"labtether.name":     "Dev Grafana",
			},
		},
	}

	services := wsc.buildServicesFromContainers(containers)
	if len(services) != 1 {
		t.Fatalf("got %d services, want 1", len(services))
	}
	if services[0].Category != "Development" {
		t.Errorf("category = %q, want %q", services[0].Category, "Development")
	}
	if services[0].Name != "Dev Grafana" {
		t.Errorf("name = %q, want %q", services[0].Name, "Dev Grafana")
	}
	// Icon should still be grafana since no label override was set for icon
	if services[0].IconKey != "grafana" {
		t.Errorf("icon_key = %q, want %q", services[0].IconKey, "grafana")
	}
}

func TestExtractTraefikURL(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			"standard traefik v2 rule",
			map[string]string{
				"traefik.http.routers.plex.rule": "Host(`plex.example.com`)",
			},
			"https://plex.example.com",
		},
		{
			"no traefik labels",
			map[string]string{
				"com.docker.compose.project": "mystack",
			},
			"",
		},
		{
			"nil labels",
			nil,
			"",
		},
		{
			"traefik label without Host rule",
			map[string]string{
				"traefik.http.routers.myapp.entrypoints": "websecure",
			},
			"",
		},
		{
			"complex host rule with path",
			map[string]string{
				"traefik.http.routers.grafana.rule": "Host(`grafana.home.lan`) && PathPrefix(`/`)",
			},
			"https://grafana.home.lan",
		},
		{
			"empty labels map",
			map[string]string{},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTraefikURL(tt.labels)
			if got != tt.want {
				t.Errorf("extractTraefikURL(%v) = %q, want %q", tt.labels, got, tt.want)
			}
		})
	}
}

func TestMatchHTMLFingerprint(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"truenas root page", `<html><head><title>TrueNAS</title></head></html>`, "truenas"},
		{"truenas in body", `<html><body>Welcome to TrueNAS SCALE</body></html>`, "truenas"},
		{"synology dsm", `<html><head><title>Synology DiskStation</title></head></html>`, "synology"},
		{"synology keyword", `<html><body>Synology NAS</body></html>`, "synology"},
		{"diskstation keyword", `<html><body>DiskStation Manager</body></html>`, "synology"},
		{"qnap qts", `<html><body>QNAP Systems, Inc.</body></html>`, "qnap"},
		{"proxmox backup server", `<html><head><title>Proxmox Backup Server</title></head></html>`, "proxmox-backup"},
		{"proxmox ve", `<html><head><title>Proxmox Virtual Environment</title></head></html>`, "proxmox"},
		{"pve manager", `<html><body>PVE Manager</body></html>`, "proxmox"},
		{"pfsense", `<html><head><title>pfSense - Login</title></head></html>`, "pfsense"},
		{"opnsense", `<html><head><title>OPNsense - Login</title></head></html>`, "opnsense"},
		{"home assistant", `<html><body><home-assistant></home-assistant></body></html>`, "homeassistant"},
		{"pihole", `<html><head><title>Pi-hole Admin Console</title></head></html>`, "pihole"},
		{"unifi network", `<html><body>UniFi Network Application</body></html>`, "unifi"},
		{"cockpit ws", `<html><head><base href="/cockpit/@localhost/"></head></html>`, "cockpit"},
		{"unknown service", `<html><head><title>My App</title></head></html>`, ""},
		{"empty body", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchHTMLFingerprint([]byte(tt.html))
			if got != tt.want {
				t.Errorf("matchHTMLFingerprint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchHTMLFingerprintPBSBeforePVE(t *testing.T) {
	// PBS page should match "proxmox-backup" not "proxmox" even though both contain "proxmox".
	html := `<html><head><title>Proxmox Backup Server</title></head><body>Proxmox Backup Server Management</body></html>`
	got := matchHTMLFingerprint([]byte(html))
	if got != "proxmox-backup" {
		t.Errorf("PBS page matched %q, want %q", got, "proxmox-backup")
	}
}

func TestFingerprintByHTTPTrueNAS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>TrueNAS - Storage</title></head></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name: "Port 80", Category: CatOther, URL: server.URL, Source: "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "truenas" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "truenas")
	}
	if svc.Category != CatStorage {
		t.Fatalf("category = %q, want %q", svc.Category, CatStorage)
	}
}

func TestFingerprintByHTTPTrueNASRedirect(t *testing.T) {
	// TrueNAS redirects / to /ui/ — fingerprinting should follow the redirect target.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
		case "/ui/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>TrueNAS</title></head></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client: &http.Client{
			Timeout:       2 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		},
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name: "Port 80", Category: CatOther, URL: server.URL, Source: "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "truenas" {
		t.Fatalf("serviceKey = %q, want %q (redirect to /ui/ should be followed)", svc.ServiceKey, "truenas")
	}
}

func TestFingerprintByAPITrueNAS(t *testing.T) {
	// TrueNAS API returns 401 for unauthenticated requests but the endpoint exists.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>My NAS</title></head></html>`)) // no HTML marker
		case "/api/v2.0/system/version":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Not authenticated"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name: "Port 80", Category: CatOther, URL: server.URL, Source: "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "truenas" {
		t.Fatalf("serviceKey = %q, want %q (API probe should match)", svc.ServiceKey, "truenas")
	}
}

func TestFingerprintByAPISynology(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" || r.URL.Path == "":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>NAS Login</title></head></html>`))
		case strings.HasPrefix(r.URL.Path, "/webapi/query.cgi"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"SYNO.API.Auth":{"maxVersion":7,"minVersion":1,"path":"auth.cgi"}},"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name: "Port 5000", Category: CatOther, URL: server.URL, Source: "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "synology" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "synology")
	}
}

func TestFingerprintByAPIQNAP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>NAS Login</title></head></html>`))
		case "/cgi-bin/authLogin.cgi":
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><QDocRoot><authSid>none</authSid><QNAP_SID/></QDocRoot>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name: "Port 8080", Category: CatOther, URL: server.URL, Source: "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "qnap" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "qnap")
	}
}

func TestFingerprintNoMatchOnGenericService(t *testing.T) {
	// A generic web app should not match any fingerprint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>My App</title></head><body>Welcome</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		Name: "Port 9000", Category: CatOther, URL: server.URL, Source: "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "" {
		t.Fatalf("generic service matched serviceKey = %q, want empty", svc.ServiceKey)
	}
}

func TestFingerprintSkipsAlreadyClassified(t *testing.T) {
	// Services that already have a ServiceKey should not be re-fingerprinted.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>TrueNAS</body></html>`))
	}))
	defer server.Close()

	wsc := &WebServiceCollector{
		client:         server.Client(),
		insecureClient: server.Client(),
	}
	svc := agentmgr.DiscoveredWebService{
		ServiceKey: "myapp",
		Name:       "My App",
		Category:   CatOther,
		URL:        server.URL,
		Source:     "scan",
	}
	wsc.applyFingerprintMetadata(&svc)

	if svc.ServiceKey != "myapp" {
		t.Fatalf("serviceKey changed to %q, should remain %q", svc.ServiceKey, "myapp")
	}
}
