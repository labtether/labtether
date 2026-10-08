package serviceregistry

// knownDownloadsServices owns the downloads catalog.
func knownDownloadsServices() map[string]KnownService {
	return map[string]KnownService{
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
	}
}
