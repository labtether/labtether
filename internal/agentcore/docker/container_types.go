package docker

import "strings"

// ---------------------------------------------------------------------------
// Docker API response types — shared across docker/, proxy/, and collector code.
// Extracted to root so future subpackages avoid circular imports.
// ---------------------------------------------------------------------------

// dockerContainer is the raw response shape from GET /containers/json.
type DockerContainer struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Created         int64             `json:"Created"`
	Ports           []DockerPort      `json:"Ports"`
	Labels          map[string]string `json:"Labels"`
	Mounts          []DockerMount     `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]interface{} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type DockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type DockerMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

// dockerImage is the raw response shape from GET /images/json.
type DockerImage struct {
	ID       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Size     int64    `json:"Size"`
	Created  int64    `json:"Created"`
}

// dockerNetwork is the raw response shape from GET /networks.
type DockerNetwork struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}

// DockerVolumesResponse is the response shape from GET /volumes.
type DockerVolumesResponse struct {
	Volumes []DockerVolume `json:"Volumes"`
}

type DockerVolume struct {
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Mountpoint string `json:"Mountpoint"`
}

// dockerVersionResponse is the response shape from GET /version.
type DockerVersionResponse struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	Os         string `json:"Os"`
	Arch       string `json:"Arch"`
}

// dockerEvent is the raw response shape from GET /events.
type DockerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	Time int64 `json:"time"`
}

// dockerStatsResponse is the raw response shape from GET /containers/{id}/stats?stream=false.
type DockerStatsResponse struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage int64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage int64 `json:"system_cpu_usage"`
		OnlineCPUs     int   `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage int64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage int64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage int64 `json:"usage"`
		Limit int64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes int64 `json:"rx_bytes"`
		TxBytes int64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value int64  `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
}

// ---------------------------------------------------------------------------
// Docker API request types
// ---------------------------------------------------------------------------

type DockerPortBinding struct {
	HostPort      string
	ContainerPort string
	Protocol      string
}

type DockerContainerCreateRequest struct {
	Name         string
	Image        string
	Command      []string
	Environment  []string
	PortBindings []DockerPortBinding
}

// ---------------------------------------------------------------------------
// Container name helper
// ---------------------------------------------------------------------------

// containerName strips the leading "/" from Docker container names and returns
// the first name in the slice, or an empty string if the slice is empty.
func ContainerName(raw []string) string {
	if len(raw) == 0 {
		return ""
	}
	return strings.TrimPrefix(raw[0], "/")
}
