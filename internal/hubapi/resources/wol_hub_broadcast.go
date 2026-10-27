package resources

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/wol"
)

// errHubBroadcastContained means the Hub's own magic packet would stay on its
// container network and never reach the device.
var errHubBroadcastContained = errors.New("hub broadcast cannot leave its container network")

// SendWakeOnLAN sends the Hub's own magic packet when no agent can relay it.
var SendWakeOnLAN = sendHubWakeOnLAN

func sendHubWakeOnLAN(mac net.HardwareAddr, broadcastAddr string) error {
	if !hubBroadcastReachesNetwork() {
		return errHubBroadcastContained
	}
	return wol.Send(mac, broadcastAddr)
}

// containerMarkerFiles exist in every Docker and Podman container.
var containerMarkerFiles = []string{"/.dockerenv", "/run/.containerenv"}

// hubBroadcastReachesNetwork reports whether a broadcast sent by the Hub can
// reach the devices it manages. In a container on a bridge network, as in the
// Compose install and the Home Assistant add-on, it cannot: the packet ends on
// the container network and the Hub would report a wake that never happened.
// LABTETHER_WOL_HUB_BROADCAST=true turns the Hub's broadcast back on for host
// or macvlan networking; false turns it off everywhere.
func hubBroadcastReachesNetwork() bool {
	if enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("LABTETHER_WOL_HUB_BROADCAST"))); err == nil {
		return enabled
	}
	for _, marker := range containerMarkerFiles {
		if _, err := os.Stat(marker); err == nil {
			return false
		}
	}
	return true
}
