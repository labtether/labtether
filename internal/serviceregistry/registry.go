package serviceregistry

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
var registry = buildKnownServiceRegistry()

// Assemble every catalog before init builds the lookup indexes.
func buildKnownServiceRegistry() map[string]KnownService {
	services := make(map[string]KnownService)
	for _, catalog := range []map[string]KnownService{
		knownMediaServices(),
		knownGamingServices(),
		knownDownloadsServices(),
		knownNetworkingServices(),
		knownMonitoringServices(),
		knownManagementServices(),
		knownStorageServices(),
		knownHomeAutomationServices(),
		knownSecurityServices(),
		knownDatabasesServices(),
		knownDevelopmentServices(),
		knownProductivityServices(),
	} {
		for key, service := range catalog {
			services[key] = service
		}
	}
	return services
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
