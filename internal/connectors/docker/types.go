package docker

import "time"

// EngineInfo describes the Docker daemon on a host.
type EngineInfo struct {
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Hostname   string `json:"hostname,omitempty"`
}

// ContainerState is the hub-side representation of a container's current state.
type ContainerState struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	Created       string            `json:"created,omitempty"`
	Ports         string            `json:"ports,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	StackName     string            `json:"stack_name,omitempty"`
	CPUPercent    *float64          `json:"cpu_percent,omitempty"`
	MemoryPercent *float64          `json:"memory_percent,omitempty"`
	MemoryBytes   *int64            `json:"memory_bytes,omitempty"`
	MemoryLimit   *int64            `json:"memory_limit,omitempty"`
}

// ContainerStats holds per-container resource metrics.
type ContainerStats struct {
	CPUPercent      float64 `json:"cpu_percent"`
	MemoryBytes     int64   `json:"memory_bytes"`
	MemoryLimit     int64   `json:"memory_limit"`
	MemoryPercent   float64 `json:"memory_percent"`
	NetRXBytes      int64   `json:"net_rx_bytes"`
	NetTXBytes      int64   `json:"net_tx_bytes"`
	BlockReadBytes  int64   `json:"block_read_bytes"`
	BlockWriteBytes int64   `json:"block_write_bytes"`
	PIDs            int     `json:"pids"`
}

// ImageState is the hub-side representation of a Docker image.
type ImageState struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
	Size    int64    `json:"size"`
	Created string   `json:"created"`
}

// NetworkState is the hub-side representation of a Docker network.
type NetworkState struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

// VolumeState is the hub-side representation of a Docker volume.
type VolumeState struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

// ComposeStackState is the hub-side representation of a Compose stack.
type ComposeStackState struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	ConfigFile   string   `json:"config_file,omitempty"`
	ContainerIDs []string `json:"container_ids"`
}

// DockerHost aggregates all Docker state for a single agent host.
type DockerHost struct {
	AgentID       string                    `json:"agent_id"`
	Engine        EngineInfo                `json:"engine"`
	Containers    []ContainerState          `json:"containers"`
	Images        []ImageState              `json:"images"`
	Networks      []NetworkState            `json:"networks"`
	Volumes       []VolumeState             `json:"volumes"`
	ComposeStacks []ComposeStackState       `json:"compose_stacks"`
	Stats         map[string]ContainerStats `json:"stats,omitempty"`
	LastSeen      time.Time                 `json:"last_seen"`
}
