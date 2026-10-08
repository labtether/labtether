package serviceregistry

// knownStorageServices owns the storage catalog.
func knownStorageServices() map[string]KnownService {
	return map[string]KnownService{
		"proxmox-backup": {
			Key: "proxmox-backup", Name: "Proxmox Backup Server", Category: CatStorage, IconKey: "proxmox",
			DefaultPort: 8007, HealthPath: "/api2/json/version",
			Description: "Enterprise backup solution for virtual environments",
		},
		"nextcloud": {
			Key: "nextcloud", Name: "Nextcloud", Category: CatStorage, IconKey: "nextcloud",
			DefaultPort: 443, HealthPath: "/status.php",
			DockerImages: []string{"nextcloud", "linuxserver/nextcloud"},
			Description:  "Self-hosted cloud storage and collaboration platform",
		},
		"syncthing": {
			Key: "syncthing", Name: "Syncthing", Category: CatStorage, IconKey: "syncthing",
			DefaultPort: 8384, HealthPath: "/rest/noauth/health",
			DockerImages: []string{"syncthing/syncthing", "linuxserver/syncthing"},
			Description:  "Continuous peer-to-peer file synchronization",
		},
		"filebrowser": {
			Key: "filebrowser", Name: "File Browser", Category: CatStorage, IconKey: "filebrowser",
			DefaultPort: 8080, HealthPath: "/api/health",
			DockerImages: []string{"filebrowser/filebrowser"},
			Description:  "Web-based file manager",
		},
		"minio": {
			Key: "minio", Name: "MinIO", Category: CatStorage, IconKey: "minio",
			DefaultPort: 9000, HealthPath: "/minio/health/live",
			DockerImages: []string{"minio/minio", "quay.io/minio/minio"},
			Description:  "High-performance S3-compatible object storage",
		},
		"seafile": {
			Key: "seafile", Name: "Seafile", Category: CatStorage, IconKey: "seafile",
			DefaultPort: 80, HealthPath: "/api2/ping/",
			DockerImages: []string{"seafileltd/seafile-mc"},
			Description:  "High-performance file sync and share platform",
		},
		"sftpgo": {
			Key: "sftpgo", Name: "SFTPGo", Category: CatStorage, IconKey: "sftpgo",
			DefaultPort: 8080, HealthPath: "/api/v2/healthz",
			DockerImages: []string{"drakkan/sftpgo"},
			Description:  "Full-featured SFTP/FTP/WebDAV server with web UI",
		},
		"duplicati": {
			Key: "duplicati", Name: "Duplicati", Category: CatStorage, IconKey: "duplicati",
			DefaultPort: 8200, HealthPath: "/",
			DockerImages: []string{"linuxserver/duplicati"},
			Description:  "Encrypted cloud backup with scheduling and deduplication",
		},
		"kopia": {
			Key: "kopia", Name: "Kopia", Category: CatStorage, IconKey: "kopia",
			DefaultPort: 51515, HealthPath: "/api/v1/repo/status",
			DockerImages: []string{"kopia/kopia"},
			Description:  "Fast and secure backup tool with deduplication",
		},
		"owncloud": {
			Key: "owncloud", Name: "ownCloud", Category: CatStorage, IconKey: "owncloud",
			DefaultPort: 8080, HealthPath: "/status.php",
			DockerImages: []string{"owncloud/server"},
			Description:  "Self-hosted file hosting and collaboration platform",
		},
		"ocis": {
			Key: "ocis", Name: "ownCloud Infinite Scale", Category: CatStorage, IconKey: "owncloud",
			DefaultPort: 9200, HealthPath: "/",
			DockerImages: []string{"owncloud/ocis"},
			Description:  "Next-gen ownCloud platform with microservice architecture",
		},
		"truenas": {
			Key: "truenas", Name: "TrueNAS", Category: CatStorage, IconKey: "truenas",
			DefaultPort: 80, HealthPath: "/api/v2.0/system/version",
			Description: "Enterprise-grade network-attached storage platform",
		},
		"qnap": {
			Key: "qnap", Name: "QNAP QTS", Category: CatStorage, IconKey: "qnap",
			DefaultPort: 8080, HealthPath: "/",
			Description: "Network-attached storage operating system",
		},
		"synology": {
			Key: "synology", Name: "Synology DSM", Category: CatStorage, IconKey: "synology",
			DefaultPort: 5000, HealthPath: "/",
			Description: "DiskStation Manager network-attached storage",
		},
	}
}
