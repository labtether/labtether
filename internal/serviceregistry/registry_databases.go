package serviceregistry

// knownDatabasesServices owns the databases catalog.
func knownDatabasesServices() map[string]KnownService {
	return map[string]KnownService{
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
	}
}
