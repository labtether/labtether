package main

import (
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
)

func TestNormalizeTrueNASStatus(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     string
	}{
		// ZFS pool states
		{"pool ONLINE", map[string]string{"status": "ONLINE"}, "online"},
		{"pool DEGRADED", map[string]string{"status": "DEGRADED"}, "offline"},
		{"pool FAULTED", map[string]string{"status": "FAULTED"}, "offline"},
		{"pool UNAVAIL", map[string]string{"status": "UNAVAIL"}, "offline"},
		{"pool REMOVED", map[string]string{"status": "REMOVED"}, "offline"},

		// Service/VM states
		{"running", map[string]string{"state": "RUNNING"}, "online"},
		{"stopped", map[string]string{"state": "STOPPED"}, "offline"},
		{"active", map[string]string{"state": "ACTIVE"}, "online"},

		// Healthy flag
		{"healthy", map[string]string{"status": "HEALTHY"}, "online"},

		// Combined status + state
		{"online running", map[string]string{"status": "online", "state": "running"}, "online"},
		{"degraded running", map[string]string{"status": "DEGRADED", "state": "running"}, "offline"},

		// Empty / unknown defaults to online (assets discovered via API are reachable)
		{"empty", map[string]string{}, "online"},
		{"unknown status", map[string]string{"status": "REBUILDING"}, "online"},
		{"nil metadata", nil, "online"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTrueNASStatus(tt.metadata)
			if got != tt.want {
				t.Errorf("normalizeTrueNASStatus(%v) = %q, want %q", tt.metadata, got, tt.want)
			}
		})
	}
}

func TestNormalizePortainerStatus(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     string
	}{
		{"endpoint up", map[string]string{"status": "up"}, "online"},
		{"endpoint down", map[string]string{"status": "down"}, "offline"},
		{"container running", map[string]string{"status": "Up 3 hours", "state": "running"}, "online"},
		{"stack active", map[string]string{"status": "active"}, "online"},
		{"stack inactive", map[string]string{"status": "inactive"}, "offline"},
		{"unknown default", map[string]string{}, "stale"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePortainerStatus(tt.metadata)
			if got != tt.want {
				t.Errorf("normalizePortainerStatus(%v) = %q, want %q", tt.metadata, got, tt.want)
			}
		})
	}
}

func TestNormalizeDockerStatus(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     string
	}{
		{"container running", map[string]string{"status": "Up 2 minutes", "state": "running"}, "online"},
		{"container exited", map[string]string{"status": "Exited (1) 2 hours ago", "state": "exited"}, "offline"},
		{"container paused", map[string]string{"state": "paused"}, "offline"},
		{"container healthy", map[string]string{"status": "Up 2 minutes", "state": "healthy"}, "online"},
		{"empty metadata", map[string]string{}, "online"},
		{"nil metadata", nil, "online"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDockerStatus(tt.metadata)
			if got != tt.want {
				t.Errorf("normalizeDockerStatus(%v) = %q, want %q", tt.metadata, got, tt.want)
			}
		})
	}
}

func TestStableConnectorLogID(t *testing.T) {
	key := "UPID:pve01:0001:0002:abcd:qmstart:101:root@pam:"
	got := stableConnectorLogID("log_proxmox_task", key)
	if got == "" {
		t.Fatalf("expected non-empty stable id")
	}
	if !strings.HasPrefix(got, "log_proxmox_task_") {
		t.Fatalf("expected prefixed id, got %q", got)
	}
	if strings.Contains(got, ":") {
		t.Fatalf("stable id must not contain raw separators, got %q", got)
	}
	if got2 := stableConnectorLogID("log_proxmox_task", key); got2 != got {
		t.Fatalf("stable id must be deterministic: %q != %q", got2, got)
	}
}

func TestProxmoxTaskAssetID(t *testing.T) {
	tests := []struct {
		name     string
		task     proxmox.Task
		fallback string
		want     string
	}{
		{
			name: "qemu target",
			task: proxmox.Task{ID: "qemu/101"},
			want: "proxmox-vm-101",
		},
		{
			name: "lxc target",
			task: proxmox.Task{ID: "lxc/202"},
			want: "proxmox-ct-202",
		},
		{
			name: "node target",
			task: proxmox.Task{ID: "node/PVE-01"},
			want: "proxmox-node-pve-01",
		},
		{
			name:     "fallback node",
			task:     proxmox.Task{Node: "PVE-02"},
			fallback: "cluster-asset",
			want:     "proxmox-node-pve-02",
		},
		{
			name:     "fallback asset",
			task:     proxmox.Task{},
			fallback: "cluster-asset",
			want:     "cluster-asset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proxmoxTaskAssetID(tt.task, tt.fallback)
			if got != tt.want {
				t.Fatalf("proxmoxTaskAssetID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProxmoxTaskLevel(t *testing.T) {
	tests := []struct {
		name string
		task proxmox.Task
		want string
	}{
		{
			name: "error exit status",
			task: proxmox.Task{ExitStatus: "TASK ERROR: timeout"},
			want: "error",
		},
		{
			name: "warning status",
			task: proxmox.Task{Status: "warning"},
			want: "warn",
		},
		{
			name: "ok",
			task: proxmox.Task{ExitStatus: "OK"},
			want: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proxmoxTaskLevel(tt.task)
			if got != tt.want {
				t.Fatalf("proxmoxTaskLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrueNASAlertMessage(t *testing.T) {
	tests := []struct {
		name  string
		alert map[string]any
		want  string
	}{
		{
			name:  "formatted preferred",
			alert: map[string]any{"formatted": "Pool is degraded"},
			want:  "Pool is degraded",
		},
		{
			name:  "fallback text",
			alert: map[string]any{"text": "Disk temperature warning"},
			want:  "Disk temperature warning",
		},
		{
			name:  "fallback klass",
			alert: map[string]any{"klass": "PoolStatus"},
			want:  "truenas alert: PoolStatus",
		},
		{
			name:  "default",
			alert: map[string]any{},
			want:  "truenas alert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trueNASAlertMessage(tt.alert)
			if got != tt.want {
				t.Fatalf("trueNASAlertMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectorAnyTime(t *testing.T) {
	rfc := "2026-02-23T06:29:46Z"
	gotRFC := collectorAnyTime(rfc)
	if gotRFC.IsZero() {
		t.Fatalf("collectorAnyTime must parse RFC3339")
	}
	if gotRFC.UTC().Format(time.RFC3339) != rfc {
		t.Fatalf("collectorAnyTime(rfc) = %s, want %s", gotRFC.UTC().Format(time.RFC3339), rfc)
	}

	gotUnix := collectorAnyTime(float64(1771828186))
	if gotUnix.IsZero() {
		t.Fatalf("collectorAnyTime must parse unix float timestamp")
	}
	if gotUnix.UTC().Unix() != 1771828186 {
		t.Fatalf("collectorAnyTime(unix) = %d, want %d", gotUnix.UTC().Unix(), 1771828186)
	}
}

func TestCollectorEndpointIdentity(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantIP   string
	}{
		{
			name:     "https hostname",
			raw:      "https://omeganas.local:9443",
			wantHost: "omeganas.local",
			wantIP:   "",
		},
		{
			name:     "hostname without scheme",
			raw:      "omeganas.local:9000",
			wantHost: "omeganas.local",
			wantIP:   "",
		},
		{
			name:     "https ip",
			raw:      "https://192.168.1.44:9443",
			wantHost: "192.168.1.44",
			wantIP:   "192.168.1.44",
		},
		{
			name:     "tcp endpoint url",
			raw:      "tcp://10.0.0.25:2375",
			wantHost: "10.0.0.25",
			wantIP:   "10.0.0.25",
		},
		{
			name:     "invalid",
			raw:      "://:",
			wantHost: "",
			wantIP:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, ip := collectorEndpointIdentity(tt.raw)
			if host != tt.wantHost || ip != tt.wantIP {
				t.Fatalf("collectorEndpointIdentity(%q) = (%q, %q), want (%q, %q)", tt.raw, host, ip, tt.wantHost, tt.wantIP)
			}
		})
	}
}

func TestBestRunsOnIdentityTargetPrefersIP(t *testing.T) {
	source := assets.Asset{
		ID:      "portainer-endpoint-1",
		Type:    "container-host",
		Name:    "portainer-omeganas",
		Source:  "portainer",
		GroupID: "home",
		Metadata: map[string]string{
			"endpoint_host": "omeganas.local",
			"endpoint_ip":   "10.0.0.25",
		},
	}
	targetA := assets.Asset{
		ID:      "truenas-host-omeganas",
		Type:    "nas",
		Name:    "OmegaNAS",
		Source:  "truenas",
		GroupID: "home",
		Metadata: map[string]string{
			"collector_endpoint_ip": "10.0.0.25",
		},
	}
	targetB := assets.Asset{
		ID:      "truenas-host-backup",
		Type:    "nas",
		Name:    "BackupNAS",
		Source:  "truenas",
		GroupID: "home",
		Metadata: map[string]string{
			"collector_endpoint_ip": "10.0.0.44",
		},
	}

	identities := map[string]collectorIdentity{
		source.ID:  collectCollectorIdentity(source),
		targetA.ID: collectCollectorIdentity(targetA),
		targetB.ID: collectCollectorIdentity(targetB),
	}

	targetID, reason, ok := bestRunsOnIdentityTarget(source, []assets.Asset{targetA, targetB}, identities)
	if !ok {
		t.Fatalf("expected identity match")
	}
	if targetID != targetA.ID {
		t.Fatalf("bestRunsOnIdentityTarget() target = %q, want %q", targetID, targetA.ID)
	}
	if reason != "ip" {
		t.Fatalf("bestRunsOnIdentityTarget() reason = %q, want ip", reason)
	}
}

func TestBestRunsOnIdentityTargetAmbiguousHostname(t *testing.T) {
	source := assets.Asset{
		ID:      "truenas-host-omeganas",
		Type:    "nas",
		Name:    "OmegaNAS",
		Source:  "truenas",
		GroupID: "home",
	}
	targetA := assets.Asset{
		ID:      "proxmox-vm-100",
		Type:    "vm",
		Name:    "OmegaNAS",
		Source:  "proxmox",
		GroupID: "home",
	}
	targetB := assets.Asset{
		ID:      "proxmox-vm-101",
		Type:    "vm",
		Name:    "omeganas",
		Source:  "proxmox",
		GroupID: "home",
	}

	identities := map[string]collectorIdentity{
		source.ID:  collectCollectorIdentity(source),
		targetA.ID: collectCollectorIdentity(targetA),
		targetB.ID: collectCollectorIdentity(targetB),
	}

	if targetID, reason, ok := bestRunsOnIdentityTarget(source, []assets.Asset{targetA, targetB}, identities); ok {
		t.Fatalf("expected ambiguous hostname match to fail, got target=%q reason=%q", targetID, reason)
	}
}
