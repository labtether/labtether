.PHONY: help fmt lint test check check-docs build bootstrap install-compose upgrade-compose setup-doctor db-backup db-restore package-ha-addon-repo security-gosec coverage-portainer perf-gate dev-up dev-up-restart dev-backend dev-backend-bg dev-backend-bg-restart dev-backend-stop dev-frontend dev-frontend-bg dev-frontend-bg-restart dev-frontend-stop dev-stop-all db-migrate db-migrate-status compose-up compose-up-fast compose-down compose-logs smoke-test desktop-smoke-test integration-test check-port build-agent-linux check-go check-docker check-docker-compose check-node check-npm check-ps check-find

help:
	@echo "Targets:"
	@echo "  fmt            - Run gofmt on Go sources"
	@echo "  lint           - Run go vet"
	@echo "  test           - Run go test ./..."
	@echo "  check          - Quick validation: go vet + go test + tsc --noEmit"
	@echo "  check-docs     - Docs quality gates: markdown lint + local broken-link check"
	@echo "  build          - Build hub binary to build/labtether"
	@echo "  bootstrap      - One-command first-time setup (env + migrate + compose + smoke + doctor)"
	@echo "  install-compose - Prepare .env.deploy and start the release-image deploy stack (VERSION=vX.Y.Z)"
	@echo "  upgrade-compose - Pull and restart the release-image deploy stack at a new version (VERSION=vX.Y.Z)"
	@echo "  setup-doctor   - Validate setup readiness and runtime health"
	@echo "  db-backup      - Backup Postgres to backups/ (pg_dump + gzip)"
	@echo "  db-restore     - Restore Postgres from backup file (BACKUP_FILE=path.sql.gz)"
	@echo "  package-ha-addon-repo - Build Home Assistant add-on repository layout + tarball artifact"
	@echo "  security-gosec - Run gosec with strict reviewed allowlist enforcement"
	@echo "  coverage-portainer - Enforce strict Portainer-only 100% coverage gate"
	@echo "  perf-gate      - Run backend hotspot perf-contract guard tests"
	@echo "  dev-up         - Start backend + frontend dev runtimes in one command"
	@echo "  dev-up-restart - Restart backend + frontend dev runtimes and print install command"
	@echo "  dev-backend-bg - Build and run Go backend in the background (macOS direct process, Linux tmux)"
	@echo "  dev-backend-bg-restart - Restart background backend runtime"
	@echo "  dev-backend-stop - Stop background backend runtime"
	@echo "  dev-frontend-bg - Run Next.js dev server in tmux session (background, default dev workflow)"
	@echo "  dev-frontend-bg-restart - Restart frontend tmux session"
	@echo "  dev-stop-all   - Stop backend/frontend and optionally clean up Simulator + Colima"
	@echo "  dev-frontend-stop - Stop frontend tmux session"
	@echo "  dev-backend    - Build and run Go backend locally (foreground, interactive debugging)"
	@echo "  dev-frontend   - Run Next.js dev server (foreground, interactive debugging)"
	@echo "  db-migrate     - Apply DB migrations"
	@echo "  db-migrate-status - Show applied migrations and checksum status (requires psql)"
	@echo "  compose-up     - Start full stack via Docker (rebuild images)"
	@echo "  compose-up-fast - Start full stack via Docker using existing images"
	@echo "  compose-down   - Stop Docker stack"
	@echo "  compose-logs   - Tail Docker compose logs"
	@echo "  smoke-test     - Run smoke tests against running backend"
	@echo "  desktop-smoke-test - Run desktop session smoke checks against a real target"
	@echo "  integration-test - Run async queue integration flow checks"
	@echo "  check-port     - Validate port-3000 is free for local frontend run"

check-go:
	@command -v go >/dev/null 2>&1 || { echo "Missing required command: go"; exit 1; }

check-find:
	@command -v find >/dev/null 2>&1 || { echo "Missing required command: find"; exit 1; }

check-docker:
	@command -v docker >/dev/null 2>&1 || { echo "Missing required command: docker"; exit 1; }

check-docker-compose:
	@docker compose version >/dev/null 2>&1 || command -v docker-compose >/dev/null 2>&1 || { echo "Missing required command: docker compose (or docker-compose)"; exit 1; }

check-node:
	@command -v node >/dev/null 2>&1 || { echo "Missing required command: node"; exit 1; }

check-npm:
	@command -v npm >/dev/null 2>&1 || { echo "Missing required command: npm"; exit 1; }

check-ps:
	@command -v ps >/dev/null 2>&1 || { echo "Missing required command: ps"; exit 1; }

fmt: check-go check-find
	@gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

lint: check-go
	@go vet ./...

test: check-go
	@go test ./...

check: check-go check-node check-npm
	@echo "Running go vet..."
	@go vet ./...
	@echo "Running go test..."
	@go test ./...
	@echo "Running tsc --noEmit..."
	@cd web/console && npx tsc --noEmit
	@echo "All checks passed."

check-docs: check-node check-npm
	@./scripts/check-docs.sh

build: check-go
	@mkdir -p build
	@go build -o build/labtether ./cmd/labtether
	@echo "Built: build/labtether"

bootstrap:
	@./scripts/bootstrap.sh

install-compose:
	@./scripts/install-compose.sh --version "$(VERSION)"

upgrade-compose:
	@./scripts/upgrade-compose.sh --version "$(VERSION)"

setup-doctor:
	@./scripts/setup-doctor.sh

db-backup:
	@bash scripts/db-backup.sh

db-restore:
	@if [ -z "$(BACKUP_FILE)" ]; then \
		echo "Usage: make db-restore BACKUP_FILE=backups/labtether_YYYYMMDD_HHMMSS.sql.gz"; \
		exit 1; \
	fi
	@bash scripts/db-restore.sh --yes "$(BACKUP_FILE)"

package-ha-addon-repo:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make package-ha-addon-repo VERSION=0.1.0 IMAGE_PREFIX=ghcr.io/<owner>/labtether-homeassistant-addon"; \
		exit 1; \
	fi
	@if [ -z "$(IMAGE_PREFIX)" ]; then \
		echo "Usage: make package-ha-addon-repo VERSION=0.1.0 IMAGE_PREFIX=ghcr.io/<owner>/labtether-homeassistant-addon"; \
		exit 1; \
	fi
	@./scripts/release/package-ha-addon-repo.sh --version "$(VERSION)" --image-prefix "$(IMAGE_PREFIX)"

security-gosec: check-go
	@./scripts/check-gosec-allowlist.sh

coverage-portainer: check-go
	@./scripts/check-portainer-coverage.sh

perf-gate: check-go
	@./scripts/perf/backend-hotspot-gate.sh

dev-up: check-go check-node check-npm
	@./scripts/dev-up.sh

dev-up-restart: check-go check-node check-npm
	@./scripts/dev-up.sh --restart

dev-backend: check-go
	@echo "Building Go backend..."
	@mkdir -p build
	@go build -o build/labtether ./cmd/labtether
	@echo "Starting backend on :8080 (Postgres must be running)..."
	@set -a; \
	if [ -f .env ]; then . ./.env; fi; \
	set +a; \
	DATABASE_URL="$${DATABASE_URL:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}" \
	LABTETHER_OWNER_TOKEN="$${LABTETHER_OWNER_TOKEN:-dev-owner-token-change-me}" \
	LABTETHER_ADMIN_PASSWORD="$${LABTETHER_ADMIN_PASSWORD-password}" \
	LABTETHER_ENCRYPTION_KEY="$${LABTETHER_ENCRYPTION_KEY:?LABTETHER_ENCRYPTION_KEY must be set (generate: openssl rand -base64 32)}" \
	LABTETHER_TLS_MODE="$${LABTETHER_TLS_MODE:-auto}" \
	API_PORT="$${API_PORT:-8080}" \
	./build/labtether

dev-backend-bg: check-go
	@./scripts/dev-backend-bg.sh

dev-backend-bg-restart: check-go
	@./scripts/dev-backend-bg.sh --restart

dev-backend-stop: check-go
	@./scripts/dev-backend-stop.sh

dev-frontend: check-node check-npm
	@echo "Starting Next.js dev server on :3000..."
	@cd web/console && npm run dev

dev-frontend-bg: check-node check-npm
	@./scripts/dev-frontend-bg.sh

dev-frontend-bg-restart: check-node check-npm
	@./scripts/dev-frontend-bg.sh --restart

dev-frontend-stop: check-node check-npm
	@./scripts/dev-frontend-stop.sh

dev-stop-all: check-node check-npm
	@./scripts/dev-stop-all.sh

db-migrate: check-go
	@./scripts/db-migrate.sh

db-migrate-status:
	@./scripts/db-migrate-status.sh

check-port: check-ps
	@if nc -z localhost 3000 2>/dev/null; then \
		conflict=$$(pgrep -f 'next-server|next dev|node.*3000' 2>/dev/null | while read pid; do \
			cmd=$$(ps -o comm= -p $$pid 2>/dev/null); \
			if [ "$$cmd" != "ssh" ] && [ "$$cmd" != "com.docker.b" ] && [ "$$cmd" != "docker" ] && [ "$$cmd" != "vpnkit-bridg" ]; then \
				echo "$$pid ($$cmd)"; \
			fi; \
		done); \
		if [ -n "$$conflict" ]; then \
			echo "ERROR: Port 3000 is in use by non-Docker processes:"; \
			echo "$$conflict"; \
			echo "Kill them first: pgrep -f 'next-server|next dev' | xargs kill -9"; \
			exit 1; \
		fi; \
	fi

compose-up: check-port check-docker-compose
	@if docker compose version >/dev/null 2>&1; then \
		docker compose up -d --build; \
	else \
		docker-compose up -d --build; \
	fi

compose-up-fast: check-port check-docker-compose
	@if docker compose version >/dev/null 2>&1; then \
		docker compose up -d; \
	else \
		docker-compose up -d; \
	fi

compose-down: check-docker-compose
	@if docker compose version >/dev/null 2>&1; then \
		docker compose down; \
	else \
		docker-compose down; \
	fi

compose-logs: check-docker-compose
	@if docker compose version >/dev/null 2>&1; then \
		docker compose logs -f --tail=200; \
	else \
		docker-compose logs -f --tail=200; \
	fi

smoke-test:
	@./scripts/smoke-test.sh

desktop-smoke-test:
	@./scripts/desktop-smoke-test.sh

integration-test:
	@./scripts/integration-queue-flow.sh

# --- Linux Agent ---

build-agent-linux: check-go
	@echo "Building LabTether agent for Linux..."
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/labtether-agent-linux-amd64 ./agents/labtether-agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o build/labtether-agent-linux-arm64 ./agents/labtether-agent
	@echo "Linux agent binaries: build/labtether-agent-linux-amd64, build/labtether-agent-linux-arm64"

# --- Windows Agent ---

build-agent-windows: check-go
	@echo "Building LabTether agent for Windows..."
	@mkdir -p build
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o build/labtether-agent-windows-amd64.exe ./agents/labtether-agent
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o build/labtether-agent-windows-arm64.exe ./agents/labtether-agent
	@echo "Windows agent binaries: build/labtether-agent-windows-amd64.exe, build/labtether-agent-windows-arm64.exe"

# --- All Agents ---

build-agent-all: build-agent-linux build-agent-windows

