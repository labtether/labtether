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

// hostBridgeInterfaces live only in the host's network namespace. Seeing one
// from inside a container means it runs with host networking, where the Hub's
// broadcast does reach the LAN.
var hostBridgeInterfaces = map[string]struct{}{"docker0": {}, "podman0": {}, "cni-podman0": {}}

var listHubInterfaces = net.Interfaces

// hubBroadcastReachesNetwork reports whether a broadcast sent by the Hub can
// reach the devices it manages. In a container on a bridge network, as in the
// Compose install and the Home Assistant add-on, it cannot: the packet ends on
// the container network and the Hub would report a wake that never happened.
// Host networking is detected. LABTETHER_WOL_HUB_BROADCAST=true turns the
// Hub's broadcast on for macvlan networking, which cannot be detected; false
// turns it off everywhere.
func hubBroadcastReachesNetwork() bool {
	if enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("LABTETHER_WOL_HUB_BROADCAST"))); err == nil {
		return enabled
	}
	inContainer := false
	for _, marker := range containerMarkerFiles {
		if _, err := os.Stat(marker); err == nil {
			inContainer = true
			break
		}
	}
	return !inContainer || sharesHostNetwork()
}

func sharesHostNetwork() bool {
	interfaces, err := listHubInterfaces()
	if err != nil {
		return false
	}
	for _, iface := range interfaces {
		if _, ok := hostBridgeInterfaces[iface.Name]; ok {
			return true
		}
	}
	return false
}
