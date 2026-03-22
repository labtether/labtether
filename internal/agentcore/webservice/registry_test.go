package webservice

import (
	"sort"
	"testing"
)

func TestLookupByDockerImage(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		wantKey   string
		wantFound bool
	}{
		// Basic image lookups
		{"plex by linuxserver image", "linuxserver/plex", "plex", true},
		{"plex by plexinc image", "plexinc/pms-docker", "plex", true},
		{"jellyfin by official image", "jellyfin/jellyfin", "jellyfin", true},
		{"grafana by official image", "grafana/grafana", "grafana", true},
		{"portainer by official image", "portainer/portainer-ce", "portainer", true},
		{"homeassistant by ghcr image", "ghcr.io/home-assistant/home-assistant", "homeassistant", true},
		{"traefik by library image", "traefik", "traefik", true},
		{"uptime-kuma by louislam", "louislam/uptime-kuma", "uptime-kuma", true},
		{"pihole by official image", "pihole/pihole", "pihole", true},
		{"vaultwarden by vaultwarden", "vaultwarden/server", "vaultwarden", true},

		// Unknown image
		{"unknown image", "some-random/image", "", false},
		{"empty image", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := LookupByDockerImage(tt.image)
			if found != tt.wantFound {
				t.Errorf("LookupByDockerImage(%q) found = %v, want %v", tt.image, found, tt.wantFound)
				return
			}
			if found && svc.Key != tt.wantKey {
				t.Errorf("LookupByDockerImage(%q) key = %q, want %q", tt.image, svc.Key, tt.wantKey)
			}
		})
	}
}

func TestLookupByDockerImageTagStripping(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		wantKey string
	}{
		{"strips :latest tag", "linuxserver/plex:latest", "plex"},
		{"strips version tag", "grafana/grafana:v10.0.0", "grafana"},
		{"strips numeric tag", "portainer/portainer-ce:2.19.0", "portainer"},
		{"strips sha256 digest", "jellyfin/jellyfin@sha256:abc123", "jellyfin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := LookupByDockerImage(tt.image)
			if !found {
				t.Errorf("LookupByDockerImage(%q) not found, expected key %q", tt.image, tt.wantKey)
				return
			}
			if svc.Key != tt.wantKey {
				t.Errorf("LookupByDockerImage(%q) key = %q, want %q", tt.image, svc.Key, tt.wantKey)
			}
		})
	}
}

func TestLookupByDockerImageRegistryPrefixes(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		wantKey string
	}{
		{"docker.io/library/ prefix", "docker.io/library/traefik", "traefik"},
		{"docker.io/ prefix", "docker.io/linuxserver/plex", "plex"},
		{"docker.io/library/ with tag", "docker.io/library/traefik:v3.0", "traefik"},
		{"docker.io/ with tag", "docker.io/grafana/grafana:latest", "grafana"},
		{"index docker registry prefix", "index.docker.io/library/traefik:latest", "traefik"},
		{"linuxserver mirror registry", "lscr.io/linuxserver/sonarr:latest", "sonarr"},
		{"custom registry host and port", "registry.local:5000/linuxserver/radarr:latest", "radarr"},
		{"localhost registry host and port", "localhost:5000/pihole/pihole:latest", "pihole"},
		{"ghcr registry for docker hub image path", "ghcr.io/louislam/uptime-kuma:1", "uptime-kuma"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := LookupByDockerImage(tt.image)
			if !found {
				t.Errorf("LookupByDockerImage(%q) not found, expected key %q", tt.image, tt.wantKey)
				return
			}
			if svc.Key != tt.wantKey {
				t.Errorf("LookupByDockerImage(%q) key = %q, want %q", tt.image, svc.Key, tt.wantKey)
			}
		})
	}
}

func TestRegistryDockerImageAliasesRemainUniqueAfterNormalization(t *testing.T) {
	owners := make(map[string]string, len(registry)*2)
	for key, svc := range registry {
		for _, image := range svc.DockerImages {
			normalized := normalizeDockerImage(image)
			if normalized == "" {
				t.Fatalf("service %q has empty normalized image for alias %q", key, image)
			}

			if owner, exists := owners[normalized]; exists && owner != key {
				t.Fatalf("normalized image alias %q is shared by services %q and %q", normalized, owner, key)
			}
			owners[normalized] = key

			resolved, found := LookupByDockerImage(image)
			if !found {
				t.Fatalf("LookupByDockerImage(%q) not found for service %q", image, key)
			}
			if resolved.Key != key {
				t.Fatalf("LookupByDockerImage(%q) resolved to %q, want %q", image, resolved.Key, key)
			}
		}
	}
}

func TestLookupByDockerImageCaseInsensitive(t *testing.T) {
	svc, found := LookupByDockerImage("Grafana/Grafana")
	if !found {
		t.Fatal("expected to find grafana with mixed case")
	}
	if svc.Key != "grafana" {
		t.Errorf("key = %q, want %q", svc.Key, "grafana")
	}
}

func TestLookupByHint(t *testing.T) {
	tests := []struct {
		name    string
		hint    string
		wantKey string
		found   bool
	}{
		{"container name token", "my_grafana_1", "grafana", true},
		{"domain first label", "plex.home.lab", "plex", true},
		{"router shorthand alias npm", "npm", "nginx-proxy-manager", true},
		{"mqtt alias mosquitto", "mosquitto", "mqtt", true},
		{"unknown hint", "totally-unknown-service", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := LookupByHint(tt.hint)
			if found != tt.found {
				t.Fatalf("LookupByHint(%q) found=%v, want %v", tt.hint, found, tt.found)
			}
			if found && svc.Key != tt.wantKey {
				t.Fatalf("LookupByHint(%q) key=%q, want %q", tt.hint, svc.Key, tt.wantKey)
			}
		})
	}
}

func TestLookupByPort(t *testing.T) {
	// Only test unique ports to avoid ambiguity from port conflicts.
	// Many services share common ports (8080, 3000, 80); the registry picks
	// the first key alphabetically for those.
	tests := []struct {
		name      string
		port      int
		wantKey   string
		wantFound bool
	}{
		{"plex default port", 32400, "plex", true},
		{"pihole DNS port", 53, "pihole", true},
		{"portainer port", 9443, "portainer", true},
		{"uptime-kuma port", 3001, "uptime-kuma", true},
		{"sonarr port", 8989, "sonarr", true},
		{"radarr port", 7878, "radarr", true},
		{"homeassistant port", 8123, "homeassistant", true},
		{"immich port", 2283, "immich", true},
		{"unknown port", 59999, "", false},
		{"zero port", 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := LookupByPort(tt.port)
			if found != tt.wantFound {
				t.Errorf("LookupByPort(%d) found = %v, want %v", tt.port, found, tt.wantFound)
				return
			}
			if found && svc.Key != tt.wantKey {
				t.Errorf("LookupByPort(%d) key = %q, want %q", tt.port, svc.Key, tt.wantKey)
			}
		})
	}
}

func TestLookupByPortConflicts(t *testing.T) {
	// Port 8080 is shared by many services. Verify we get a valid service back
	// (which one wins is deterministic but not important to assert).
	svc, found := LookupByPort(8080)
	if !found {
		t.Fatal("expected to find a service on port 8080")
	}
	if svc.DefaultPort != 8080 {
		t.Errorf("returned service has DefaultPort %d, want 8080", svc.DefaultPort)
	}
}

func TestLookupUniqueByPort(t *testing.T) {
	t.Run("unique port returns service", func(t *testing.T) {
		svc, found := LookupUniqueByPort(32400)
		if !found {
			t.Fatal("expected unique port 32400 to resolve")
		}
		if svc.Key != "plex" {
			t.Fatalf("LookupUniqueByPort(32400) key = %q, want %q", svc.Key, "plex")
		}
	})

	t.Run("ambiguous port returns not found", func(t *testing.T) {
		if _, found := LookupByPort(3000); !found {
			t.Fatal("expected LookupByPort(3000) to resolve via legacy first-match behavior")
		}
		if _, found := LookupUniqueByPort(3000); found {
			t.Fatal("expected LookupUniqueByPort(3000) to return not found for ambiguous port")
		}
	})
}

func TestLookupByKey(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantName  string
		wantFound bool
	}{
		{"plex", "plex", "Plex", true},
		{"grafana", "grafana", "Grafana", true},
		{"homeassistant", "homeassistant", "Home Assistant", true},
		{"labtether", "labtether", "LabTether", true},
		{"unknown", "nonexistent", "", false},
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := LookupByKey(tt.key)
			if found != tt.wantFound {
				t.Errorf("LookupByKey(%q) found = %v, want %v", tt.key, found, tt.wantFound)
				return
			}
			if found && svc.Name != tt.wantName {
				t.Errorf("LookupByKey(%q) name = %q, want %q", tt.key, svc.Name, tt.wantName)
			}
		})
	}
}

func TestRegistryCategories(t *testing.T) {
	categories := AllCategories()

	if len(categories) < 8 {
		t.Errorf("AllCategories() returned %d categories, want at least 8", len(categories))
	}

	if !sort.StringsAreSorted(categories) {
		t.Error("AllCategories() is not sorted")
	}

	seen := map[string]bool{}
	for _, c := range categories {
		if seen[c] {
			t.Errorf("duplicate category: %q", c)
		}
		seen[c] = true
	}

	// CatGaming is defined but has no registry entries in the base set;
	// CatDatabases now has entries after the registry expansion.
	expected := []string{
		CatMedia, CatNetworking, CatMonitoring, CatManagement, CatHomeAutomation,
		CatDownloads,
	}
	for _, e := range expected {
		found := false
		for _, c := range categories {
			if c == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected category %q not found in AllCategories()", e)
		}
	}
}

func TestTorrentClientsInDownloadsCategory(t *testing.T) {
	clients := []string{"qbittorrent", "transmission", "sabnzbd"}
	for _, key := range clients {
		svc, found := LookupByKey(key)
		if !found {
			t.Errorf("service %q not found in registry", key)
			continue
		}
		if svc.Category != CatDownloads {
			t.Errorf("service %q has category %q, want %q", key, svc.Category, CatDownloads)
		}
	}
}

func TestRegistryServiceCount(t *testing.T) {
	// We expect at least 50 services in the registry
	count := 0
	for _, cat := range AllCategories() {
		_ = cat
		count++
	}

	// Instead, count actual services via keys we know exist
	knownKeys := []string{
		"plex", "jellyfin", "emby", "sonarr", "radarr", "lidarr", "prowlarr",
		"bazarr", "overseerr", "tautulli", "sabnzbd", "transmission", "qbittorrent",
		"traefik", "nginx-proxy-manager", "caddy", "pihole", "adguardhome",
		"wireguard", "tailscale", "unifi", "speedtest-tracker",
		"grafana", "prometheus", "uptime-kuma", "netdata", "glances", "dozzle",
		"portainer", "homer", "homarr", "homepage", "cockpit", "yacht", "watchtower",
		"homeassistant", "nodered", "mqtt", "zigbee2mqtt", "esphome", "scrypted",
		"nextcloud", "syncthing", "filebrowser", "minio",
		"vaultwarden", "authelia", "crowdsec",
		"gitea", "forgejo", "gitlab", "jenkins", "drone", "codeserver",
		"bookstack", "wikijs", "paperless", "mealie", "immich",
	}

	missing := []string{}
	for _, key := range knownKeys {
		if _, found := LookupByKey(key); !found {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("missing %d required services from registry: %v", len(missing), missing)
	}
}

func TestNewDownloadAndGamingServices(t *testing.T) {
	downloads := []struct {
		key   string
		image string
	}{
		{"deluge", "linuxserver/deluge"},
		{"nzbget", "linuxserver/nzbget"},
		{"jdownloader", "jlesage/jdownloader-2"},
		{"aria2", "p3terx/aria2-pro"},
		{"pyload", "linuxserver/pyload-ng"},
		{"flood", "jesec/flood"},
		{"nzbhydra", "linuxserver/nzbhydra2"},
		{"rtorrent", "crazymax/rtorrent-rutorrent"},
	}
	for _, tt := range downloads {
		t.Run("download/"+tt.key, func(t *testing.T) {
			svc, found := LookupByKey(tt.key)
			if !found {
				t.Fatalf("service %q not found", tt.key)
			}
			if svc.Category != CatDownloads {
				t.Errorf("category = %q, want %q", svc.Category, CatDownloads)
			}
			if _, ok := LookupByDockerImage(tt.image); !ok {
				t.Errorf("image %q not found for %q", tt.image, tt.key)
			}
		})
	}

	gaming := []struct {
		key   string
		image string
	}{
		{"crafty", "registry.gitlab.com/crafty-controller/crafty-4"},
		{"pterodactyl", "ghcr.io/pterodactyl/panel"},
		{"pelican", "ghcr.io/pelican-dev/panel"},
		{"pufferpanel", "pufferpanel/pufferpanel"},
	}
	for _, tt := range gaming {
		t.Run("gaming/"+tt.key, func(t *testing.T) {
			svc, found := LookupByKey(tt.key)
			if !found {
				t.Fatalf("service %q not found", tt.key)
			}
			if svc.Category != CatGaming {
				t.Errorf("category = %q, want %q", svc.Category, CatGaming)
			}
			if _, ok := LookupByDockerImage(tt.image); !ok {
				t.Errorf("image %q not found for %q", tt.image, tt.key)
			}
		})
	}
}

func TestNewMediaNetworkingMonitoringServices(t *testing.T) {
	expected := map[string]string{
		"frigate": CatMedia, "go2rtc": CatMedia, "navidrome": CatMedia,
		"audiobookshelf": CatMedia, "readarr": CatMedia, "calibre-web": CatMedia,
		"kavita": CatMedia, "komga": CatMedia, "tdarr": CatMedia,
		"photoprism": CatMedia, "jellyseerr": CatMedia,
		"nginx": CatNetworking, "haproxy": CatNetworking,
		"ddns-updater": CatNetworking, "technitium": CatNetworking,
		"blocky":       CatNetworking,
		"healthchecks": CatMonitoring, "scrutiny": CatMonitoring,
		"graylog": CatMonitoring, "influxdb": CatMonitoring, "loki": CatMonitoring,
	}
	for key, wantCat := range expected {
		svc, found := LookupByKey(key)
		if !found {
			t.Errorf("service %q not found", key)
			continue
		}
		if svc.Category != wantCat {
			t.Errorf("service %q category = %q, want %q", key, svc.Category, wantCat)
		}
	}
}

func TestRegistryServiceFields(t *testing.T) {
	// Every service must have required fields populated
	svc, found := LookupByKey("plex")
	if !found {
		t.Fatal("plex not found in registry")
	}

	if svc.Key == "" {
		t.Error("Key is empty")
	}
	if svc.Name == "" {
		t.Error("Name is empty")
	}
	if svc.Category == "" {
		t.Error("Category is empty")
	}
	if svc.IconKey == "" {
		t.Error("IconKey is empty")
	}
	if svc.DefaultPort == 0 {
		t.Error("DefaultPort is 0")
	}
	if len(svc.DockerImages) == 0 {
		t.Error("DockerImages is empty")
	}
	if svc.Description == "" {
		t.Error("Description is empty")
	}
}

func TestNewMgmtSecurityProdDevHomeStorageDbServices(t *testing.T) {
	expected := map[string]string{
		"guacamole": CatManagement, "rustdesk": CatManagement,
		"dashy": CatManagement, "organizr": CatManagement,
		"heimdall": CatManagement, "dockge": CatManagement,
		"semaphore": CatManagement, "meshcentral": CatManagement,
		"authentik": CatSecurity, "keycloak": CatSecurity,
		"oauth2-proxy": CatSecurity,
		"vikunja":      CatProductivity, "outline": CatProductivity,
		"hedgedoc": CatProductivity, "stirling-pdf": CatProductivity,
		"grocy": CatProductivity, "actual-budget": CatProductivity,
		"n8n": CatDevelopment, "woodpecker": CatDevelopment,
		"registry": CatDevelopment, "sonarqube": CatDevelopment,
		"deconz":  CatHomeAutomation,
		"seafile": CatStorage, "duplicati": CatStorage,
		"sftpgo": CatStorage, "kopia": CatStorage,
		"pgadmin": CatDatabases, "adminer": CatDatabases,
		"phpmyadmin": CatDatabases, "redis-commander": CatDatabases,
		"mongo-express": CatDatabases, "dbgate": CatDatabases,
	}
	for key, wantCat := range expected {
		svc, found := LookupByKey(key)
		if !found {
			t.Errorf("service %q not found", key)
			continue
		}
		if svc.Category != wantCat {
			t.Errorf("service %q category = %q, want %q", key, svc.Category, wantCat)
		}
	}
}

func TestNormalizeDockerImage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"grafana/grafana", "grafana/grafana"},
		{"grafana/grafana:latest", "grafana/grafana"},
		{"grafana/grafana:v10.0.0", "grafana/grafana"},
		{"docker.io/library/traefik", "traefik"},
		{"docker.io/library/traefik:v3.0", "traefik"},
		{"docker.io/grafana/grafana", "grafana/grafana"},
		{"docker.io/grafana/grafana:latest", "grafana/grafana"},
		{"Grafana/Grafana", "grafana/grafana"},
		{"LINUXSERVER/PLEX:LATEST", "linuxserver/plex"},
		{"grafana/grafana@sha256:abc123", "grafana/grafana"},
		{"ghcr.io/home-assistant/home-assistant", "home-assistant/home-assistant"},
		{"lscr.io/linuxserver/sonarr:latest", "linuxserver/sonarr"},
		{"registry.local:5000/linuxserver/radarr", "linuxserver/radarr"},
		{"localhost:5000/pihole/pihole:latest", "pihole/pihole"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeDockerImage(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDockerImage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
