package resources

import (
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// Heartbeat metadata the released Go agent sends with the usual tools
// present, from labtether-agent internal/agentplatform/*/provider_metadata.go.
var goAgentMetadata = map[string]map[string]string{
	"linux": {
		"cap_services": "list,action", "service_backend": "systemd",
		"cap_packages": "list,action", "package_backend": "apt",
		"cap_logs": "stored,query,stream", "log_backend": "journalctl",
		"cap_schedules": "list",
		"cap_network":   "list,action", "network_backend": "ifconfig", "network_action_backend": "nmcli",
	},
	"windows": {
		"cap_services": "list,action", "service_backend": "scm",
		"cap_logs": "stored,query,stream", "log_backend": "eventlog",
		"cap_schedules": "list", "cron_backend": "taskscheduler",
		"cap_network": "list,action", "network_backend": "netsh",
		"cap_packages": "list,action", "package_backend": "registry", "package_action_backend": "winget",
	},
	"darwin": {
		"cap_services": "list,action", "service_backend": "launchd",
		"cap_packages": "list,action", "package_backend": "brew",
		"cap_logs": "stored,query,stream", "log_backend": "oslog",
		"cap_schedules": "list",
		"cap_network":   "list,action", "network_backend": "networksetup", "network_action_backend": "networksetup",
	},
}

func agentAsset(platform string, metadata map[string]string) assets.Asset {
	withMetrics := map[string]string{"platform": platform, "cpu_used_percent": "4.00"}
	for key, value := range metadata {
		withMetrics[key] = value
	}
	return assets.Asset{ID: "node-" + platform, Source: "agent", Platform: platform, Metadata: withMetrics}
}

const todaysCommonSet = "cron.list,health.read,logs.query,logs.read,logs.stream,network.action,network.list," +
	"package.action,package.list,process.list,service.action,service.list,telemetry.read,terminal.open"

// Fault: the change alters the set of an agent that does not send the new
// signal, so a Go agent in the field loses (or gains) a page.
func TestAgentWithoutTheSignalKeepsTodaysSet(t *testing.T) {
	want := map[string]string{
		"linux":   todaysCommonSet + ",files.list,users.list",
		"windows": todaysCommonSet,
		"darwin":  todaysCommonSet,
	}
	for platform, metadata := range goAgentMetadata {
		for _, mode := range []string{"", "complete", "true"} {
			withMode := map[string]string{}
			for key, value := range metadata {
				withMode[key] = value
			}
			if mode != "" {
				withMode["agent_capability_mode"] = mode
			}
			got := sortedSet(InferCapabilityIDsFromAssetMetadata(agentAsset(platform, withMode)))
			if got != sortedSet(strings.Split(want[platform], ",")) {
				t.Errorf("%s mode %q: set %s, want %s", platform, mode, got, want[platform])
			}
		}
	}
}

// Fault: an explicit agent that advertises only monitoring still gets the
// terminal, files, users and process entries the Hub adds by default.
func TestExplicitMonitoringAgentGetsOnlyItsAdvertisement(t *testing.T) {
	asset := agentAsset("darwin", map[string]string{
		"agent_capability_mode": "explicit", "agent_edition": "monitoring",
		"cap_disks": "list", "cap_network": "list", "network_backend": "ifconfig", "network_action_backend": "none",
	})
	got := sortedSet(InferCapabilityIDsFromAssetMetadata(asset))
	if want := "health.read,network.list,telemetry.read"; got != want {
		t.Fatalf("set %s, want %s", got, want)
	}
	asset.Platform = "linux"
	asset.Metadata["platform"] = "linux"
	asset.Metadata["cap_processes"] = "list"
	asset.Metadata["agent_capabilities"] = "terminal,files"
	got = sortedSet(InferCapabilityIDsFromAssetMetadata(asset))
	if want := "files.list,health.read,network.list,process.list,telemetry.read,terminal.open"; got != want {
		t.Fatalf("set %s, want %s", got, want)
	}
}

// Fault: the stored set only ever grows, so a device that ran the Go agent
// and now runs an explicit agent keeps terminal.open and files.list.
func TestExplicitAgentReplacesTheStoredSet(t *testing.T) {
	store := persistence.NewMemoryCanonicalModelStore()
	goAsset := agentAsset("linux", goAgentMetadata["linux"])
	PersistCanonicalHeartbeat(store, goAsset, "agent", time.Now().UTC())

	explicit := agentAsset("linux", map[string]string{"agent_capability_mode": "explicit", "cap_network": "list"})
	PersistCanonicalHeartbeat(store, explicit, "agent", time.Now().UTC())
	set, ok, err := store.GetCapabilitySet("resource", explicit.ID)
	if err != nil || !ok {
		t.Fatalf("capability set: ok=%v err=%v", ok, err)
	}
	if got, want := sortedSet(CapabilityIDsFromSet(set)), "health.read,network.list,telemetry.read"; got != want {
		t.Fatalf("stored set %s, want %s", got, want)
	}

	PersistCanonicalHeartbeat(store, goAsset, "agent", time.Now().UTC())
	set, _, _ = store.GetCapabilitySet("resource", goAsset.ID)
	if got, want := sortedSet(CapabilityIDsFromSet(set)), sortedSet(strings.Split(todaysCommonSet+",files.list,users.list", ",")); got != want {
		t.Fatalf("Go agent set after switching back %s, want %s", got, want)
	}
}

func sortedSet(ids []string) string {
	return strings.Join(MergeCapabilityIDs(ids), ",")
}
