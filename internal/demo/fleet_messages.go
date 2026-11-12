package demo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/serviceregistry"
)

const bytesPerGB = 1 << 30

// Messages builds what one host's agent sends in one reporting cycle: a
// heartbeat, which carries the host facts and the current CPU, memory, disk,
// temperature and network readings the hub stores as metric samples, then the
// Docker inventory and stats and the web service report when the host has them.
//
// agentVersion is the agent version the hub ships. Passing the hub's own
// value keeps every demo host current, so the hub never tries to push an
// update to a host that has no connection. Empty leaves the version out.
//
// inventory adds the full Docker inventory. An agent sends it on connect and
// then only now and again; container stats go out every cycle.
func (h Host) Messages(now time.Time, agentVersion string, inventory bool) []agentmgr.Message {
	cpu := h.cpuPercent(now)
	memory := h.memoryPercent()
	disk := h.diskPercent()
	rx := networkBytesPerSec()
	tx := rx * (0.15 + randomFloat64()*0.25)

	messages := []agentmgr.Message{
		message(agentmgr.MsgHeartbeat, h.heartbeat(cpu, memory, disk, rx, tx, agentVersion)),
	}
	if h.Docker != nil {
		if inventory {
			messages = append(messages, message(agentmgr.MsgDockerDiscovery, h.dockerDiscovery(now)))
		}
		messages = append(messages, message(agentmgr.MsgDockerStats, h.dockerStats(now)))
	}
	if services := h.webServices(); len(services) > 0 {
		messages = append(messages, message(agentmgr.MsgWebServiceReport, agentmgr.WebServiceReportData{
			HostAssetID: h.ID,
			Services:    services,
		}))
	}
	return messages
}

func message(kind string, payload any) agentmgr.Message {
	data, err := json.Marshal(payload)
	if err != nil {
		// Every payload is a plain struct of strings and numbers.
		panic(fmt.Sprintf("demo: encode %s: %v", kind, err))
	}
	return agentmgr.Message{Type: kind, Data: data}
}

func (h Host) heartbeat(cpu, memory, disk, rx, tx float64, agentVersion string) agentmgr.HeartbeatData {
	diskTotal := h.DiskGB * bytesPerGB
	metadata := map[string]string{
		"agent":                     "agent",
		"hostname":                  h.ID,
		"platform":                  h.Platform,
		"os":                        h.Platform,
		"os_id":                     h.OS.ID,
		"os_name":                   h.OS.Name,
		"os_version":                h.OS.Version,
		"os_pretty_name":            h.OS.Pretty,
		"kernel_release":            h.OS.Kernel,
		"ip_address":                h.IP,
		"cpu_model":                 h.CPUModel,
		"cpu_architecture":          h.Arch,
		"cpu_cores_physical":        strconv.Itoa(h.Cores),
		"cpu_threads_logical":       strconv.Itoa(h.Threads),
		"memory_total_bytes":        strconv.FormatInt(int64(h.MemoryGB*bytesPerGB), 10),
		"disk_root_total_bytes":     strconv.FormatInt(int64(diskTotal), 10),
		"disk_root_available_bytes": strconv.FormatInt(int64(diskTotal*(100-disk)/100), 10),
		"cpu_used_percent":          decimal(cpu),
		"memory_used_percent":       decimal(memory),
		"disk_used_percent":         decimal(disk),
		"temperature_celsius":       decimal(temperature(cpu)),
		"network_rx_bytes_per_sec":  decimal(rx),
		"network_tx_bytes_per_sec":  decimal(tx),
	}
	if agentVersion != "" {
		metadata["agent_version"] = agentVersion
	}
	data := agentmgr.HeartbeatData{
		AssetID:  h.ID,
		Type:     "host",
		Name:     h.ID,
		Source:   "agent",
		Status:   "online",
		Platform: h.Platform,
		Metadata: metadata,
	}
	if h.Docker != nil {
		data.Connectors = []agentmgr.ConnectorInfo{{Type: "docker", Endpoint: "unix:///var/run/docker.sock", Reachable: true}}
	}
	return data
}

func decimal(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func (h Host) dockerDiscovery(now time.Time) agentmgr.DockerDiscoveryData {
	data := agentmgr.DockerDiscoveryData{
		HostID: h.ID,
		Engine: agentmgr.DockerEngineInfo{Version: h.Docker.Version, APIVersion: h.Docker.APIVersion, OS: "linux", Arch: h.Arch},
	}
	for _, stack := range h.Docker.Stacks {
		dir := "/opt/stacks/" + stack.Name
		network := stack.Name + "_default"
		running := 0
		names := make([]string, 0, len(stack.Containers))
		for _, c := range stack.Containers {
			info := agentmgr.DockerContainerInfo{
				ID:       containerID(h.ID, c.Name),
				Name:     c.Name,
				Image:    c.Image,
				State:    "running",
				Status:   fmt.Sprintf("Up %d days", c.AgeDays),
				Created:  now.Add(-time.Duration(c.AgeDays) * 24 * time.Hour).UTC().Truncate(time.Hour).Format(time.RFC3339),
				Networks: []string{network},
				Labels: map[string]string{
					"com.docker.compose.project":              stack.Name,
					"com.docker.compose.service":              c.Name,
					"com.docker.compose.project.working_dir":  dir,
					"com.docker.compose.project.config_files": dir + "/compose.yaml",
				},
			}
			if c.Restarting {
				info.State = "restarting"
				info.Status = "Restarting (1) 20 seconds ago"
			} else {
				running++
			}
			if c.Port > 0 {
				info.Ports = []agentmgr.DockerPortMapping{{Host: c.Port, Container: c.Internal, Protocol: "tcp"}}
			}
			data.Containers = append(data.Containers, info)
			names = append(names, c.Name)
		}
		data.Networks = append(data.Networks, agentmgr.DockerNetworkInfo{
			ID: containerID(h.ID, "network:"+network), Name: network, Driver: "bridge", Scope: "local",
		})
		data.ComposeStacks = append(data.ComposeStacks, agentmgr.DockerComposeStack{
			Name:        stack.Name,
			Status:      fmt.Sprintf("running(%d)", running),
			ConfigDir:   dir,
			ConfigFile:  dir + "/docker-compose.yml",
			ConfigFiles: []string{dir + "/compose.yaml"},
			Containers:  names,
		})
	}
	return data
}

func (h Host) dockerStats(now time.Time) agentmgr.DockerStatsData {
	data := agentmgr.DockerStatsData{HostID: h.ID}
	limit := int64(h.MemoryGB * bytesPerGB)
	for _, stack := range h.Docker.Stacks {
		for _, c := range stack.Containers {
			if c.Restarting {
				continue
			}
			memoryBytes := int64(c.MemoryMB * (0.96 + randomFloat64()*0.08) * (1 << 20))
			// Byte counters grow with the container's age.
			age := int64(c.AgeDays)*86400 + now.Unix()%86400
			data.Containers = append(data.Containers, agentmgr.DockerContainerStats{
				ID:            containerID(h.ID, c.Name),
				CPUPercent:    clamp(c.CPU*(0.6+randomFloat64()*0.8), 0, 100*float64(h.Threads)),
				MemoryBytes:   memoryBytes,
				MemoryLimit:   limit,
				MemoryPercent: float64(memoryBytes) / float64(limit) * 100,
				NetRXBytes:    age * int64(2000+c.MemoryMB*40),
				NetTXBytes:    age * int64(900+c.MemoryMB*15),
				PIDs:          3 + int(c.MemoryMB/40),
			})
		}
	}
	return data
}

// webServices lists what the agent's web service discovery would report:
// every container with a published port, then the host's own web interfaces.
// A known image is reported under its product name, as the agent does;
// category and icon are left for the hub to fill from the image.
func (h Host) webServices() []agentmgr.DiscoveredWebService {
	var services []agentmgr.DiscoveredWebService
	if h.Docker != nil {
		for _, stack := range h.Docker.Stacks {
			for _, c := range stack.Containers {
				if c.Port == 0 || c.Restarting {
					continue
				}
				id := containerID(h.ID, c.Name)
				services = append(services, agentmgr.DiscoveredWebService{
					ID:          serviceID(h.ID, "docker", fmt.Sprintf("%s:%d", id, c.Port)),
					Name:        serviceName(c),
					URL:         fmt.Sprintf("http://%s:%d", h.IP, c.Port),
					Source:      "docker",
					Status:      "up",
					ResponseMs:  2 + randomIntN(38),
					ContainerID: id,
					HostAssetID: h.ID,
					Metadata: map[string]string{
						"image":       c.Image,
						"public_port": strconv.Itoa(c.Port),
						"stack":       stack.Name,
					},
				})
			}
		}
	}
	for _, svc := range h.Services {
		services = append(services, agentmgr.DiscoveredWebService{
			ID:          serviceID(h.ID, "scan", strconv.Itoa(svc.Port)),
			Name:        svc.Name,
			URL:         svc.URL,
			Source:      "scan",
			Status:      "up",
			ResponseMs:  3 + randomIntN(30),
			HostAssetID: h.ID,
			Metadata:    map[string]string{"public_port": strconv.Itoa(svc.Port)},
		})
	}
	return services
}

// serviceName is the product name of a known image ("Grafana"), or the
// container name when the image is not in the service registry.
func serviceName(c Container) string {
	if known, ok := serviceregistry.LookupByDockerImage(c.Image); ok && known.Name != "" {
		return known.Name
	}
	return c.Name
}
