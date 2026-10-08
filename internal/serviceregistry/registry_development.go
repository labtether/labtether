package serviceregistry

// knownDevelopmentServices owns the development catalog.
func knownDevelopmentServices() map[string]KnownService {
	return map[string]KnownService{
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
	}
}
