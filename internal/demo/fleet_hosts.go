package demo

const (
	epyc    = "AMD EPYC 7302P 16-Core Processor"
	xeon    = "Intel(R) Xeon(R) E-2388G CPU @ 3.20GHz"
	pveKern = "6.8.12-11-pve"
)

var (
	debian12 = OS{ID: "debian", Name: "Debian GNU/Linux", Version: "12 (bookworm)", Pretty: "Debian GNU/Linux 12 (bookworm)", Kernel: "6.1.0-37-amd64"}
	ubuntu24 = OS{ID: "ubuntu", Name: "Ubuntu", Version: "24.04.3 LTS (Noble Numbat)", Pretty: "Ubuntu 24.04.3 LTS", Kernel: "6.8.0-71-generic"}
	proxmox8 = OS{ID: "debian", Name: "Proxmox VE", Version: "8.4", Pretty: "Proxmox VE 8.4 (Debian GNU/Linux 12 (bookworm))", Kernel: pveKern}
)

// fleet is the demo home lab: two Proxmox nodes, storage, network, service
// VMs and three desk machines. Host names and addresses are made up.
var fleet = []Host{
	{
		ID: "pve01", Platform: "linux", OS: proxmox8, IP: "10.0.10.11",
		CPUModel: epyc, Arch: "amd64", Cores: 16, Threads: 32, MemoryGB: 128, DiskGB: 480,
		CPUBase: 34, CPUSwing: 10, MemBase: 71, DiskBase: 52,
		Services: []Service{{Name: "Proxmox VE", URL: "https://10.0.10.11:8006", Port: 8006}},
	},
	{
		ID: "pve02", Platform: "linux", OS: proxmox8, IP: "10.0.10.12",
		CPUModel: xeon, Arch: "amd64", Cores: 8, Threads: 16, MemoryGB: 64, DiskGB: 480,
		CPUBase: 27, CPUSwing: 9, MemBase: 58, DiskBase: 44,
		Services: []Service{{Name: "Proxmox VE", URL: "https://10.0.10.12:8006", Port: 8006}},
	},
	{
		ID: "nas01", Platform: "linux", IP: "10.0.10.20",
		OS:       OS{ID: "debian", Name: "TrueNAS SCALE", Version: "25.04", Pretty: "TrueNAS SCALE 25.04 (Debian GNU/Linux 12 (bookworm))", Kernel: "6.12.15-production+truenas"},
		CPUModel: "12th Gen Intel(R) Core(TM) i5-12400", Arch: "amd64", Cores: 6, Threads: 12, MemoryGB: 64, DiskGB: 120,
		CPUBase: 22, CPUSwing: 9, MemBase: 68, DiskBase: 91,
		Services: []Service{{Name: "TrueNAS", URL: "https://10.0.10.20", Port: 443}},
	},
	{
		ID: "pbs01", Platform: "linux", IP: "10.0.10.21",
		OS:       OS{ID: "debian", Name: "Proxmox Backup Server", Version: "3.4", Pretty: "Proxmox Backup Server 3.4 (Debian GNU/Linux 12 (bookworm))", Kernel: pveKern},
		CPUModel: "12th Gen Intel(R) Core(TM) i3-12100", Arch: "amd64", Cores: 4, Threads: 8, MemoryGB: 16, DiskGB: 4000,
		CPUBase: 9, CPUSwing: 6, MemBase: 42, DiskBase: 66,
		Services: []Service{{Name: "Proxmox Backup Server", URL: "https://10.0.10.21:8007", Port: 8007}},
	},
	{
		ID: "vyos", Platform: "linux", IP: "10.0.10.1",
		OS:       OS{ID: "vyos", Name: "VyOS", Version: "1.4.3", Pretty: "VyOS 1.4.3 (sagitta)", Kernel: "6.6.89-amd64-vyos"},
		CPUModel: "Intel(R) Celeron(R) N5105 @ 2.00GHz", Arch: "amd64", Cores: 4, Threads: 4, MemoryGB: 8, DiskGB: 32,
		CPUBase: 8, CPUSwing: 4, MemBase: 31, DiskBase: 18,
	},
	{
		ID: "pihole", Platform: "linux", IP: "10.0.10.2",
		OS:       OS{ID: "debian", Name: "Debian GNU/Linux", Version: "12 (bookworm)", Pretty: "Debian GNU/Linux 12 (bookworm)", Kernel: "6.12.25+rpt-rpi-v8"},
		CPUModel: "Cortex-A76", Arch: "arm64", Cores: 4, Threads: 4, MemoryGB: 4, DiskGB: 32,
		CPUBase: 6, CPUSwing: 3, MemBase: 28, DiskBase: 15,
		Services: []Service{{Name: "Pi-hole", URL: "http://10.0.10.2/admin", Port: 80}},
	},
	{
		ID: "docker01", Platform: "linux", OS: debian12, IP: "10.0.10.30",
		CPUModel: epyc, Arch: "amd64", Cores: 8, Threads: 8, MemoryGB: 32, DiskGB: 200,
		CPUBase: 22, CPUSwing: 9, MemBase: 62, DiskBase: 48,
		Docker: &DockerEngine{Version: "28.3.3", APIVersion: "1.51", Stacks: docker01Stacks},
	},
	{
		ID: "media01", Platform: "linux", OS: ubuntu24, IP: "10.0.10.31",
		CPUModel: xeon, Arch: "amd64", Cores: 12, Threads: 12, MemoryGB: 32, DiskGB: 500,
		CPUBase: 21, CPUSwing: 14, MemBase: 77, DiskBase: 55,
		Docker: &DockerEngine{Version: "28.3.3", APIVersion: "1.51", Stacks: media01Stacks},
	},
	{
		ID: "monitor01", Platform: "linux", OS: debian12, IP: "10.0.10.32",
		CPUModel: xeon, Arch: "amd64", Cores: 4, Threads: 4, MemoryGB: 16, DiskGB: 200,
		CPUBase: 18, CPUSwing: 5, MemBase: 55, DiskBase: 62,
		Docker: &DockerEngine{Version: "28.1.1", APIVersion: "1.49", Stacks: monitor01Stacks},
	},
	{
		ID: "k3s-cp1", Platform: "linux", OS: ubuntu24, IP: "10.0.10.40",
		CPUModel: epyc, Arch: "amd64", Cores: 4, Threads: 4, MemoryGB: 8, DiskGB: 60,
		CPUBase: 30, CPUSwing: 8, MemBase: 58, DiskBase: 40,
	},
	{
		ID: "k3s-w1", Platform: "linux", OS: ubuntu24, IP: "10.0.10.41",
		CPUModel: xeon, Arch: "amd64", Cores: 8, Threads: 8, MemoryGB: 16, DiskGB: 100,
		CPUBase: 35, CPUSwing: 10, MemBase: 65, DiskBase: 38,
	},
	{
		ID: "homeassistant", Platform: "linux", IP: "10.0.10.50",
		OS:       OS{ID: "haos", Name: "Home Assistant OS", Version: "16.2", Pretty: "Home Assistant OS 16.2", Kernel: "6.12.48-haos"},
		CPUModel: "Intel(R) N100", Arch: "amd64", Cores: 4, Threads: 4, MemoryGB: 8, DiskGB: 64,
		CPUBase: 14, CPUSwing: 6, MemBase: 48, DiskBase: 35,
		Services: []Service{{Name: "Home Assistant", URL: "http://10.0.10.50:8123", Port: 8123}},
	},
	{
		ID: "minecraft", Platform: "linux", OS: debian12, IP: "10.0.10.60",
		CPUModel: epyc, Arch: "amd64", Cores: 4, Threads: 4, MemoryGB: 12, DiskGB: 80,
		CPUBase: 8, CPUSwing: 6, MemBase: 62, DiskBase: 35,
	},
	{
		ID: "ci-runner", Platform: "linux", OS: ubuntu24, IP: "10.0.10.70",
		CPUModel: epyc, Arch: "amd64", Cores: 8, Threads: 8, MemoryGB: 16, DiskGB: 160,
		CPUBase: 15, CPUSwing: 12, MemBase: 40, DiskBase: 52,
	},
	{
		ID: "workstation", Platform: "linux", IP: "10.0.10.71", Offline: true,
		OS:       OS{ID: "fedora", Name: "Fedora Linux", Version: "42 (Workstation Edition)", Pretty: "Fedora Linux 42 (Workstation Edition)", Kernel: "6.16.7-200.fc42.x86_64"},
		CPUModel: "AMD Ryzen 7 7700X 8-Core Processor", Arch: "amd64", Cores: 8, Threads: 16, MemoryGB: 64, DiskGB: 1000,
		CPUBase: 55, CPUSwing: 15, MemBase: 72, DiskBase: 68,
	},
	{
		ID: "htpc", Platform: "windows", IP: "10.0.10.72",
		OS:       OS{ID: "windows", Name: "Windows 11 Pro", Version: "24H2", Pretty: "Windows 11 Pro 24H2", Kernel: "10.0.26100"},
		CPUModel: "12th Gen Intel(R) Core(TM) i3-12100", Arch: "amd64", Cores: 4, Threads: 8, MemoryGB: 16, DiskGB: 512,
		CPUBase: 5, CPUSwing: 4, MemBase: 45, DiskBase: 60,
	},
	{
		ID: "studio", Platform: "darwin", IP: "10.0.10.73",
		OS:       OS{ID: "macos", Name: "macOS", Version: "26.1", Pretty: "macOS 26.1", Kernel: "25.1.0"},
		CPUModel: "Apple M4", Arch: "arm64", Cores: 10, Threads: 10, MemoryGB: 32, DiskGB: 1000,
		CPUBase: 12, CPUSwing: 8, MemBase: 52, DiskBase: 47,
	},
	{
		ID: "offsite-pbs", Platform: "linux", OS: debian12, IP: "10.20.0.10",
		CPUModel: "Intel(R) Celeron(R) J4125 CPU @ 2.00GHz", Arch: "amd64", Cores: 4, Threads: 4, MemoryGB: 8, DiskGB: 8000,
		CPUBase: 5, CPUSwing: 3, MemBase: 30, DiskBase: 42,
		Services: []Service{{Name: "Proxmox Backup Server", URL: "https://10.20.0.10:8007", Port: 8007}},
	},
}

var docker01Stacks = []Stack{
	{Name: "proxy", Containers: []Container{
		{Name: "traefik", Image: "traefik:v3.4", Port: 8080, Internal: 8080, MemoryMB: 96, CPU: 0.6, AgeDays: 41},
		{Name: "authelia", Image: "authelia/authelia:4.39", Port: 9091, Internal: 9091, MemoryMB: 64, CPU: 0.2, AgeDays: 41},
	}},
	{Name: "nextcloud", Containers: []Container{
		{Name: "nextcloud", Image: "nextcloud:31-apache", Port: 8081, Internal: 80, MemoryMB: 612, CPU: 2.4, AgeDays: 19},
		{Name: "nextcloud-db", Image: "postgres:17-alpine", MemoryMB: 288, CPU: 0.9, AgeDays: 19},
		{Name: "nextcloud-redis", Image: "redis:7-alpine", MemoryMB: 22, CPU: 0.2, AgeDays: 19},
	}},
	{Name: "paperless", Containers: []Container{
		{Name: "paperless-web", Image: "ghcr.io/paperless-ngx/paperless-ngx:2.17", Port: 8010, Internal: 8000, MemoryMB: 540, CPU: 1.1, AgeDays: 8},
		{Name: "paperless-worker", Image: "ghcr.io/paperless-ngx/paperless-ngx:2.17", MemoryMB: 0, CPU: 0, AgeDays: 8, Restarting: true},
		{Name: "paperless-broker", Image: "redis:7-alpine", MemoryMB: 18, CPU: 0.1, AgeDays: 8},
	}},
	{Name: "home", Containers: []Container{
		{Name: "vaultwarden", Image: "vaultwarden/server:1.34.1", Port: 8222, Internal: 80, MemoryMB: 48, CPU: 0.1, AgeDays: 63},
		{Name: "uptime-kuma", Image: "louislam/uptime-kuma:1", Port: 3001, Internal: 3001, MemoryMB: 142, CPU: 0.8, AgeDays: 63},
		{Name: "homepage", Image: "ghcr.io/gethomepage/homepage:v1.3.2", Port: 3000, Internal: 3000, MemoryMB: 118, CPU: 0.3, AgeDays: 27},
	}},
	{Name: "gitea", Containers: []Container{
		{Name: "gitea", Image: "gitea/gitea:1.24", Port: 3002, Internal: 3000, MemoryMB: 204, CPU: 0.7, AgeDays: 33},
		{Name: "gitea-db", Image: "postgres:17-alpine", MemoryMB: 96, CPU: 0.3, AgeDays: 33},
	}},
}

var media01Stacks = []Stack{
	{Name: "media", Containers: []Container{
		{Name: "jellyfin", Image: "jellyfin/jellyfin:10.10.7", Port: 8096, Internal: 8096, MemoryMB: 1840, CPU: 9.5, AgeDays: 22},
		{Name: "sonarr", Image: "lscr.io/linuxserver/sonarr:4.0.15", Port: 8989, Internal: 8989, MemoryMB: 312, CPU: 0.8, AgeDays: 22},
		{Name: "radarr", Image: "lscr.io/linuxserver/radarr:5.26.2", Port: 7878, Internal: 7878, MemoryMB: 286, CPU: 0.6, AgeDays: 22},
		{Name: "prowlarr", Image: "lscr.io/linuxserver/prowlarr:1.37.0", Port: 9696, Internal: 9696, MemoryMB: 174, CPU: 0.3, AgeDays: 22},
		{Name: "qbittorrent", Image: "lscr.io/linuxserver/qbittorrent:5.1.2", Port: 8080, Internal: 8080, MemoryMB: 428, CPU: 1.9, AgeDays: 22},
		{Name: "jellyseerr", Image: "fallenbagel/jellyseerr:2.7.2", Port: 5055, Internal: 5055, MemoryMB: 236, CPU: 0.4, AgeDays: 22},
	}},
	{Name: "immich", Containers: []Container{
		{Name: "immich-server", Image: "ghcr.io/immich-app/immich-server:v1.137.3", Port: 2283, Internal: 2283, MemoryMB: 742, CPU: 2.2, AgeDays: 12},
		{Name: "immich-machine-learning", Image: "ghcr.io/immich-app/immich-machine-learning:v1.137.3", MemoryMB: 1210, CPU: 1.4, AgeDays: 12},
		{Name: "immich-db", Image: "ghcr.io/immich-app/postgres:14-vectorchord0.4.3", MemoryMB: 356, CPU: 0.7, AgeDays: 12},
		{Name: "immich-redis", Image: "redis:7-alpine", MemoryMB: 20, CPU: 0.1, AgeDays: 12},
	}},
}

var monitor01Stacks = []Stack{
	{Name: "monitoring", Containers: []Container{
		{Name: "grafana", Image: "grafana/grafana:12.0.2", Port: 3000, Internal: 3000, MemoryMB: 268, CPU: 1.2, AgeDays: 30},
		{Name: "prometheus", Image: "prom/prometheus:v3.4.1", Port: 9090, Internal: 9090, MemoryMB: 1180, CPU: 3.8, AgeDays: 30},
		{Name: "loki", Image: "grafana/loki:3.5.1", Port: 3100, Internal: 3100, MemoryMB: 498, CPU: 1.6, AgeDays: 30},
		{Name: "alertmanager", Image: "prom/alertmanager:v0.28.1", Port: 9093, Internal: 9093, MemoryMB: 42, CPU: 0.1, AgeDays: 30},
	}},
}
