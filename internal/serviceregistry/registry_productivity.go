package serviceregistry

// knownProductivityServices owns the productivity catalog.
func knownProductivityServices() map[string]KnownService {
	return map[string]KnownService{
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
}
