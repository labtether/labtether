package serviceregistry

// knownManagementServices owns the management catalog.
func knownManagementServices() map[string]KnownService {
	return map[string]KnownService{
		"portainer": {
			Key: "portainer", Name: "Portainer", Category: CatManagement, IconKey: "portainer",
			DefaultPort: 9443, HealthPath: "/api/status",
			DockerImages: []string{"portainer/portainer-ce", "portainer/portainer-ee"},
			Description:  "Docker and Kubernetes management UI",
		},
		"homer": {
			Key: "homer", Name: "Homer", Category: CatManagement, IconKey: "homer",
			DefaultPort: 8080, HealthPath: "/",
			DockerImages: []string{"b4bz/homer"},
			Description:  "Static application dashboard",
		},
		"homarr": {
			Key: "homarr", Name: "Homarr", Category: CatManagement, IconKey: "homarr",
			DefaultPort: 7575, HealthPath: "/api/health",
			DockerImages: []string{"ghcr.io/ajnart/homarr"},
			Description:  "Customizable homelab dashboard with integrations",
		},
		"homepage": {
			Key: "homepage", Name: "Homepage", Category: CatManagement, IconKey: "homepage",
			DefaultPort: 3000, HealthPath: "/api/health",
			DockerImages: []string{"ghcr.io/gethomepage/homepage"},
			Description:  "Modern application dashboard with service integrations",
		},
		"labtether": {
			Key: "labtether", Name: "LabTether", Category: CatManagement, IconKey: "",
			DefaultPort: 8443, HealthPath: "/healthz",
			DockerImages: []string{"labtether/labtether", "ghcr.io/labtether/labtether"},
			Description:  "LabTether hub and operator console",
		},
		"cockpit": {
			Key: "cockpit", Name: "Cockpit", Category: CatManagement, IconKey: "cockpit",
			DefaultPort: 9090, HealthPath: "/",
			DockerImages: []string{"cockpit/ws"},
			Description:  "Web-based server administration interface",
		},
		"yacht": {
			Key: "yacht", Name: "Yacht", Category: CatManagement, IconKey: "yacht",
			DefaultPort: 8000, HealthPath: "/api/",
			DockerImages: []string{"selfhostedpro/yacht"},
			Description:  "Docker container management web UI",
		},
		"watchtower": {
			Key: "watchtower", Name: "Watchtower", Category: CatManagement, IconKey: "watchtower",
			DefaultPort: 8080, HealthPath: "/v1/update",
			DockerImages: []string{"containrrr/watchtower"},
			Description:  "Automatic Docker container updates",
		},
		"guacamole": {
			Key: "guacamole", Name: "Apache Guacamole", Category: CatManagement, IconKey: "guacamole",
			DefaultPort: 8080, HealthPath: "/",
			DockerImages: []string{"guacamole/guacamole", "jwetzell/guacamole"},
			Description:  "Remote desktop gateway accessible via browser",
		},
		"rustdesk": {
			Key: "rustdesk", Name: "RustDesk", Category: CatManagement, IconKey: "rustdesk",
			DefaultPort: 21118, HealthPath: "/",
			DockerImages: []string{"rustdesk/rustdesk-server"},
			Description:  "Self-hosted remote desktop software",
		},
		"meshcentral": {
			Key: "meshcentral", Name: "MeshCentral", Category: CatManagement, IconKey: "meshcentral",
			DefaultPort: 443, HealthPath: "/",
			DockerImages: []string{"typhonragewind/meshcentral"},
			Description:  "Full-featured remote management solution",
		},
		"dashy": {
			Key: "dashy", Name: "Dashy", Category: CatManagement, IconKey: "dashy",
			DefaultPort: 8080, HealthPath: "/",
			DockerImages: []string{"lissy93/dashy"},
			Description:  "Feature-rich homelab dashboard with status indicators",
		},
		"organizr": {
			Key: "organizr", Name: "Organizr", Category: CatManagement, IconKey: "organizr",
			DefaultPort: 80, HealthPath: "/",
			DockerImages: []string{"organizr/organizr"},
			Description:  "Homelab service organizer and SSO portal",
		},
		"proxmox": {
			Key: "proxmox", Name: "Proxmox VE", Category: CatManagement, IconKey: "proxmox",
			DefaultPort: 8006, HealthPath: "/api2/json/version",
			Description: "Open-source server virtualization platform",
		},
		"heimdall": {
			Key: "heimdall", Name: "Heimdall", Category: CatManagement, IconKey: "heimdall",
			DefaultPort: 80, HealthPath: "/",
			DockerImages: []string{"linuxserver/heimdall"},
			Description:  "Application dashboard for quick access to services",
		},
		"dockge": {
			Key: "dockge", Name: "Dockge", Category: CatManagement, IconKey: "dockge",
			DefaultPort: 5001, HealthPath: "/",
			DockerImages: []string{"louislam/dockge"},
			Description:  "Docker Compose stack manager with a modern UI",
		},
		"semaphore": {
			Key: "semaphore", Name: "Semaphore", Category: CatManagement, IconKey: "semaphore",
			DefaultPort: 3000, HealthPath: "/api/ping",
			DockerImages: []string{"semaphoreui/semaphore"},
			Description:  "Modern web UI for Ansible automation",
		},
		"flame": {
			Key: "flame", Name: "Flame", Category: CatManagement, IconKey: "flame",
			DefaultPort: 5005, HealthPath: "/",
			DockerImages: []string{"pawelmalak/flame"},
			Description:  "Self-hosted startpage and application dashboard",
		},
		"komodo": {
			Key: "komodo", Name: "Komodo", Category: CatManagement, IconKey: "komodo",
			DefaultPort: 9120, HealthPath: "/",
			DockerImages: []string{"ghcr.io/mbecker20/komodo"},
			Description:  "Docker Compose deployment and monitoring dashboard",
		},
	}
}
