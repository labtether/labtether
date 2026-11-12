package demo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/webservice"
)

// The demo's Containers and Services pages have rows only if the hub's own
// Docker and web service intake accepts what the fleet sends. A payload the
// intake cannot decode or classify would leave those pages empty.
func TestFleetMessagesFillContainersAndServices(t *testing.T) {
	dockerHosts := docker.NewCoordinator(nil)
	services := webservice.NewCoordinator()
	now := time.Now()

	for _, host := range OnlineHosts() {
		for _, msg := range host.Messages(now, "v2.0.0", true) {
			switch msg.Type {
			case agentmgr.MsgDockerDiscovery:
				dockerHosts.HandleDiscovery(host.ID, msg)
			case agentmgr.MsgDockerStats:
				dockerHosts.HandleStats(host.ID, msg)
			case agentmgr.MsgWebServiceReport:
				services.HandleReport(host.ID, msg)
			}
		}
	}

	host, ok := dockerHosts.GetHost("docker01")
	if !ok {
		t.Fatal("docker01 is not a Docker host after its inventory message")
	}
	if len(host.ComposeStacks) != 5 || len(host.Containers) != 13 {
		t.Fatalf("docker01 has %d stacks and %d containers, want 5 and 13", len(host.ComposeStacks), len(host.Containers))
	}
	restarting := 0
	for _, container := range host.Containers {
		if container.StackName == "" {
			t.Errorf("container %s has no stack", container.Name)
		}
		if container.State == "restarting" {
			restarting++
			continue
		}
		if _, ok := host.Stats[container.ID]; !ok {
			t.Errorf("running container %s has no stats", container.Name)
		}
	}
	if restarting != 1 {
		t.Fatalf("docker01 has %d restarting containers, want 1 (paperless-worker)", restarting)
	}

	urls := map[string]string{}
	for _, svc := range services.ListAll() {
		if svc.Status != "up" {
			t.Errorf("service %s on %s is %q, want up", svc.Name, svc.HostAssetID, svc.Status)
		}
		if other, taken := urls[svc.URL]; taken {
			t.Errorf("services %s and %s share the address %s", other, svc.Name, svc.URL)
		}
		urls[svc.URL] = svc.Name
	}
	// The hub names and classifies a service from its image, as for a real agent.
	var nextcloud *agentmgr.DiscoveredWebService
	for _, svc := range services.ListByHost("docker01") {
		if svc.URL == "http://10.0.10.30:8081" {
			nextcloud = &svc
			break
		}
	}
	if nextcloud == nil || nextcloud.ServiceKey == "" || nextcloud.IconKey == "" {
		t.Fatalf("the Nextcloud container was not recognised as a known service: %+v", nextcloud)
	}
}

// A host with no connection must never look out of date: the hub would try to
// push an agent update to it. The heartbeat therefore carries the version it
// is given, and an offline host sends nothing at all.
func TestFleetHeartbeats(t *testing.T) {
	seen := map[string]bool{}
	for _, host := range OnlineHosts() {
		if host.Offline {
			t.Fatalf("offline host %s is in OnlineHosts", host.ID)
		}
		if seen[host.ID] {
			t.Fatalf("host %s is listed twice", host.ID)
		}
		seen[host.ID] = true

		messages := host.Messages(time.Now(), "v9.9.9", false)
		if messages[0].Type != agentmgr.MsgHeartbeat {
			t.Fatalf("%s: first message is %s, want a heartbeat", host.ID, messages[0].Type)
		}
		var beat agentmgr.HeartbeatData
		if err := json.Unmarshal(messages[0].Data, &beat); err != nil {
			t.Fatalf("%s: heartbeat does not decode: %v", host.ID, err)
		}
		if beat.Type != "host" || beat.Source != "agent" || beat.Name != host.ID {
			t.Errorf("%s: heartbeat is type %q source %q name %q", host.ID, beat.Type, beat.Source, beat.Name)
		}
		if beat.Metadata["agent_version"] != "v9.9.9" {
			t.Errorf("%s: agent_version is %q, want the version passed in", host.ID, beat.Metadata["agent_version"])
		}
		for _, key := range []string{"os_pretty_name", "ip_address", "cpu_model", "cpu_threads_logical", "memory_total_bytes", "disk_root_total_bytes", "cpu_used_percent", "temperature_celsius", "network_tx_bytes_per_sec"} {
			if beat.Metadata[key] == "" {
				t.Errorf("%s: heartbeat has no %s", host.ID, key)
			}
		}
		for _, msg := range messages {
			if msg.Type == agentmgr.MsgDockerDiscovery {
				t.Errorf("%s: sent a Docker inventory in a cycle without one", host.ID)
			}
		}
	}
	if seen["workstation"] {
		t.Fatal("workstation is offline in the demo and must send nothing")
	}
	if len(seen) != len(Hosts())-1 {
		t.Fatalf("%d of %d hosts send messages, want all but workstation", len(seen), len(Hosts()))
	}
}
