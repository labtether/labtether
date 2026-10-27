package resources

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without a relay agent, a Hub in a container must not report a wake packet
// as sent: its broadcast never leaves the container network.
func TestWakeOnLANFromContainerWithoutRelayIsReportedAsFailed(t *testing.T) {
	marker := filepath.Join(t.TempDir(), ".dockerenv")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	originalMarkers := containerMarkerFiles
	t.Cleanup(func() { containerMarkerFiles = originalMarkers })
	t.Setenv("LABTETHER_WOL_HUB_BROADCAST", "")

	containerMarkerFiles = []string{marker}
	if hubBroadcastReachesNetwork() {
		t.Fatal("hub in a container reported that its broadcast reaches the network")
	}
	t.Setenv("LABTETHER_WOL_HUB_BROADCAST", "true")
	if !hubBroadcastReachesNetwork() {
		t.Fatal("explicit host-networking override ignored")
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
