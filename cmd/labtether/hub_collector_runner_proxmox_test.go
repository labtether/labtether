package main

import (
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/metricschema"
)

func TestProxmoxResourceHeartbeatIncludesBackupFields(t *testing.T) {
	collectedAt := time.Unix(1_739_955_600, 0).UTC()
	backupAt := collectedAt.Add(-48 * time.Hour)

	resource := proxmox.Resource{
		Type:    "qemu",
		Node:    "pve-01",
		VMID:    101,
		Name:    "web-01",
		Status:  "running",
		CPU:     0.42,
		Mem:     2_000,
		MaxMem:  4_000,
		Disk:    70,
		MaxDisk: 100,
		NetIn:   12345,
		NetOut:  67890,
		Uptime:  3600,
	}

	req, include := proxmoxResourceHeartbeat(resource, "collector-1", map[string]time.Time{
		"101": backupAt,
	}, nil, collectedAt)
	if !include {
		t.Fatalf("expected resource to be included")
	}
	if req.AssetID != "proxmox-vm-101" {
		t.Fatalf("unexpected asset id: %s", req.AssetID)
	}

	// Proxmox /cluster/resources reports netin/netout as cumulative total bytes
	// since boot, not per-second rates. We intentionally omit these to avoid
	// displaying absurdly large values (e.g. 11.6 TB/s) in the UI.
	if req.Metadata[metricschema.HeartbeatKeyNetworkRXBytesPerSec] != "" {
		t.Fatalf("network rx should be omitted (cumulative, not rate): got %q", req.Metadata[metricschema.HeartbeatKeyNetworkRXBytesPerSec])
	}
	if req.Metadata[metricschema.HeartbeatKeyNetworkTXBytesPerSec] != "" {
		t.Fatalf("network tx should be omitted (cumulative, not rate): got %q", req.Metadata[metricschema.HeartbeatKeyNetworkTXBytesPerSec])
	}

	if req.Metadata["last_backup_at"] == "" {
		t.Fatalf("expected last_backup_at")
	}
	if req.Metadata["days_since_backup"] == "" {
		t.Fatalf("expected days_since_backup")
	}
	if req.Metadata["backup_state"] != "" {
		t.Fatalf("did not expect backup_state when backup exists")
	}
}

func TestProxmoxResourceHeartbeatMarksMissingBackup(t *testing.T) {
	resource := proxmox.Resource{
		Type:   "lxc",
		Node:   "pve-01",
		VMID:   202,
		Name:   "ct-202",
		Status: "running",
	}
	req, include := proxmoxResourceHeartbeat(resource, "collector-2", map[string]time.Time{}, nil, time.Now().UTC())
	if !include {
		t.Fatalf("expected resource to be included")
	}
	if req.Metadata["backup_state"] != "missing" {
		t.Fatalf("expected backup_state=missing, got %q", req.Metadata["backup_state"])
	}
}

func TestProxmoxResourceHeartbeatNormalizesNodeAssetID(t *testing.T) {
	resource := proxmox.Resource{
		Type:   "node",
		Node:   "PVE Primary",
		Status: "online",
	}
	req, include := proxmoxResourceHeartbeat(resource, "collector-3", map[string]time.Time{}, nil, time.Now().UTC())
	if !include {
		t.Fatalf("expected node resource to be included")
	}
	if !strings.HasPrefix(req.AssetID, "proxmox-node-") {
		t.Fatalf("unexpected node asset id: %s", req.AssetID)
	}
}

func TestProxmoxGuestIdentityMetadataFromConfigQemu(t *testing.T) {
	metadata := proxmoxGuestIdentityMetadataFromConfig(map[string]any{
		"smbios1":   "uuid=12345678-90ab-cdef-1234-567890abcdef,manufacturer=QEMU",
		"net0":      "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0",
		"net1":      "e1000=11:22:33:44:55:66,bridge=vmbr1",
		"ipconfig0": "ip=192.168.50.10/24,gw=192.168.50.1",
		"ipconfig1": "ip=dhcp",
	})

	if got := metadata["guest_uuid"]; got != "12345678-90ab-cdef-1234-567890abcdef" {
		t.Fatalf("unexpected guest_uuid: %q", got)
	}
	if got := metadata["guest_primary_ip"]; got != "192.168.50.10" {
		t.Fatalf("unexpected guest_primary_ip: %q", got)
	}
	if got := metadata["guest_primary_mac"]; got != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("unexpected guest_primary_mac: %q", got)
	}
	if got := metadata["guest_ips"]; got != "192.168.50.10" {
		t.Fatalf("unexpected guest_ips: %q", got)
	}
	if got := metadata["guest_mac_addresses"]; got != "AA:BB:CC:DD:EE:FF,11:22:33:44:55:66" {
		t.Fatalf("unexpected guest_mac_addresses: %q", got)
	}
}

func TestProxmoxGuestIdentityMetadataFromConfigLXC(t *testing.T) {
	metadata := proxmoxGuestIdentityMetadataFromConfig(map[string]any{
		"hostname": "ct-web",
		"net0":     "name=eth0,bridge=vmbr0,hwaddr=BC:24:11:22:33:44,ip=10.0.1.22/24,gw=10.0.1.1",
	})

	if got := metadata["guest_hostname"]; got != "ct-web" {
		t.Fatalf("unexpected guest_hostname: %q", got)
	}
	if got := metadata["guest_primary_ip"]; got != "10.0.1.22" {
		t.Fatalf("unexpected guest_primary_ip: %q", got)
	}
	if got := metadata["guest_primary_mac"]; got != "BC:24:11:22:33:44" {
		t.Fatalf("unexpected guest_primary_mac: %q", got)
	}
}

func TestProxmoxResourceHeartbeatMergesGuestIdentityMetadata(t *testing.T) {
	resource := proxmox.Resource{
		Type:   "qemu",
		Node:   "pve-01",
		VMID:   404,
		Name:   "app-404",
		Status: "running",
	}

	guestIdentity := map[string]string{
		"guest_uuid":          "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"guest_primary_ip":    "10.5.0.9",
		"guest_primary_mac":   "AA:BB:CC:DD:EE:FF",
		"guest_mac_addresses": "AA:BB:CC:DD:EE:FF",
	}
	req, include := proxmoxResourceHeartbeat(resource, "collector-identity", map[string]time.Time{}, guestIdentity, time.Now().UTC())
	if !include {
		t.Fatalf("expected proxmox VM resource to be included")
	}
	if got := req.Metadata["guest_uuid"]; got != guestIdentity["guest_uuid"] {
		t.Fatalf("expected guest_uuid metadata, got %q", got)
	}
	if got := req.Metadata["guest_primary_ip"]; got != guestIdentity["guest_primary_ip"] {
		t.Fatalf("expected guest_primary_ip metadata, got %q", got)
	}
	if got := req.Metadata["guest_primary_mac"]; got != guestIdentity["guest_primary_mac"] {
		t.Fatalf("expected guest_primary_mac metadata, got %q", got)
	}
}
