package webservice

// KnownService defines metadata for a recognized web service.
type KnownService struct {
	Key          string   // unique lowercase identifier (e.g. "plex", "grafana")
	Name         string   // human-readable display name
	Category     string   // one of the Cat* constants
	IconKey      string   // icon identifier (matches Key for dashboard rendering)
	DefaultPort  int      // most common default port
	DockerImages []string // known Docker image names (without tags)
	HealthPath   string   // HTTP health check path (e.g. "/api/health")
	Description  string   // short description of the service
}

// Service category constants.
const (
	CatMedia          = "Media"
	CatDownloads      = "Downloads"
	CatGaming         = "Gaming"
	CatNetworking     = "Networking"
	CatMonitoring     = "Monitoring"
	CatDevelopment    = "Development"
	CatHomeAutomation = "Home Automation"
	CatStorage        = "Storage"
	CatDatabases      = "Databases"
	CatSecurity       = "Security"
	CatProductivity   = "Productivity"
	CatManagement     = "Management"
	CatOther          = "Other"
)

// registry is the static lookup table of known services, keyed by service key.
var registry = map[string]KnownService{
	// ─── Media ───────────────────────────────────────────────────────────
	"plex": {
		Key: "plex", Name: "Plex", Category: CatMedia, IconKey: "plex",
		DefaultPort: 32400, HealthPath: "/web/index.html",
		DockerImages: []string{"linuxserver/plex", "plexinc/pms-docker"},
		Description:  "Media server for streaming movies, TV, and music",
	},
	"jellyfin": {
		Key: "jellyfin", Name: "Jellyfin", Category: CatMedia, IconKey: "jellyfin",
		DefaultPort: 8096, HealthPath: "/health",
		DockerImages: []string{"jellyfin/jellyfin", "linuxserver/jellyfin"},
		Description:  "Free open-source media server",
	},
	"emby": {
		Key: "emby", Name: "Emby", Category: CatMedia, IconKey: "emby",
		DefaultPort: 8096, HealthPath: "/emby/system/info/public",
		DockerImages: []string{"emby/embyserver", "linuxserver/emby"},
		Description:  "Media server with live TV and DVR",
	},
	"sonarr": {
		Key: "sonarr", Name: "Sonarr", Category: CatMedia, IconKey: "sonarr",
		DefaultPort: 8989, HealthPath: "/api/v3/system/status",
		DockerImages: []string{"linuxserver/sonarr", "hotio/sonarr"},
		Description:  "TV series management and automation",
	},
	"radarr": {
		Key: "radarr", Name: "Radarr", Category: CatMedia, IconKey: "radarr",
		DefaultPort: 7878, HealthPath: "/api/v3/system/status",
		DockerImages: []string{"linuxserver/radarr", "hotio/radarr"},
		Description:  "Movie management and automation",
	},
	"lidarr": {
		Key: "lidarr", Name: "Lidarr", Category: CatMedia, IconKey: "lidarr",
		DefaultPort: 8686, HealthPath: "/api/v1/system/status",
		DockerImages: []string{"linuxserver/lidarr", "hotio/lidarr"},
		Description:  "Music collection management and automation",
	},
	"prowlarr": {
		Key: "prowlarr", Name: "Prowlarr", Category: CatMedia, IconKey: "prowlarr",
		DefaultPort: 9696, HealthPath: "/api/v1/health",
		DockerImages: []string{"linuxserver/prowlarr", "hotio/prowlarr"},
		Description:  "Indexer manager for Sonarr, Radarr, and Lidarr",
	},
	"bazarr": {
		Key: "bazarr", Name: "Bazarr", Category: CatMedia, IconKey: "bazarr",
		DefaultPort: 6767, HealthPath: "/api/system/status",
		DockerImages: []string{"linuxserver/bazarr", "hotio/bazarr"},
		Description:  "Subtitle management for Sonarr and Radarr",
	},
	"overseerr": {
		Key: "overseerr", Name: "Overseerr", Category: CatMedia, IconKey: "overseerr",
		DefaultPort: 5055, HealthPath: "/api/v1/status",
		DockerImages: []string{"linuxserver/overseerr", "sctx/overseerr"},
		Description:  "Media request management for Plex",
	},
	"tautulli": {
		Key: "tautulli", Name: "Tautulli", Category: CatMedia, IconKey: "tautulli",
		DefaultPort: 8181, HealthPath: "/api/v2",
		DockerImages: []string{"linuxserver/tautulli", "tautulli/tautulli"},
		Description:  "Plex media server monitoring and analytics",
	},
	"frigate": {
		Key: "frigate", Name: "Frigate", Category: CatMedia, IconKey: "frigate",
		DefaultPort: 5000, HealthPath: "/api/version",
		DockerImages: []string{"ghcr.io/blakeblackshear/frigate"},
		Description:  "NVR with AI object detection for IP cameras",
	},
	"go2rtc": {
		Key: "go2rtc", Name: "go2rtc", Category: CatMedia, IconKey: "go2rtc",
		DefaultPort: 1984, HealthPath: "/api/ws",
		DockerImages: []string{"alexxit/go2rtc"},
		Description:  "Camera streaming server with WebRTC, RTSP, and more",
	},
	"navidrome": {
		Key: "navidrome", Name: "Navidrome", Category: CatMedia, IconKey: "navidrome",
		DefaultPort: 4533, HealthPath: "/api/ping",
		DockerImages: []string{"deluan/navidrome"},
		Description:  "Modern, self-hosted music server compatible with Subsonic",
	},
	"audiobookshelf": {
		Key: "audiobookshelf", Name: "Audiobookshelf", Category: CatMedia, IconKey: "audiobookshelf",
		DefaultPort: 13378, HealthPath: "/healthcheck",
		DockerImages: []string{"ghcr.io/advplyr/audiobookshelf"},
		Description:  "Self-hosted audiobook and podcast server",
	},
	"readarr": {
		Key: "readarr", Name: "Readarr", Category: CatMedia, IconKey: "readarr",
		DefaultPort: 8787, HealthPath: "/api/v1/system/status",
		DockerImages: []string{"linuxserver/readarr", "hotio/readarr"},
		Description:  "Book management and automation",
	},
	"calibre-web": {
		Key: "calibre-web", Name: "Calibre-Web", Category: CatMedia, IconKey: "calibre-web",
		DefaultPort: 8083, HealthPath: "/",
		DockerImages: []string{"linuxserver/calibre-web"},
		Description:  "Web-based eBook management UI for Calibre libraries",
	},
	"kavita": {
		Key: "kavita", Name: "Kavita", Category: CatMedia, IconKey: "kavita",
		DefaultPort: 5000, HealthPath: "/api/health",
		DockerImages: []string{"jvmilazz0/kavita"},
		Description:  "Self-hosted digital library for manga, comics, and books",
	},
	"komga": {
		Key: "komga", Name: "Komga", Category: CatMedia, IconKey: "komga",
		DefaultPort: 25600, HealthPath: "/api/v1/series",
		DockerImages: []string{"gotson/komga"},
		Description:  "Self-hosted comic and manga server",
	},
	"tdarr": {
		Key: "tdarr", Name: "Tdarr", Category: CatMedia, IconKey: "tdarr",
		DefaultPort: 8265, HealthPath: "/api/v2/status",
		DockerImages: []string{"ghcr.io/haveagitgat/tdarr"},
		Description:  "Distributed media transcoding automation",
	},
	"requestrr": {
		Key: "requestrr", Name: "Requestrr", Category: CatMedia, IconKey: "requestrr",
		DefaultPort: 4545, HealthPath: "/",
		DockerImages: []string{"thomst08/requestrr"},
		Description:  "Chatbot for media requests via Discord and other platforms",
	},
	"flaresolverr": {
		Key: "flaresolverr", Name: "FlareSolverr", Category: CatMedia, IconKey: "flaresolverr",
		DefaultPort: 8191, HealthPath: "/health",
		DockerImages: []string{"ghcr.io/flaresolverr/flaresolverr"},
		Description:  "Proxy server to bypass Cloudflare protection",
	},
	"stash": {
		Key: "stash", Name: "Stash", Category: CatMedia, IconKey: "stash",
		DefaultPort: 9999, HealthPath: "/",
		DockerImages: []string{"stashapp/stash"},
		Description:  "Self-hosted media organizer and player",
	},
	"photoprism": {
		Key: "photoprism", Name: "PhotoPrism", Category: CatMedia, IconKey: "photoprism",
		DefaultPort: 2342, HealthPath: "/api/v1/status",
		DockerImages: []string{"photoprism/photoprism"},
		Description:  "AI-powered photo management and sharing platform",
	},
	"airsonic": {
		Key: "airsonic", Name: "Airsonic-Advanced", Category: CatMedia, IconKey: "airsonic",
		DefaultPort: 4040, HealthPath: "/rest/ping",
		DockerImages: []string{"linuxserver/airsonic-advanced"},
		Description:  "Enhanced music streaming server (Airsonic-Advanced fork)",
	},
	"jellyseerr": {
		Key: "jellyseerr", Name: "Jellyseerr", Category: CatMedia, IconKey: "jellyseerr",
		DefaultPort: 5055, HealthPath: "/api/v1/status",
		DockerImages: []string{"fallenbagel/jellyseerr"},
		Description:  "Media request management for Jellyfin",
	},
	"whisparr": {
		Key: "whisparr", Name: "Whisparr", Category: CatMedia, IconKey: "whisparr",
		DefaultPort: 6969, HealthPath: "/api/v3/system/status",
		DockerImages: []string{"hotio/whisparr"},
		Description:  "Adult content management and automation",
	},
	"mylar3": {
		Key: "mylar3", Name: "Mylar3", Category: CatMedia, IconKey: "mylar3",
		DefaultPort: 8090, HealthPath: "/",
		DockerImages: []string{"linuxserver/mylar3"},
		Description:  "Automated comic book downloader",
	},
	"unmanic": {
		Key: "unmanic", Name: "Unmanic", Category: CatMedia, IconKey: "unmanic",
		DefaultPort: 8888, HealthPath: "/unmanic/api/v2/version",
		DockerImages: []string{"josh5/unmanic"},
		Description:  "Media transcoding automation pipeline",
	},
	"metube": {
		Key: "metube", Name: "MeTube", Category: CatMedia, IconKey: "metube",
		DefaultPort: 8081, HealthPath: "/",
		DockerImages: []string{"ghcr.io/alexta69/metube", "alexta69/metube"},
		Description:  "YouTube video downloader with web UI",
	},
	"streammaster": {
		Key: "streammaster", Name: "StreamMaster", Category: CatMedia, IconKey: "streammaster",
		DefaultPort: 7095, HealthPath: "/",
		DockerImages: []string{"senexcrenshaw/streammaster"},
		Description:  "IPTV proxy and stream management",
	},
	"romm": {
		Key: "romm", Name: "RomM", Category: CatGaming, IconKey: "romm",
		DefaultPort: 8080, HealthPath: "/api/heartbeat",
		DockerImages: []string{"zurdi15/romm", "rommapp/romm"},
		Description:  "ROM manager and game library organizer",
	},
	"sabnzbd": {
		Key: "sabnzbd", Name: "SABnzbd", Category: CatDownloads, IconKey: "sabnzbd",
		DefaultPort: 8080, HealthPath: "/api",
		DockerImages: []string{"linuxserver/sabnzbd"},
		Description:  "Usenet binary newsreader",
	},
	"transmission": {
		Key: "transmission", Name: "Transmission", Category: CatDownloads, IconKey: "transmission",
		DefaultPort: 9091, HealthPath: "/transmission/web/",
		DockerImages: []string{"linuxserver/transmission", "haugene/transmission-openvpn"},
		Description:  "Lightweight BitTorrent client",
	},
	"qbittorrent": {
		Key: "qbittorrent", Name: "qBittorrent", Category: CatDownloads, IconKey: "qbittorrent",
		DefaultPort: 8080, HealthPath: "/api/v2/app/version",
		DockerImages: []string{"linuxserver/qbittorrent", "hotio/qbittorrent"},
		Description:  "Feature-rich BitTorrent client",
	},

	// ─── Downloads ──────────────────────────────────────────────────────

	"deluge": {
		Key: "deluge", Name: "Deluge", Category: CatDownloads, IconKey: "deluge",
		DefaultPort: 8112, HealthPath: "/json",
		DockerImages: []string{"linuxserver/deluge", "binhex/arch-delugevpn"},
		Description:  "Lightweight BitTorrent client with plugin support",
	},
	"nzbget": {
		Key: "nzbget", Name: "NZBGet", Category: CatDownloads, IconKey: "nzbget",
		DefaultPort: 6789, HealthPath: "/",
		DockerImages: []string{"linuxserver/nzbget"},
		Description:  "Efficient Usenet downloader",
	},
	"jdownloader": {
		Key: "jdownloader", Name: "JDownloader", Category: CatDownloads, IconKey: "jdownloader",
		DefaultPort: 5800, HealthPath: "/",
		DockerImages: []string{"jlesage/jdownloader-2"},
		Description:  "Free download management tool",
	},
	"aria2": {
		Key: "aria2", Name: "Aria2", Category: CatDownloads, IconKey: "aria2",
		DefaultPort: 6800, HealthPath: "/jsonrpc",
		DockerImages: []string{"p3terx/aria2-pro", "hurlenko/aria2-ariang"},
		Description:  "Lightweight multi-protocol download utility",
	},
	"pyload": {
		Key: "pyload", Name: "pyLoad", Category: CatDownloads, IconKey: "pyload",
		DefaultPort: 8000, HealthPath: "/api/login",
		DockerImages: []string{"linuxserver/pyload-ng"},
		Description:  "Free and open-source download manager",
	},
	"flood": {
		Key: "flood", Name: "Flood", Category: CatDownloads, IconKey: "flood",
		DefaultPort: 3000, HealthPath: "/api/auth/verify",
		DockerImages: []string{"jesec/flood"},
		Description:  "Modern web UI for rTorrent, qBittorrent, and Transmission",
	},
	"nzbhydra": {
		Key: "nzbhydra", Name: "NZBHydra 2", Category: CatDownloads, IconKey: "nzbhydra2",
		DefaultPort: 5076, HealthPath: "/",
		DockerImages: []string{"linuxserver/nzbhydra2"},
		Description:  "Meta search for Usenet indexers",
	},
	"rtorrent": {
		Key: "rtorrent", Name: "rTorrent/ruTorrent", Category: CatDownloads, IconKey: "rutorrent",
		DefaultPort: 8080, HealthPath: "/",
		DockerImages: []string{"crazymax/rtorrent-rutorrent", "linuxserver/rutorrent"},
		Description:  "BitTorrent client with ruTorrent web UI",
	},

	// ─── Gaming ─────────────────────────────────────────────────────────

	"crafty": {
		Key: "crafty", Name: "Crafty Controller", Category: CatGaming, IconKey: "crafty-controller",
		DefaultPort: 8443, HealthPath: "/",
		DockerImages: []string{"registry.gitlab.com/crafty-controller/crafty-4"},
		Description:  "Minecraft server management panel",
	},
	"pterodactyl": {
		Key: "pterodactyl", Name: "Pterodactyl", Category: CatGaming, IconKey: "pterodactyl",
		DefaultPort: 80, HealthPath: "/",
		DockerImages: []string{"ghcr.io/pterodactyl/panel"},
		Description:  "Game server management panel",
	},
	"pelican": {
		Key: "pelican", Name: "Pelican Panel", Category: CatGaming, IconKey: "pelican-panel",
		DefaultPort: 80, HealthPath: "/",
		DockerImages: []string{"ghcr.io/pelican-dev/panel"},
		Description:  "Game server management panel (Pterodactyl fork)",
	},
	"pufferpanel": {
		Key: "pufferpanel", Name: "PufferPanel", Category: CatGaming, IconKey: "pufferpanel",
		DefaultPort: 8080, HealthPath: "/api/health",
		DockerImages: []string{"pufferpanel/pufferpanel"},
		Description:  "Open-source game server management",
	},
	"amp": {
		Key: "amp", Name: "AMP", Category: CatGaming, IconKey: "amp",
		DefaultPort: 8080, HealthPath: "/",
		DockerImages: []string{"cubecoders/amp"},
		Description:  "Application Management Panel for game servers",
	},

	// ─── Networking ──────────────────────────────────────────────────────
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

	// ─── Monitoring ──────────────────────────────────────────────────────
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

	// ─── Management ──────────────────────────────────────────────────────
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
	"proxmox-backup": {
		Key: "proxmox-backup", Name: "Proxmox Backup Server", Category: CatStorage, IconKey: "proxmox",
		DefaultPort: 8007, HealthPath: "/api2/json/version",
		Description: "Enterprise backup solution for virtual environments",
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

	// ─── Home Automation ─────────────────────────────────────────────────
	"homeassistant": {
		Key: "homeassistant", Name: "Home Assistant", Category: CatHomeAutomation, IconKey: "homeassistant",
		DefaultPort: 8123, HealthPath: "/api/",
		DockerImages: []string{"ghcr.io/home-assistant/home-assistant", "homeassistant/home-assistant"},
		Description:  "Open-source home automation platform",
	},
	"nodered": {
		Key: "nodered", Name: "Node-RED", Category: CatHomeAutomation, IconKey: "nodered",
		DefaultPort: 1880, HealthPath: "/",
		DockerImages: []string{"nodered/node-red"},
		Description:  "Low-code programming for event-driven automation",
	},
	"mqtt": {
		Key: "mqtt", Name: "Mosquitto MQTT", Category: CatHomeAutomation, IconKey: "mqtt",
		DefaultPort: 1883, HealthPath: "",
		DockerImages: []string{"eclipse-mosquitto"},
		Description:  "Lightweight MQTT message broker",
	},
	"zigbee2mqtt": {
		Key: "zigbee2mqtt", Name: "Zigbee2MQTT", Category: CatHomeAutomation, IconKey: "zigbee2mqtt",
		DefaultPort: 8080, HealthPath: "/api/health",
		DockerImages: []string{"koenkk/zigbee2mqtt"},
		Description:  "Zigbee to MQTT bridge",
	},
	"esphome": {
		Key: "esphome", Name: "ESPHome", Category: CatHomeAutomation, IconKey: "esphome",
		DefaultPort: 6052, HealthPath: "/",
		DockerImages: []string{"ghcr.io/esphome/esphome", "esphome/esphome"},
		Description:  "ESP8266/ESP32 device configuration and management",
	},
	"scrypted": {
		Key: "scrypted", Name: "Scrypted", Category: CatHomeAutomation, IconKey: "scrypted",
		DefaultPort: 10443, HealthPath: "/",
		DockerImages: []string{"koush/scrypted"},
		Description:  "Home video integration and automation platform",
	},
	"deconz": {
		Key: "deconz", Name: "deCONZ", Category: CatHomeAutomation, IconKey: "deconz",
		DefaultPort: 80, HealthPath: "/api/config",
		DockerImages: []string{"deconzcommunity/deconz"},
		Description:  "Zigbee gateway and network manager via ConBee/RaspBee",
	},
	"zwavejs": {
		Key: "zwavejs", Name: "Z-Wave JS UI", Category: CatHomeAutomation, IconKey: "zwave-js-ui",
		DefaultPort: 8091, HealthPath: "/health",
		DockerImages: []string{"zwavejs/zwave-js-ui"},
		Description:  "Z-Wave network management and MQTT bridge",
	},
	"wyze-bridge": {
		Key: "wyze-bridge", Name: "Wyze Bridge", Category: CatHomeAutomation, IconKey: "wyze",
		DefaultPort: 5000, HealthPath: "/",
		DockerImages: []string{"mrlt8/wyze-bridge"},
		Description:  "RTSP/WebRTC bridge for Wyze camera streams",
	},
	"double-take": {
		Key: "double-take", Name: "Double Take", Category: CatHomeAutomation, IconKey: "double-take",
		DefaultPort: 3000, HealthPath: "/api/config",
		DockerImages: []string{"jakowenko/double-take"},
		Description:  "Unified facial recognition for home automation",
	},

	// ─── Storage ─────────────────────────────────────────────────────────
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

	// ─── Security ────────────────────────────────────────────────────────
	"vaultwarden": {
		Key: "vaultwarden", Name: "Vaultwarden", Category: CatSecurity, IconKey: "vaultwarden",
		DefaultPort: 8080, HealthPath: "/alive",
		DockerImages: []string{"vaultwarden/server"},
		Description:  "Lightweight Bitwarden-compatible password manager server",
	},
	"authelia": {
		Key: "authelia", Name: "Authelia", Category: CatSecurity, IconKey: "authelia",
		DefaultPort: 9091, HealthPath: "/api/health",
		DockerImages: []string{"authelia/authelia"},
		Description:  "Authentication and authorization server with SSO and 2FA",
	},
	"crowdsec": {
		Key: "crowdsec", Name: "CrowdSec", Category: CatSecurity, IconKey: "crowdsec",
		DefaultPort: 8080, HealthPath: "/v1/decisions",
		DockerImages: []string{"crowdsecurity/crowdsec"},
		Description:  "Collaborative intrusion prevention system",
	},
	"authentik": {
		Key: "authentik", Name: "Authentik", Category: CatSecurity, IconKey: "authentik",
		DefaultPort: 9000, HealthPath: "/api/v3/root/config/",
		DockerImages: []string{"ghcr.io/goauthentik/server"},
		Description:  "Flexible and versatile identity provider",
	},
	"keycloak": {
		Key: "keycloak", Name: "Keycloak", Category: CatSecurity, IconKey: "keycloak",
		DefaultPort: 8080, HealthPath: "/health",
		DockerImages: []string{"quay.io/keycloak/keycloak"},
		Description:  "Enterprise-grade identity and access management",
	},
	"oauth2-proxy": {
		Key: "oauth2-proxy", Name: "OAuth2 Proxy", Category: CatSecurity, IconKey: "oauth2-proxy",
		DefaultPort: 4180, HealthPath: "/ping",
		DockerImages: []string{"quay.io/oauth2-proxy/oauth2-proxy"},
		Description:  "Reverse proxy providing OAuth2/OIDC authentication",
	},
	"lldap": {
		Key: "lldap", Name: "LLDAP", Category: CatSecurity, IconKey: "lldap",
		DefaultPort: 17170, HealthPath: "/",
		DockerImages: []string{"lldap/lldap"},
		Description:  "Lightweight LDAP server for user management",
	},
	"zitadel": {
		Key: "zitadel", Name: "ZITADEL", Category: CatSecurity, IconKey: "zitadel",
		DefaultPort: 8080, HealthPath: "/debug/healthz",
		DockerImages: []string{"ghcr.io/zitadel/zitadel"},
		Description:  "Cloud-native identity and access management platform",
	},

	// ─── Databases ──────────────────────────────────────────────────────
	"pgadmin": {
		Key: "pgadmin", Name: "pgAdmin", Category: CatDatabases, IconKey: "pgadmin",
		DefaultPort: 80, HealthPath: "/misc/ping",
		DockerImages: []string{"dpage/pgadmin4"},
		Description:  "Feature-rich PostgreSQL administration and management tool",
	},
	"adminer": {
		Key: "adminer", Name: "Adminer", Category: CatDatabases, IconKey: "adminer",
		DefaultPort: 8080, HealthPath: "/",
		DockerImages: []string{"adminer"},
		Description:  "Lightweight database management tool for multiple DB engines",
	},
	"phpmyadmin": {
		Key: "phpmyadmin", Name: "phpMyAdmin", Category: CatDatabases, IconKey: "phpmyadmin",
		DefaultPort: 80, HealthPath: "/",
		DockerImages: []string{"phpmyadmin"},
		Description:  "Web-based MySQL and MariaDB administration tool",
	},
	"redis-commander": {
		Key: "redis-commander", Name: "Redis Commander", Category: CatDatabases, IconKey: "redis",
		DefaultPort: 8081, HealthPath: "/",
		DockerImages: []string{"rediscommander/redis-commander"},
		Description:  "Web-based Redis database management UI",
	},
	"mongo-express": {
		Key: "mongo-express", Name: "Mongo Express", Category: CatDatabases, IconKey: "mongodb",
		DefaultPort: 8081, HealthPath: "/",
		DockerImages: []string{"mongo-express"},
		Description:  "Web-based MongoDB administration interface",
	},
	"dbgate": {
		Key: "dbgate", Name: "DbGate", Category: CatDatabases, IconKey: "dbgate",
		DefaultPort: 3000, HealthPath: "/",
		DockerImages: []string{"dbgate/dbgate"},
		Description:  "Cross-platform database manager supporting SQL and NoSQL engines",
	},
	"cloudbeaver": {
		Key: "cloudbeaver", Name: "CloudBeaver", Category: CatDatabases, IconKey: "cloudbeaver",
		DefaultPort: 8978, HealthPath: "/status",
		DockerImages: []string{"dbeaver/cloudbeaver"},
		Description:  "Web-based database management UI powered by DBeaver",
	},

	// ─── Development ─────────────────────────────────────────────────────
	"gitea": {
		Key: "gitea", Name: "Gitea", Category: CatDevelopment, IconKey: "gitea",
		DefaultPort: 3000, HealthPath: "/api/v1/version",
		DockerImages: []string{"gitea/gitea"},
		Description:  "Lightweight self-hosted Git service",
	},
	"forgejo": {
		Key: "forgejo", Name: "Forgejo", Category: CatDevelopment, IconKey: "forgejo",
		DefaultPort: 3000, HealthPath: "/api/v1/version",
		DockerImages: []string{"codeberg.org/forgejo/forgejo"},
		Description:  "Community-driven Git forge (Gitea fork)",
	},
	"gitlab": {
		Key: "gitlab", Name: "GitLab", Category: CatDevelopment, IconKey: "gitlab",
		DefaultPort: 80, HealthPath: "/-/health",
		DockerImages: []string{"gitlab/gitlab-ce", "gitlab/gitlab-ee"},
		Description:  "Complete DevOps platform with Git repository management",
	},
	"jenkins": {
		Key: "jenkins", Name: "Jenkins", Category: CatDevelopment, IconKey: "jenkins",
		DefaultPort: 8080, HealthPath: "/login",
		DockerImages: []string{"jenkins/jenkins"},
		Description:  "Open-source automation server for CI/CD",
	},
	"drone": {
		Key: "drone", Name: "Drone", Category: CatDevelopment, IconKey: "drone",
		DefaultPort: 80, HealthPath: "/healthz",
		DockerImages: []string{"drone/drone"},
		Description:  "Container-native continuous delivery platform",
	},
	"codeserver": {
		Key: "codeserver", Name: "code-server", Category: CatDevelopment, IconKey: "codeserver",
		DefaultPort: 8443, HealthPath: "/healthz",
		DockerImages: []string{"linuxserver/code-server", "codercom/code-server"},
		Description:  "VS Code running in the browser",
	},
	"n8n": {
		Key: "n8n", Name: "n8n", Category: CatDevelopment, IconKey: "n8n",
		DefaultPort: 5678, HealthPath: "/healthz",
		DockerImages: []string{"n8nio/n8n"},
		Description:  "Workflow automation tool with a visual editor",
	},
	"woodpecker": {
		Key: "woodpecker", Name: "Woodpecker CI", Category: CatDevelopment, IconKey: "woodpecker-ci",
		DefaultPort: 8000, HealthPath: "/healthz",
		DockerImages: []string{"woodpeckerci/woodpecker-server"},
		Description:  "Simple, lightweight CI/CD pipeline runner",
	},
	"registry": {
		Key: "registry", Name: "Docker Registry", Category: CatDevelopment, IconKey: "docker",
		DefaultPort: 5000, HealthPath: "/v2/",
		DockerImages: []string{"registry"},
		Description:  "Self-hosted Docker image registry",
	},
	"sonarqube": {
		Key: "sonarqube", Name: "SonarQube", Category: CatDevelopment, IconKey: "sonarqube",
		DefaultPort: 9000, HealthPath: "/api/system/status",
		DockerImages: []string{"sonarqube"},
		Description:  "Continuous code quality and security analysis",
	},
	"vault": {
		Key: "vault", Name: "Vault", Category: CatDevelopment, IconKey: "vault",
		DefaultPort: 8200, HealthPath: "/v1/sys/health",
		DockerImages: []string{"hashicorp/vault"},
		Description:  "Secrets management, encryption, and identity tool",
	},
	"huginn": {
		Key: "huginn", Name: "Huginn", Category: CatDevelopment, IconKey: "huginn",
		DefaultPort: 3000, HealthPath: "/",
		DockerImages: []string{"ghcr.io/huginn/huginn"},
		Description:  "Self-hosted agent automation and monitoring platform",
	},

	// ─── Productivity ────────────────────────────────────────────────────
	"bookstack": {
		Key: "bookstack", Name: "BookStack", Category: CatProductivity, IconKey: "bookstack",
		DefaultPort: 80, HealthPath: "/status",
		DockerImages: []string{"linuxserver/bookstack", "solidnerd/bookstack"},
		Description:  "Self-hosted wiki and documentation platform",
	},
	"wikijs": {
		Key: "wikijs", Name: "Wiki.js", Category: CatProductivity, IconKey: "wikijs",
		DefaultPort: 3000, HealthPath: "/healthz",
		DockerImages: []string{"linuxserver/wikijs", "ghcr.io/requarks/wiki"},
		Description:  "Modern and powerful wiki engine",
	},
	"paperless": {
		Key: "paperless", Name: "Paperless-ngx", Category: CatProductivity, IconKey: "paperless",
		DefaultPort: 8000, HealthPath: "/api/",
		DockerImages: []string{"ghcr.io/paperless-ngx/paperless-ngx"},
		Description:  "Document management system with OCR",
	},
	"mealie": {
		Key: "mealie", Name: "Mealie", Category: CatProductivity, IconKey: "mealie",
		DefaultPort: 9925, HealthPath: "/api/app/about",
		DockerImages: []string{"ghcr.io/mealie-recipes/mealie", "hkotel/mealie"},
		Description:  "Self-hosted recipe manager and meal planner",
	},
	"immich": {
		Key: "immich", Name: "Immich", Category: CatProductivity, IconKey: "immich",
		DefaultPort: 2283, HealthPath: "/api/server-info/ping",
		DockerImages: []string{"ghcr.io/immich-app/immich-server"},
		Description:  "Self-hosted photo and video management",
	},
	"vikunja": {
		Key: "vikunja", Name: "Vikunja", Category: CatProductivity, IconKey: "vikunja",
		DefaultPort: 3456, HealthPath: "/api/v1/info",
		DockerImages: []string{"vikunja/vikunja"},
		Description:  "Self-hosted to-do and project management app",
	},
	"outline": {
		Key: "outline", Name: "Outline", Category: CatProductivity, IconKey: "outline",
		DefaultPort: 3000, HealthPath: "/api/auth.config",
		DockerImages: []string{"outlinewiki/outline"},
		Description:  "Team knowledge base and wiki platform",
	},
	"hedgedoc": {
		Key: "hedgedoc", Name: "HedgeDoc", Category: CatProductivity, IconKey: "hedgedoc",
		DefaultPort: 3000, HealthPath: "/status",
		DockerImages: []string{"quay.io/hedgedoc/hedgedoc"},
		Description:  "Real-time collaborative markdown editor",
	},
	"stirling-pdf": {
		Key: "stirling-pdf", Name: "Stirling PDF", Category: CatProductivity, IconKey: "stirling-pdf",
		DefaultPort: 8080, HealthPath: "/api/v1/info/status",
		DockerImages: []string{"frooodle/s-pdf"},
		Description:  "Self-hosted PDF manipulation and conversion tools",
	},
	"grocy": {
		Key: "grocy", Name: "Grocy", Category: CatProductivity, IconKey: "grocy",
		DefaultPort: 80, HealthPath: "/",
		DockerImages: []string{"linuxserver/grocy"},
		Description:  "Self-hosted grocery and household management",
	},
	"actual-budget": {
		Key: "actual-budget", Name: "Actual Budget", Category: CatProductivity, IconKey: "actual-budget",
		DefaultPort: 5006, HealthPath: "/",
		DockerImages: []string{"actualbudget/actual-server"},
		Description:  "Privacy-focused local-first personal budgeting app",
	},
	"planka": {
		Key: "planka", Name: "Planka", Category: CatProductivity, IconKey: "planka",
		DefaultPort: 1337, HealthPath: "/",
		DockerImages: []string{"ghcr.io/plankanban/planka"},
		Description:  "Self-hosted Kanban board for project management",
	},
	"wekan": {
		Key: "wekan", Name: "WeKan", Category: CatProductivity, IconKey: "wekan",
		DefaultPort: 8080, HealthPath: "/",
		DockerImages: []string{"wekanteam/wekan"},
		Description:  "Open-source Kanban board application",
	},
	"tandoor": {
		Key: "tandoor", Name: "Tandoor Recipes", Category: CatProductivity, IconKey: "tandoor-recipes",
		DefaultPort: 8080, HealthPath: "/",
		DockerImages: []string{"vabene1111/recipes"},
		Description:  "Self-hosted recipe manager and meal planner",
	},
	"firefly-iii": {
		Key: "firefly-iii", Name: "Firefly III", Category: CatProductivity, IconKey: "firefly-iii",
		DefaultPort: 8080, HealthPath: "/api/v1/about",
		DockerImages: []string{"fireflyiii/core"},
		Description:  "Self-hosted personal finance and budget manager",
	},
	"monica": {
		Key: "monica", Name: "Monica", Category: CatProductivity, IconKey: "monica",
		DefaultPort: 80, HealthPath: "/",
		DockerImages: []string{"monica"},
		Description:  "Personal CRM to track relationships and interactions",
	},
	"docmost": {
		Key: "docmost", Name: "Docmost", Category: CatProductivity, IconKey: "docmost",
		DefaultPort: 3000, HealthPath: "/",
		DockerImages: []string{"docmost/docmost"},
		Description:  "Collaborative documentation and wiki platform",
	},
	"etherpad": {
		Key: "etherpad", Name: "Etherpad", Category: CatProductivity, IconKey: "etherpad",
		DefaultPort: 9001, HealthPath: "/api/",
		DockerImages: []string{"etherpad/etherpad"},
		Description:  "Real-time collaborative text editor",
	},
}

// imageIndex maps normalized Docker image names to service keys.
var imageIndex map[string]string

// portIndex maps default ports to service keys.
var portIndex map[int]string

// uniquePortIndex maps ports to service keys only when exactly one service uses that default port.
var uniquePortIndex map[int]string

// hintIndex maps normalized service hints (names/domains/container labels) to service keys.
var hintIndex map[string]string

func init() {
	buildRegistryIndexes()
	registerBaselineHintAliases()
}
