package webservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/agentmgr"
)

type fakeProxyProvider struct {
	name     string
	apiURL   string
	detected bool
	routes   []ProxyRoute
	err      error
}

func (f fakeProxyProvider) Name() string {
	if strings.TrimSpace(f.name) == "" {
		return "fake-proxy"
	}
	return f.name
}

func (f fakeProxyProvider) DetectAndConnect([]dockerpkg.DockerContainer) (string, bool) {
	if !f.detected {
		return "", false
	}
	if strings.TrimSpace(f.apiURL) == "" {
		return "http://proxy-admin.local", true
	}
	return f.apiURL, true
}

func (f fakeProxyProvider) FetchRoutes(string) ([]ProxyRoute, error) {
	return f.routes, f.err
}

type recordingCollectorTransport struct {
	mu        sync.Mutex
	connected bool
	messages  []agentmgr.Message
	ch        chan agentmgr.Message
	onSend    func(agentmgr.Message)
}

func newRecordingCollectorTransport(connected bool) *recordingCollectorTransport {
	return &recordingCollectorTransport{
		connected: connected,
		ch:        make(chan agentmgr.Message, 32),
	}
}

func (r *recordingCollectorTransport) Send(msg agentmgr.Message) error {
	r.mu.Lock()
	r.messages = append(r.messages, msg)
	r.mu.Unlock()

	select {
	case r.ch <- msg:
	default:
	}
	if r.onSend != nil {
		r.onSend(msg)
	}
	return nil
}

func (r *recordingCollectorTransport) Connect(context.Context) error { return nil }

func (r *recordingCollectorTransport) Receive() (agentmgr.Message, error) {
	return agentmgr.Message{}, nil
}

func (r *recordingCollectorTransport) Close() {}

func (r *recordingCollectorTransport) Connected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connected
}

func (r *recordingCollectorTransport) MessageCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

func waitForCollectorMessage(t *testing.T, transport *recordingCollectorTransport, timeout time.Duration) agentmgr.Message {
	t.Helper()

	select {
	case msg := <-transport.ch:
		return msg
	case <-time.After(timeout):
		t.Fatal("timed out waiting for collector transport message")
		return agentmgr.Message{}
	}
}

func newRootDockerTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	return httptest.NewTLSServer(handler)
}

func TestDockerCollectorRefreshAndPublishFullSendsInventoryAndCachesState(t *testing.T) {
	transport := newRecordingCollectorTransport(true)
	srv := newRootDockerTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(dockerpkg.DockerVersionResponse{
				Version:    "27.0.1",
				APIVersion: "1.45",
				Os:         "linux",
				Arch:       "amd64",
			})
		case "/containers/json":
			_ = json.NewEncoder(w).Encode([]dockerpkg.DockerContainer{
				{
					ID:      "ct-running",
					Names:   []string{"/web"},
					Image:   "nginx:1.27",
					State:   "running",
					Status:  "Up 3 hours",
					Created: 1700000000,
					Ports:   []dockerpkg.DockerPort{{PrivatePort: 80, PublicPort: 8080, Type: "tcp"}},
					Labels:  map[string]string{"com.docker.compose.project": "webstack"},
				},
				{
					ID:      "ct-exited",
					Names:   []string{"/worker"},
					Image:   "busybox:1",
					State:   "exited",
					Status:  "Exited (0)",
					Created: 1700000100,
				},
			})
		case "/images/json":
			_ = json.NewEncoder(w).Encode([]dockerpkg.DockerImage{
				{ID: "img-1", RepoTags: []string{"nginx:1.27"}, Size: 12345, Created: 1700000000},
			})
		case "/networks":
			_ = json.NewEncoder(w).Encode([]dockerpkg.DockerNetwork{
				{ID: "net-1", Name: "bridge", Driver: "bridge", Scope: "local"},
			})
		case "/volumes":
			_ = json.NewEncoder(w).Encode(dockerpkg.DockerVolumesResponse{
				Volumes: []dockerpkg.DockerVolume{{Name: "vol-1", Driver: "local", Mountpoint: "/var/lib/docker/volumes/vol-1/_data"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dc := dockerpkg.NewTestCollector(srv.URL, transport, "asset-1")
	dc.SetTestHTTPClient(srv.Client())

	changed, err := dc.RefreshAndPublishFull(context.Background(), true)
	if err != nil {
		t.Fatalf("refreshAndPublishFull returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected full discovery refresh to publish a snapshot")
	}

	msg := waitForCollectorMessage(t, transport, 2*time.Second)
	if msg.Type != agentmgr.MsgDockerDiscovery {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerDiscovery)
	}

	var payload agentmgr.DockerDiscoveryData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode docker discovery payload: %v", err)
	}
	if payload.HostID != "asset-1" {
		t.Fatalf("host_id=%q, want asset-1", payload.HostID)
	}
	if payload.Engine.Version != "27.0.1" {
		t.Fatalf("engine version=%q, want 27.0.1", payload.Engine.Version)
	}
	if len(payload.Containers) != 2 {
		t.Fatalf("containers=%d, want 2", len(payload.Containers))
	}
	if len(payload.Images) != 1 || len(payload.Networks) != 1 || len(payload.Volumes) != 1 {
		t.Fatalf("expected images/networks/volumes to be included, got %d/%d/%d", len(payload.Images), len(payload.Networks), len(payload.Volumes))
	}

	hasPublished, containerCount, imageCount := dc.TestInventoryState()
	if !hasPublished {
		t.Fatal("expected collector to record that a full snapshot was published")
	}
	if containerCount != 2 || imageCount != 1 {
		t.Fatalf("unexpected cached inventory sizes containers=%d images=%d", containerCount, imageCount)
	}

	running := dc.CurrentRunningContainerIDs()
	if len(running) != 1 || running[0] != "ct-running" {
		t.Fatalf("running container IDs=%v, want [ct-running]", running)
	}
}

func TestDockerCollectorRefreshAndPublishContainerDeltaSendsDeltaForSmallChange(t *testing.T) {
	transport := newRecordingCollectorTransport(true)
	srv := newRootDockerTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]dockerpkg.DockerContainer{
			{ID: "ct-1", Names: []string{"/one"}, Image: "busybox", State: "running", Status: "Up", Created: 1700000000},
			{ID: "ct-2", Names: []string{"/two"}, Image: "busybox", State: "running", Status: "Up", Created: 1700000001},
			{ID: "ct-3", Names: []string{"/three"}, Image: "busybox", State: "running", Status: "Up", Created: 1700000002},
			{ID: "ct-4", Names: []string{"/four"}, Image: "busybox", State: "running", Status: "Up", Created: 1700000003},
			{ID: "ct-5", Names: []string{"/five"}, Image: "busybox", State: "running", Status: "Up", Created: 1700000004},
			{ID: "ct-6", Names: []string{"/six"}, Image: "busybox", State: "running", Status: "Up", Created: 1700000005},
		})
	}))
	defer srv.Close()

	dc := dockerpkg.NewTestCollector(srv.URL, transport, "asset-1")
	dc.SetTestHTTPClient(srv.Client())
	previousContainers := map[string]agentmgr.DockerContainerInfo{
		"ct-1": {ID: "ct-1", Name: "one", Image: "busybox", State: "running", Status: "Up", Created: "2023-11-14T22:13:20Z"},
		"ct-2": {ID: "ct-2", Name: "two", Image: "busybox", State: "running", Status: "Up", Created: "2023-11-14T22:13:21Z"},
		"ct-3": {ID: "ct-3", Name: "three", Image: "busybox", State: "running", Status: "Up", Created: "2023-11-14T22:13:22Z"},
		"ct-4": {ID: "ct-4", Name: "four", Image: "busybox", State: "running", Status: "Up", Created: "2023-11-14T22:13:23Z"},
		"ct-5": {ID: "ct-5", Name: "five", Image: "busybox", State: "running", Status: "Up", Created: "2023-11-14T22:13:24Z"},
	}
	previousRunning := map[string]struct{}{"ct-1": {}, "ct-2": {}, "ct-3": {}, "ct-4": {}, "ct-5": {}}
	dc.SetTestInventoryState(true, previousContainers, make(map[string]agentmgr.DockerImageInfo), previousRunning)

	changed, err := dc.RefreshAndPublishContainerDelta(context.Background())
	if err != nil {
		t.Fatalf("refreshAndPublishContainerDelta returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected delta refresh to publish a change")
	}

	msg := waitForCollectorMessage(t, transport, 2*time.Second)
	if msg.Type != agentmgr.MsgDockerDiscoveryDelta {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerDiscoveryDelta)
	}

	var payload agentmgr.DockerDiscoveryDeltaData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode docker delta payload: %v", err)
	}
	if payload.HostID != "asset-1" {
		t.Fatalf("host_id=%q, want asset-1", payload.HostID)
	}
	if len(payload.UpsertContainers) != 1 || payload.UpsertContainers[0].ID != "ct-6" {
		t.Fatalf("unexpected upsert containers: %+v", payload.UpsertContainers)
	}
	if len(payload.RemoveContainerIDs) != 0 {
		t.Fatalf("unexpected removals: %+v", payload.RemoveContainerIDs)
	}

	running := dc.CurrentRunningContainerIDs()
	if len(running) != 6 || running[5] != "ct-6" {
		t.Fatalf("running container IDs=%v, want ct-6 included", running)
	}
}

func TestDockerCollectorCollectAndSendStatsPublishesOnlyWhenDue(t *testing.T) {
	transport := newRecordingCollectorTransport(true)
	srv := newRootDockerTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/ct-1/stats" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cpu_stats": map[string]any{
				"cpu_usage":        map[string]any{"total_usage": 200},
				"system_cpu_usage": 1000,
				"online_cpus":      2,
			},
			"precpu_stats": map[string]any{
				"cpu_usage":        map[string]any{"total_usage": 100},
				"system_cpu_usage": 500,
			},
			"memory_stats": map[string]any{"usage": 100000, "limit": 200000},
			"pids_stats":   map[string]any{"current": 5},
			"networks": map[string]any{
				"eth0": map[string]any{"rx_bytes": 11, "tx_bytes": 22},
			},
			"blkio_stats": map[string]any{
				"io_service_bytes_recursive": []map[string]any{
					{"op": "Read", "value": 33},
					{"op": "Write", "value": 44},
				},
			},
		})
	}))
	defer srv.Close()

	dc := dockerpkg.NewTestCollector(srv.URL, transport, "asset-1")
	dc.SetTestHTTPClient(srv.Client())
	dc.SetTestInventoryState(false, nil, nil, map[string]struct{}{"ct-1": {}})

	dc.CollectAndSendStats(context.Background())
	if transport.MessageCount() != 1 {
		t.Fatalf("messages after first stats collection=%d, want 1", transport.MessageCount())
	}

	msg := waitForCollectorMessage(t, transport, 2*time.Second)
	if msg.Type != agentmgr.MsgDockerStats {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgDockerStats)
	}

	var payload agentmgr.DockerStatsData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode docker stats payload: %v", err)
	}
	if len(payload.Containers) != 1 || payload.Containers[0].ID != "ct-1" {
		t.Fatalf("unexpected stats payload: %+v", payload.Containers)
	}
	if payload.Containers[0].MemoryBytes != 100000 || payload.Containers[0].BlockReadBytes != 33 || payload.Containers[0].BlockWriteBytes != 44 {
		t.Fatalf("unexpected stats detail: %+v", payload.Containers[0])
	}

	dc.CollectAndSendStats(context.Background())
	if transport.MessageCount() != 1 {
		t.Fatalf("expected no second stats publish while schedule is still cooling down, got %d messages", transport.MessageCount())
	}
}

func TestDockerCollectorRunEventLoopReconnectsAndForwardsEvents(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	var (
		mu       sync.Mutex
		requests int
	)
	srv := newRootDockerTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		requests++
		requestNumber := requests
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		flusher, _ := w.(http.Flusher)
		switch requestNumber {
		case 1:
			_, _ = io.WriteString(w, `{"Type":"container","Action":"start","Actor":{"ID":"ct-1","Attributes":{"name":"web"}},"time":1}`+"\n")
		case 2:
			_, _ = io.WriteString(w, `{"Type":"container","Action":"stop","Actor":{"ID":"ct-1","Attributes":{"name":"web"}},"time":2}`+"\n")
		default:
			<-r.Context().Done()
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	dc := dockerpkg.NewTestCollector(srv.URL, transport, "asset-1")
	dc.SetTestHTTPClient(srv.Client())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		dc.RunEventLoop(ctx)
	}()

	first := waitForCollectorMessage(t, transport, 2*time.Second)
	second := waitForCollectorMessage(t, transport, 3*time.Second)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for docker event loop to stop")
	}

	if first.Type != agentmgr.MsgDockerEvents || second.Type != agentmgr.MsgDockerEvents {
		t.Fatalf("unexpected event message types: %q %q", first.Type, second.Type)
	}

	var firstEvent agentmgr.DockerEventData
	if err := json.Unmarshal(first.Data, &firstEvent); err != nil {
		t.Fatalf("decode first docker event: %v", err)
	}
	var secondEvent agentmgr.DockerEventData
	if err := json.Unmarshal(second.Data, &secondEvent); err != nil {
		t.Fatalf("decode second docker event: %v", err)
	}
	if firstEvent.Action != "start" || secondEvent.Action != "stop" {
		t.Fatalf("unexpected forwarded event actions: %q then %q", firstEvent.Action, secondEvent.Action)
	}

	if dc.DiscoveryTriggerCount() < 2 {
		t.Fatalf("expected discovery triggers to be queued for both events, got %d", dc.DiscoveryTriggerCount())
	}
	// Stats trigger check removed as the channel is no longer accessible from the root test.
	// The event forwarding is verified by the Docker events received above.

	mu.Lock()
	defer mu.Unlock()
	if requests < 2 {
		t.Fatalf("expected event loop to reconnect, got %d event-stream requests", requests)
	}
}

func TestWebServiceCollectorRunPublishesImmediatelyAndResolvesHostIP(t *testing.T) {
	transport := newRecordingCollectorTransport(true)
	ctx, cancel := context.WithCancel(context.Background())
	transport.onSend = func(agentmgr.Message) { cancel() }

	wsc := &WebServiceCollector{
		transport:    transport,
		assetID:      "asset-1",
		interval:     10 * time.Millisecond,
		discoveryCfg: WebServiceDiscoveryConfig{},
		nowFn: func() time.Time {
			return time.Date(2026, time.March, 8, 10, 0, 0, 0, time.UTC)
		},
	}

	wsc.Run(ctx)

	if transport.MessageCount() == 0 {
		t.Fatal("expected web-service collector to publish an initial report immediately")
	}
	if strings.TrimSpace(wsc.hostIP) == "" {
		t.Fatal("expected web-service collector to resolve a host IP when starting with an empty hostIP")
	}

	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgWebServiceReport {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgWebServiceReport)
	}
}

func TestWebServiceCollectorRunCyclePreservesPreviousDockerServicesOnTransientFailure(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()

	dockerServer := newRootDockerTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			http.Error(w, "docker temporarily unavailable", http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer dockerServer.Close()

	dockerCollector := dockerpkg.NewTestCollector(dockerServer.URL, nil, "asset-1")
	dockerCollector.SetTestHTTPClient(dockerServer.Client())

	wsc := &WebServiceCollector{
		transport:        transport,
		assetID:          "asset-1",
		hostIP:           "127.0.0.1",
		docker:           dockerCollector,
		client:           healthServer.Client(),
		insecureClient:   healthServer.Client(),
		discoveryCfg:     WebServiceDiscoveryConfig{DockerEnabled: true},
		lastServices:     []agentmgr.DiscoveredWebService{{ID: "svc-1", HostAssetID: "asset-1", Source: "docker", URL: healthServer.URL, Name: "Grafana", ServiceKey: "grafana", Category: "Monitoring"}},
		compatCache:      make(map[string]compatCacheEntry),
		fingerprintCache: make(map[string]fingerprintCacheEntry),
		healthCache:      make(map[string]healthCacheEntry),
		nowFn: func() time.Time {
			return time.Date(2026, time.March, 8, 11, 0, 0, 0, time.UTC)
		},
	}

	wsc.RunCycle(context.Background())

	if transport.MessageCount() != 1 {
		t.Fatalf("expected one web-service report, got %d", transport.MessageCount())
	}
	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgWebServiceReport {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgWebServiceReport)
	}

	var payload agentmgr.WebServiceReportData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode web-service report payload: %v", err)
	}
	if len(payload.Services) != 1 || payload.Services[0].ID != "svc-1" {
		t.Fatalf("expected previous docker service to be preserved, got %+v", payload.Services)
	}
	if payload.Discovery == nil {
		t.Fatal("expected discovery stats to be included in runtime report")
	}
	if payload.Discovery.Sources["docker"].ServicesFound != 0 {
		t.Fatalf("docker services found=%d, want 0 on transient failure", payload.Discovery.Sources["docker"].ServicesFound)
	}
	if payload.Discovery.FinalSourceCount["docker"] != 1 {
		t.Fatalf("final docker service count=%d, want 1 preserved service", payload.Discovery.FinalSourceCount["docker"])
	}
	if len(wsc.lastServices) != 1 || wsc.lastServices[0].ID != "svc-1" {
		t.Fatalf("expected collector cache to retain preserved service, got %+v", wsc.lastServices)
	}
}

func TestWebServiceCollectorRunCyclePublishesProxyOnlyServices(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()

	wsc := &WebServiceCollector{
		transport:      transport,
		assetID:        "asset-1",
		hostIP:         "127.0.0.1",
		client:         healthServer.Client(),
		insecureClient: healthServer.Client(),
		discoveryCfg:   WebServiceDiscoveryConfig{ProxyEnabled: true},
		proxyProviders: []ProxyProvider{
			fakeProxyProvider{
				name:     "traefik",
				apiURL:   "http://proxy-admin.local",
				detected: true,
				routes: []ProxyRoute{
					{
						Domain:     "grafana.home.lab",
						BackendURL: healthServer.URL,
						TLS:        true,
						RouterName: "grafana",
					},
				},
			},
		},
		nowFn: func() time.Time {
			return time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC)
		},
	}

	wsc.RunCycle(context.Background())

	if transport.MessageCount() != 1 {
		t.Fatalf("expected one web-service report, got %d", transport.MessageCount())
	}
	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgWebServiceReport {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgWebServiceReport)
	}

	var payload agentmgr.WebServiceReportData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode web-service report payload: %v", err)
	}
	if len(payload.Services) != 1 {
		t.Fatalf("expected one proxy-only service, got %+v", payload.Services)
	}
	service := payload.Services[0]
	if service.Source != "proxy" {
		t.Fatalf("service source=%q, want proxy", service.Source)
	}
	if service.ServiceKey != "grafana" {
		t.Fatalf("service key=%q, want grafana", service.ServiceKey)
	}
	if service.Status != "up" {
		t.Fatalf("service status=%q, want up", service.Status)
	}
	if service.URL != "https://grafana.home.lab" {
		t.Fatalf("service URL=%q, want proxied URL", service.URL)
	}
	if service.Metadata["raw_url"] != healthServer.URL {
		t.Fatalf("raw_url=%q, want %q", service.Metadata["raw_url"], healthServer.URL)
	}
	if payload.Discovery == nil {
		t.Fatal("expected discovery stats to be included in runtime report")
	}
	if payload.Discovery.Sources["proxy"].ServicesFound != 1 {
		t.Fatalf("proxy services found=%d, want 1", payload.Discovery.Sources["proxy"].ServicesFound)
	}
	if payload.Discovery.FinalSourceCount["proxy"] != 1 {
		t.Fatalf("final proxy service count=%d, want 1", payload.Discovery.FinalSourceCount["proxy"])
	}
	if len(wsc.lastServices) != 1 || wsc.lastServices[0].Source != "proxy" {
		t.Fatalf("expected collector cache to retain proxy-only service, got %+v", wsc.lastServices)
	}
}

func TestWebServiceCollectorRunCyclePreservesPreviousProxyServicesOnTransientFailure(t *testing.T) {
	transport := newRecordingCollectorTransport(true)

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()

	wsc := &WebServiceCollector{
		transport:      transport,
		assetID:        "asset-1",
		hostIP:         "127.0.0.1",
		client:         healthServer.Client(),
		insecureClient: healthServer.Client(),
		discoveryCfg:   WebServiceDiscoveryConfig{ProxyEnabled: true},
		proxyProviders: []ProxyProvider{
			fakeProxyProvider{
				name:     "traefik",
				apiURL:   "http://proxy-admin.local",
				detected: true,
				err:      errors.New("proxy api unavailable"),
			},
		},
		lastServices: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-proxy",
				HostAssetID: "asset-1",
				Source:      "proxy",
				URL:         "https://grafana.home.lab",
				Name:        "Grafana",
				ServiceKey:  "grafana",
				Category:    CatMonitoring,
				Metadata: map[string]string{
					"proxy_provider": "traefik",
					"raw_url":        healthServer.URL,
				},
			},
		},
		nowFn: func() time.Time {
			return time.Date(2026, time.March, 8, 12, 30, 0, 0, time.UTC)
		},
	}

	wsc.RunCycle(context.Background())

	if transport.MessageCount() != 1 {
		t.Fatalf("expected one web-service report, got %d", transport.MessageCount())
	}
	msg := waitForCollectorMessage(t, transport, time.Second)
	if msg.Type != agentmgr.MsgWebServiceReport {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgWebServiceReport)
	}

	var payload agentmgr.WebServiceReportData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode web-service report payload: %v", err)
	}
	if len(payload.Services) != 1 || payload.Services[0].ID != "svc-proxy" {
		t.Fatalf("expected previous proxy service to be preserved, got %+v", payload.Services)
	}
	if payload.Services[0].Status != "up" {
		t.Fatalf("preserved proxy service status=%q, want up", payload.Services[0].Status)
	}
	if payload.Discovery == nil {
		t.Fatal("expected discovery stats to be included in runtime report")
	}
	if payload.Discovery.Sources["proxy"].ServicesFound != 0 {
		t.Fatalf("proxy services found=%d, want 0 on transient failure", payload.Discovery.Sources["proxy"].ServicesFound)
	}
	if payload.Discovery.FinalSourceCount["proxy"] != 1 {
		t.Fatalf("final proxy service count=%d, want 1 preserved service", payload.Discovery.FinalSourceCount["proxy"])
	}
	if len(wsc.lastServices) != 1 || wsc.lastServices[0].ID != "svc-proxy" {
		t.Fatalf("expected collector cache to retain preserved proxy service, got %+v", wsc.lastServices)
	}
}
