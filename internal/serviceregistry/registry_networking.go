package serviceregistry

// knownNetworkingServices owns the networking catalog.
func knownNetworkingServices() map[string]KnownService {
	return map[string]KnownService{
		"traefik": {
			Key: "traefik", Name: "Traefik", Category: CatNetworking, IconKey: "traefik",
			DefaultPort: 80, HealthPath: "/api/overview",
			DockerImages: []string{"traefik"},
			Description:  "Cloud-native reverse proxy and load balancer",
		},
		"nginx-proxy-manager": {
			Key: "nginx-proxy-manager", Name: "Nginx Proxy Manager", Category: CatNetworking, IconKey: "nginx-proxy-manager",
			DefaultPort: 81, HealthPath: "/api/",
			DockerImages: []string{"jc21/nginx-proxy-manager"},
			Description:  "Reverse proxy with a web-based management UI",
		},
		"caddy": {
			Key: "caddy", Name: "Caddy", Category: CatNetworking, IconKey: "caddy",
			DefaultPort: 443, HealthPath: "/",
			DockerImages: []string{"caddy", "lucaslorentz/caddy-docker-proxy"},
			Description:  "Fast, cross-platform HTTP/2 web server with automatic HTTPS",
		},
		"pihole": {
			Key: "pihole", Name: "Pi-hole", Category: CatNetworking, IconKey: "pihole",
			DefaultPort: 53, HealthPath: "/admin/api.php",
			DockerImages: []string{"pihole/pihole"},
			Description:  "Network-wide ad blocking via DNS",
		},
		"adguardhome": {
			Key: "adguardhome", Name: "AdGuard Home", Category: CatNetworking, IconKey: "adguardhome",
			DefaultPort: 3000, HealthPath: "/control/status",
			DockerImages: []string{"adguard/adguardhome"},
			Description:  "Network-wide ad and tracker blocking DNS server",
		},
		"wireguard": {
			Key: "wireguard", Name: "WireGuard", Category: CatNetworking, IconKey: "wireguard",
			DefaultPort: 51820, HealthPath: "",
			DockerImages: []string{"linuxserver/wireguard"},
			Description:  "Fast, modern VPN tunnel",
		},
		"tailscale": {
			Key: "tailscale", Name: "Tailscale", Category: CatNetworking, IconKey: "tailscale",
			DefaultPort: 41641, HealthPath: "",
			DockerImages: []string{"tailscale/tailscale"},
			Description:  "Zero-config mesh VPN based on WireGuard",
		},
		"unifi": {
			Key: "unifi", Name: "UniFi Controller", Category: CatNetworking, IconKey: "unifi",
			DefaultPort: 8443, HealthPath: "/status",
			DockerImages: []string{"linuxserver/unifi-controller", "jacobalberty/unifi"},
			Description:  "UniFi network management controller",
		},
		"speedtest-tracker": {
			Key: "speedtest-tracker", Name: "Speedtest Tracker", Category: CatNetworking, IconKey: "speedtest-tracker",
			DefaultPort: 8080, HealthPath: "/api/healthcheck",
			DockerImages: []string{"linuxserver/speedtest-tracker", "henrywhitaker3/speedtest-tracker"},
			Description:  "Internet speed tracking and monitoring",
		},
		"nginx": {
			Key: "nginx", Name: "Nginx", Category: CatNetworking, IconKey: "nginx",
			DefaultPort: 80, HealthPath: "/",
			DockerImages: []string{"nginx", "linuxserver/nginx"},
			Description:  "High-performance web server and reverse proxy",
		},
		"haproxy": {
			Key: "haproxy", Name: "HAProxy", Category: CatNetworking, IconKey: "haproxy",
			DefaultPort: 8404, HealthPath: "/stats",
			DockerImages: []string{"haproxy"},
			Description:  "Reliable, high-performance TCP/HTTP load balancer",
		},
		"ddns-updater": {
			Key: "ddns-updater", Name: "DDNS Updater", Category: CatNetworking, IconKey: "ddns-updater",
			DefaultPort: 8000, HealthPath: "/",
			DockerImages: []string{"qmcgaw/ddns-updater"},
			Description:  "Dynamic DNS updater supporting many providers",
		},
		"technitium": {
			Key: "technitium", Name: "Technitium DNS", Category: CatNetworking, IconKey: "technitium",
			DefaultPort: 5380, HealthPath: "/",
			DockerImages: []string{"technitium/dns-server"},
			Description:  "Self-hosted authoritative and recursive DNS server",
		},
		"blocky": {
			Key: "blocky", Name: "Blocky", Category: CatNetworking, IconKey: "blocky",
			DefaultPort: 4000, HealthPath: "/api/blocking/status",
			DockerImages: []string{"spx01/blocky"},
			Description:  "Fast and lightweight DNS proxy and ad-blocker",
		},
		"ntopng": {
			Key: "ntopng", Name: "ntopng", Category: CatNetworking, IconKey: "ntopng",
			DefaultPort: 3000, HealthPath: "/",
			DockerImages: []string{"ntop/ntopng"},
			Description:  "High-speed network traffic analysis and monitoring",
		},
		"cloudflare-ddns": {
			Key: "cloudflare-ddns", Name: "Cloudflare DDNS", Category: CatNetworking, IconKey: "cloudflare",
			DefaultPort: 0, HealthPath: "",
			DockerImages: []string{"oznu/cloudflare-ddns"},
			Description:  "Automatic Cloudflare DNS record updater for dynamic IPs",
		},
		"wg-easy": {
			Key: "wg-easy", Name: "WG Easy", Category: CatNetworking, IconKey: "wg-easy",
			DefaultPort: 51821, HealthPath: "/",
			DockerImages: []string{"ghcr.io/wg-easy/wg-easy"},
			Description:  "WireGuard VPN with an easy-to-use web UI",
		},
		"gluetun": {
			Key: "gluetun", Name: "Gluetun", Category: CatNetworking, IconKey: "gluetun",
			DefaultPort: 8000, HealthPath: "/v1/publicip/ip",
			DockerImages: []string{"qmcgaw/gluetun"},
			Description:  "VPN client container supporting many providers",
		},
		"openspeedtest": {
			Key: "openspeedtest", Name: "OpenSpeedTest", Category: CatNetworking, IconKey: "openspeedtest",
			DefaultPort: 3000, HealthPath: "/",
			DockerImages: []string{"openspeedtest/latest"},
			Description:  "Self-hosted network speed test server",
		},
		"pairdrop": {
			Key: "pairdrop", Name: "PairDrop", Category: CatNetworking, IconKey: "pairdrop",
			DefaultPort: 3000, HealthPath: "/",
			DockerImages: []string{"linuxserver/pairdrop", "schlagmichdansen/pairdrop"},
			Description:  "Local file sharing in the browser inspired by AirDrop",
		},
		"pfsense": {
			Key: "pfsense", Name: "pfSense", Category: CatNetworking, IconKey: "pfsense",
			DefaultPort: 443, HealthPath: "/",
			Description: "Open-source firewall and router platform",
		},
		"opnsense": {
			Key: "opnsense", Name: "OPNsense", Category: CatNetworking, IconKey: "opnsense",
			DefaultPort: 443, HealthPath: "/",
			Description: "Open-source firewall and routing platform",
		},
	}
}
