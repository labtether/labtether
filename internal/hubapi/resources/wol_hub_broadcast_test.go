package resources

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without a relay agent, a Hub in a bridged container must not report a wake
// packet as sent: its broadcast never leaves the container network. With host
// networking the broadcast does reach the LAN and must keep working.
func TestWakeOnLANFromContainerWithoutRelayIsReportedAsFailed(t *testing.T) {
	marker := filepath.Join(t.TempDir(), ".dockerenv")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	originalMarkers, originalInterfaces := containerMarkerFiles, listHubInterfaces
	t.Cleanup(func() { containerMarkerFiles, listHubInterfaces = originalMarkers, originalInterfaces })
	t.Setenv("LABTETHER_WOL_HUB_BROADCAST", "")
	interfaces := []string{"lo", "eth0"}
	listHubInterfaces = func() ([]net.Interface, error) {
		out := make([]net.Interface, 0, len(interfaces))
		for _, name := range interfaces {
			out = append(out, net.Interface{Name: name})
		}
		return out, nil
	}

	containerMarkerFiles = []string{marker}
	if hubBroadcastReachesNetwork() {
		t.Fatal("hub in a bridged container reported that its broadcast reaches the network")
	}
	interfaces = []string{"lo", "enp3s0", "docker0"}
	if !hubBroadcastReachesNetwork() {
		t.Fatal("hub container with host networking reported a contained broadcast")
	}
	interfaces = []string{"lo", "eth0"}
	t.Setenv("LABTETHER_WOL_HUB_BROADCAST", "true")
	if !hubBroadcastReachesNetwork() {
		t.Fatal("explicit macvlan override ignored")
	}
	t.Setenv("LABTETHER_WOL_HUB_BROADCAST", "")
	containerMarkerFiles = []string{filepath.Join(t.TempDir(), "missing")}
	if !hubBroadcastReachesNetwork() {
		t.Fatal("hub outside a container reported a contained broadcast")
	}

	containerMarkerFiles = []string{marker}
	deps, recorder := newWoLHandlerTestDeps()
	seedWoLAsset(t, deps, "sleepy-node", "linux", map[string]string{"mac_address": testWakeMAC})
	rec := invokeWake(deps, "sleepy-node")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "LABTETHER_WOL_HUB_BROADCAST") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	requireWakeOutcome(t, recorder.snapshot(), "failed", "no_relay_available")
}
