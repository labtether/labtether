package serviceregistry

// knownMonitoringServices owns the monitoring catalog.
func knownMonitoringServices() map[string]KnownService {
	return map[string]KnownService{
		"grafana": {
			Key: "grafana", Name: "Grafana", Category: CatMonitoring, IconKey: "grafana",
			DefaultPort: 3000, HealthPath: "/api/health",
			DockerImages: []string{"grafana/grafana", "grafana/grafana-oss"},
			Description:  "Analytics and interactive visualization platform",
		},
		"prometheus": {
			Key: "prometheus", Name: "Prometheus", Category: CatMonitoring, IconKey: "prometheus",
			DefaultPort: 9090, HealthPath: "/-/healthy",
			DockerImages: []string{"prom/prometheus"},
			Description:  "Systems and service monitoring with time series database",
		},
		"uptime-kuma": {
			Key: "uptime-kuma", Name: "Uptime Kuma", Category: CatMonitoring, IconKey: "uptime-kuma",
			DefaultPort: 3001, HealthPath: "/api/status-page/heartbeat",
			DockerImages: []string{"louislam/uptime-kuma"},
			Description:  "Self-hosted uptime monitoring tool",
		},
		"netdata": {
			Key: "netdata", Name: "Netdata", Category: CatMonitoring, IconKey: "netdata",
			DefaultPort: 19999, HealthPath: "/api/v1/info",
			DockerImages: []string{"netdata/netdata"},
			Description:  "Real-time performance and health monitoring",
		},
		"glances": {
			Key: "glances", Name: "Glances", Category: CatMonitoring, IconKey: "glances",
			DefaultPort: 61208, HealthPath: "/api/3/quicklook",
			DockerImages: []string{"nicolargo/glances"},
			Description:  "Cross-platform system monitoring tool",
		},
		"dozzle": {
			Key: "dozzle", Name: "Dozzle", Category: CatMonitoring, IconKey: "dozzle",
			DefaultPort: 8080, HealthPath: "/healthcheck",
			DockerImages: []string{"amir20/dozzle"},
			Description:  "Real-time Docker log viewer",
		},
		"healthchecks": {
			Key: "healthchecks", Name: "Healthchecks", Category: CatMonitoring, IconKey: "healthchecks",
			DefaultPort: 8000, HealthPath: "/api/v3/checks/",
			DockerImages: []string{"linuxserver/healthchecks"},
			Description:  "Cron job and scheduled task monitoring service",
		},
		"scrutiny": {
			Key: "scrutiny", Name: "Scrutiny", Category: CatMonitoring, IconKey: "scrutiny",
			DefaultPort: 8080, HealthPath: "/api/health",
			DockerImages: []string{"ghcr.io/analogj/scrutiny"},
			Description:  "Hard drive S.M.A.R.T monitoring and web UI",
		},
		"graylog": {
			Key: "graylog", Name: "Graylog", Category: CatMonitoring, IconKey: "graylog",
			DefaultPort: 9000, HealthPath: "/api/system",
			DockerImages: []string{"graylog/graylog"},
			Description:  "Centralized log management and analysis platform",
		},
		"influxdb": {
			Key: "influxdb", Name: "InfluxDB", Category: CatMonitoring, IconKey: "influxdb",
			DefaultPort: 8086, HealthPath: "/health",
			DockerImages: []string{"influxdb"},
			Description:  "High-performance time series database",
		},
		"loki": {
			Key: "loki", Name: "Loki", Category: CatMonitoring, IconKey: "loki",
			DefaultPort: 3100, HealthPath: "/ready",
			DockerImages: []string{"grafana/loki"},
			Description:  "Horizontally scalable log aggregation system",
		},
		"changedetection": {
			Key: "changedetection", Name: "changedetection.io", Category: CatMonitoring, IconKey: "changedetection",
			DefaultPort: 5000, HealthPath: "/",
			DockerImages: []string{"ghcr.io/dgtlmoon/changedetection.io"},
			Description:  "Self-hosted website change detection and monitoring",
		},
		"ntfy": {
			Key: "ntfy", Name: "ntfy", Category: CatMonitoring, IconKey: "ntfy",
			DefaultPort: 80, HealthPath: "/v1/health",
			DockerImages: []string{"binwiederhier/ntfy"},
			Description:  "Self-hosted push notification service",
		},
		"gotify": {
			Key: "gotify", Name: "Gotify", Category: CatMonitoring, IconKey: "gotify",
			DefaultPort: 80, HealthPath: "/health",
			DockerImages: []string{"gotify/server"},
			Description:  "Self-hosted push notification server",
		},
		"beszel": {
			Key: "beszel", Name: "Beszel", Category: CatMonitoring, IconKey: "beszel",
			DefaultPort: 8090, HealthPath: "/",
			DockerImages: []string{"henrygd/beszel"},
			Description:  "Lightweight server monitoring hub with agent support",
		},
	}
}
