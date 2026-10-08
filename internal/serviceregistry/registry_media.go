package serviceregistry

// knownMediaServices owns the media catalog.
func knownMediaServices() map[string]KnownService {
	return map[string]KnownService{
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
	}
}
