package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
)

// handleV2OpenAPI serves a static minimal OpenAPI 3.0 specification that
// documents the v2 endpoints. No authentication is required so that tools
// (curl, Swagger UI, code generators) can fetch the spec without credentials.
func (s *apiServer) handleV2OpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(v2OpenAPISpec))
}

// v2OpenAPISpec is a static OpenAPI 3.0 document covering the v2 API surface.
// Kept as a Go string constant — no build tooling required.
//
// Organisation: paths are grouped by functional area (assets, metrics, docker,
// etc.) with comments marking each section. Scopes required for each endpoint
// are noted in the description field.
const v2OpenAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "LabTether Hub API v2",
    "version": "2.0.0",
    "description": "REST API for the LabTether homelab control plane. All endpoints except /api/v2/openapi.json require authentication via a session cookie, Bearer API key (lt_...), or owner token."
  },
  "servers": [
    { "url": "/", "description": "This hub instance" }
  ],
  "tags": [
    { "name": "meta",           "description": "Metadata and documentation" },
    { "name": "auth",           "description": "Authentication and identity" },
    { "name": "assets",         "description": "Infrastructure assets (servers, VMs, devices)" },
    { "name": "system",         "description": "Per-asset processes, services, networking, storage, packages, users, and logs" },
    { "name": "metrics",        "description": "Telemetry and metric time-series" },
    { "name": "exec",           "description": "Remote command execution" },
    { "name": "actions",        "description": "Saved actions (reusable command templates)" },
    { "name": "schedules",      "description": "Recurring commands executed through the durable hub job queue on connected agent assets" },
    { "name": "groups",         "description": "Asset grouping" },
    { "name": "docker",         "description": "Docker host, container, and stack management" },
    { "name": "files",          "description": "File transfers between assets" },
    { "name": "bulk",           "description": "Bulk operations across multiple assets" },
    { "name": "alerts",         "description": "Alert instances and rules" },
    { "name": "incidents",      "description": "Incident tracking" },
    { "name": "connectors",     "description": "External system connectors (generic)" },
    { "name": "proxmox",        "description": "Proxmox VE integration" },
    { "name": "truenas",        "description": "TrueNAS integration" },
    { "name": "pbs",            "description": "Proxmox Backup Server integration" },
    { "name": "portainer",      "description": "Portainer integration" },
    { "name": "homeassistant",  "description": "Home Assistant integration" },
    { "name": "credentials",    "description": "Credential profiles for asset access" },
    { "name": "terminal",       "description": "Terminal sessions, history, and snippets" },
    { "name": "agents",         "description": "Agent lifecycle and enrollment" },
    { "name": "hub",            "description": "Hub status, TLS, and Tailscale" },
    { "name": "web-services",   "description": "Discovered web services / dashboards" },
    { "name": "collectors",     "description": "Hub-side metric collectors" },
    { "name": "notifications",  "description": "Notification channels and delivery history" },
    { "name": "synthetic",      "description": "Synthetic uptime checks" },
    { "name": "discovery",      "description": "Network discovery and proposals" },
    { "name": "topology",       "description": "Topology canvas: zones, connections, layout" },
    { "name": "dependencies",   "description": "Service dependency graph" },
    { "name": "failover",       "description": "Failover pair management" },
    { "name": "dead-letters",   "description": "Dead-letter queue inspection" },
    { "name": "audit",          "description": "Audit event log" },
    { "name": "logs",           "description": "Log views (saved queries)" },
    { "name": "settings",       "description": "Hub settings (Prometheus, etc.)" },
    { "name": "keys",           "description": "API key management (admin only)" },
    { "name": "webhooks",       "description": "Outgoing webhook configuration" },
    { "name": "events",         "description": "Real-time event stream (WebSocket/SSE)" }
  ],
  "components": {
    "securitySchemes": {
      "sessionCookie": {
        "type": "apiKey",
        "in": "cookie",
        "name": "labtether_session",
        "description": "Session cookie set by POST /auth/login. Identifies the logged-in user."
      },
      "bearerAPIKey": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "lt_...",
        "description": "API key created via POST /api/v2/keys. Prefix: lt_. Scopes are bound to the key at creation."
      },
      "ownerToken": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "owner-token",
        "description": "Owner bootstrap token used during initial setup. Full admin access."
      }
    },
    "schemas": {
      "Error": {
        "type": "object",
        "properties": {
          "request_id": { "type": "string" },
          "error":      { "type": "string" },
          "message":    { "type": "string" },
          "status":     { "type": "integer" }
        }
      }
    }
  },
  "security": [
    { "sessionCookie": [] },
    { "bearerAPIKey": [] },
    { "ownerToken": [] }
  ],
  "paths": {

    "/api/v2/openapi.json": {
      "get": {
        "summary": "OpenAPI specification",
        "description": "Returns this OpenAPI 3.0 document. No authentication required.",
        "security": [],
        "operationId": "getOpenAPI",
        "tags": ["meta"],
        "responses": { "200": { "description": "OpenAPI specification document" } }
      }
    },

    "/api/v2/whoami": {
      "get": {
        "summary": "Current principal identity",
        "description": "Returns the authenticated user, API key, or owner token identity. Scope: any authenticated principal.",
        "operationId": "whoami",
        "tags": ["auth"],
        "responses": { "200": { "description": "Principal info" } }
      }
    },

    "/api/v2/search": {
      "get": {
        "summary": "Cross-resource search",
        "description": "Full-text search across assets, groups, services, and more. Scope: search:read.",
        "operationId": "search",
        "tags": ["meta"],
        "responses": { "200": { "description": "Search results" } }
      }
    },

    "/api/v2/assets": {
      "get":  { "summary": "List assets",  "description": "Scope: assets:read.",  "operationId": "listAssets",  "tags": ["assets"], "responses": { "200": { "description": "Asset list" } } },
      "post": { "summary": "Create asset", "description": "Scope: assets:write.", "operationId": "createAsset", "tags": ["assets"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/assets/{id}": {
      "get":    { "summary": "Get asset",    "description": "Scope: assets:read.",  "operationId": "getAsset",    "tags": ["assets"], "responses": { "200": { "description": "Asset" } } },
      "put":    { "summary": "Update asset", "description": "Scope: assets:write.", "operationId": "updateAsset", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update asset", "description": "Scope: assets:write.", "operationId": "patchAsset", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete asset", "description": "Scope: assets:write.", "operationId": "deleteAsset", "tags": ["assets"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/exec": {
      "post": { "summary": "Execute a command on one asset", "description": "Scope: assets:exec. The asset must be allowed by the authenticated principal and have a connected agent.", "operationId": "execAsset", "tags": ["exec"], "responses": { "200": { "description": "Command result" }, "409": { "description": "Asset agent is offline" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files": {
      "get": { "summary": "List a directory", "description": "Scope: files:read. Supply the absolute path in the path query parameter.", "operationId": "listAssetFiles", "tags": ["files"], "parameters": [{ "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }], "responses": { "200": { "description": "Directory entries" } } },
      "delete": { "summary": "Delete a file or directory", "description": "Scope: files:write. Supply the absolute path in the path query parameter.", "operationId": "deleteAssetFile", "tags": ["files"], "parameters": [{ "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }], "responses": { "200": { "description": "Delete result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/read": {
      "get": { "summary": "Download a file", "description": "Scope: files:read. Streams the file response rather than wrapping it in the JSON response envelope.", "operationId": "readAssetFile", "tags": ["files"], "parameters": [{ "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }], "responses": { "200": { "description": "File bytes" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/write": {
      "post": { "summary": "Upload a file", "description": "Scope: files:write.", "operationId": "writeAssetFile", "tags": ["files"], "responses": { "200": { "description": "Upload result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/mkdir": {
      "post": { "summary": "Create a directory", "description": "Scope: files:write.", "operationId": "createAssetDirectory", "tags": ["files"], "responses": { "200": { "description": "Create result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/rename": {
      "post": { "summary": "Rename or move a file", "description": "Scope: files:write.", "operationId": "renameAssetFile", "tags": ["files"], "responses": { "200": { "description": "Rename result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/copy": {
      "post": { "summary": "Copy a file", "description": "Scope: files:write.", "operationId": "copyAssetFile", "tags": ["files"], "responses": { "200": { "description": "Copy result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/processes": {
      "get": { "summary": "List running processes", "description": "Scope: processes:read.", "operationId": "listAssetProcesses", "tags": ["system"], "responses": { "200": { "description": "Process list" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/processes/kill": {
      "post": { "summary": "Terminate a process", "description": "Scope: processes:kill.", "operationId": "killAssetProcess", "tags": ["system"], "responses": { "200": { "description": "Termination result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services": {
      "get": { "summary": "List system services", "description": "Scope: services:read.", "operationId": "listAssetServices", "tags": ["system"], "responses": { "200": { "description": "Service list" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services/{name}/start": {
      "post": { "summary": "Start a system service", "description": "Scope: services:write.", "operationId": "startAssetService", "tags": ["system"], "responses": { "200": { "description": "Service action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services/{name}/stop": {
      "post": { "summary": "Stop a system service", "description": "Scope: services:write.", "operationId": "stopAssetService", "tags": ["system"], "responses": { "200": { "description": "Service action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services/{name}/restart": {
      "post": { "summary": "Restart a system service", "description": "Scope: services:write.", "operationId": "restartAssetService", "tags": ["system"], "responses": { "200": { "description": "Service action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/network": {
      "get": { "summary": "List network interfaces", "description": "Scope: network:read.", "operationId": "listAssetNetwork", "tags": ["system"], "responses": { "200": { "description": "Network interfaces" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/disks": {
      "get": { "summary": "List disks and filesystems", "description": "Scope: disks:read.", "operationId": "listAssetDisks", "tags": ["system"], "responses": { "200": { "description": "Disk inventory" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages": {
      "get": { "summary": "List installed packages", "description": "Scope: packages:read.", "operationId": "listAssetPackages", "tags": ["system"], "responses": { "200": { "description": "Installed packages" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/upgradable": {
      "get": { "summary": "List upgradable packages", "description": "Scope: packages:read.", "operationId": "listAssetUpgradablePackages", "tags": ["system"], "responses": { "200": { "description": "Upgradable packages" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/install": {
      "post": { "summary": "Install a package", "description": "Scope: packages:write.", "operationId": "installAssetPackage", "tags": ["system"], "responses": { "200": { "description": "Package action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/update": {
      "post": { "summary": "Update package metadata", "description": "Scope: packages:write.", "operationId": "updateAssetPackageMetadata", "tags": ["system"], "responses": { "200": { "description": "Package action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/upgrade": {
      "post": { "summary": "Upgrade packages", "description": "Scope: packages:write.", "operationId": "upgradeAssetPackages", "tags": ["system"], "responses": { "200": { "description": "Package action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/cron": {
      "get": { "summary": "List scheduled operating-system jobs", "description": "Scope: cron:read.", "operationId": "listAssetCron", "tags": ["system"], "responses": { "200": { "description": "Scheduled jobs" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/users": {
      "get": { "summary": "List operating-system users", "description": "Scope: users:read.", "operationId": "listAssetUsers", "tags": ["system"], "responses": { "200": { "description": "User list" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/logs": {
      "get": { "summary": "Query asset logs", "description": "Scope: logs:read.", "operationId": "queryAssetLogs", "tags": ["logs"], "responses": { "200": { "description": "Log entries" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/wake": {
      "post": {
        "summary": "Wake an asset",
        "description": "Scope: assets:power. Sends Wake-on-LAN directly from the hub or queues delivery through a correlated online relay agent.",
        "operationId": "wakeAsset",
        "tags": ["assets"],
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "202": { "description": "Magic packet sent directly or queued through an agent relay" },
          "422": { "description": "No valid MAC address is known" }
        }
      }
    },
    "/api/v2/assets/{id}/reboot": {
      "post": {
        "summary": "Reboot an asset",
        "description": "Scope: assets:power. Sends a typed power.action to the connected agent and returns success only after a strictly correlated power.result reports that the operating system accepted the reboot request.",
        "operationId": "rebootAsset",
        "tags": ["assets"],
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "202": { "description": "Operating system accepted the reboot request" },
          "409": { "description": "Agent offline or request rejected" },
          "422": { "description": "Power action unsupported on the agent platform" },
          "429": { "description": "Global power-action concurrency limit reached" },
          "502": { "description": "Delivery or operating-system execution failed" },
          "504": { "description": "Timed out waiting for a correlated agent result" }
        }
      }
    },
    "/api/v2/assets/{id}/shutdown": {
      "post": {
        "summary": "Shut down an asset",
        "description": "Scope: assets:power. Sends a typed power.action to the connected agent and returns success only after a strictly correlated power.result reports that the operating system accepted the shutdown request.",
        "operationId": "shutdownAsset",
        "tags": ["assets"],
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "202": { "description": "Operating system accepted the shutdown request" },
          "409": { "description": "Agent offline or request rejected" },
          "422": { "description": "Power action unsupported on the agent platform" },
          "429": { "description": "Global power-action concurrency limit reached" },
          "502": { "description": "Delivery or operating-system execution failed" },
          "504": { "description": "Timed out waiting for a correlated agent result" }
        }
      }
    },
    "/api/v2/assets/{id}/metrics": {
      "get": {
        "summary": "Asset metric time-series",
        "description": "Scope: metrics:read.",
        "operationId": "getAssetMetrics",
        "tags": ["metrics"],
        "parameters": [
          { "name": "id",     "in": "path",  "required": true, "schema": { "type": "string" } },
          { "name": "window", "in": "query", "schema": { "type": "string", "example": "1h" } },
          { "name": "step",   "in": "query", "schema": { "type": "string", "example": "1m" } }
        ],
        "responses": { "200": { "description": "Metric series" } }
      }
    },
    "/api/v2/assets/{id}/metrics/latest": {
      "get": {
        "summary": "Latest metric snapshot for one asset",
        "description": "Scope: metrics:read.",
        "operationId": "getAssetMetricsLatest",
        "tags": ["metrics"],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": { "description": "Snapshot of current metric values" },
          "404": { "description": "Asset not found" }
        }
      }
    },
    "/api/v2/metrics/overview": {
      "get": {
        "summary": "Metrics overview for all assets",
        "description": "Returns per-asset metric snapshots. Scope: metrics:read.",
        "operationId": "getMetricsOverview",
        "tags": ["metrics"],
        "responses": { "200": { "description": "Per-asset snapshots" } }
      }
    },
    "/api/v2/metrics/query": {
      "get": {
        "summary": "Cross-asset metric query",
        "description": "Query a specific metric across multiple assets. Scope: metrics:read.",
        "operationId": "queryMetrics",
        "tags": ["metrics"],
        "parameters": [
          { "name": "asset_ids", "in": "query", "required": true, "schema": { "type": "string", "description": "Comma-separated asset IDs" } },
          { "name": "metric",    "in": "query", "required": true, "schema": { "type": "string" } },
          { "name": "from",      "in": "query", "schema": { "type": "string", "format": "date-time" } },
          { "name": "to",        "in": "query", "schema": { "type": "string", "format": "date-time" } },
          { "name": "step",      "in": "query", "schema": { "type": "string", "example": "1m" } }
        ],
        "responses": {
          "200": { "description": "Per-asset series results" },
          "400": { "description": "Bad request" }
        }
      }
    },

    "/api/v2/exec": {
      "post": {
        "summary": "Execute command on one or more assets",
        "description": "Fan-out command execution across multiple assets. Scope: assets:exec.",
        "operationId": "execMulti",
        "tags": ["exec"],
        "responses": { "200": { "description": "Per-asset execution results" } }
      }
    },

    "/api/v2/actions": {
      "get":  { "summary": "List saved actions",  "description": "Scope: actions:read. Actions are returned only when every target asset exists and is accessible to the caller.",  "operationId": "listSavedActions",  "tags": ["actions"], "parameters": [{ "name": "limit", "in": "query", "required": false, "schema": { "type": "integer", "minimum": 1, "maximum": 100, "default": 100 } }, { "name": "offset", "in": "query", "required": false, "schema": { "type": "integer", "minimum": 0 } }], "responses": { "200": { "description": "Accessible action list" }, "503": { "description": "Saved action service unavailable" } } },
      "post": { "summary": "Create saved action", "description": "Scope: actions:write. Creation is atomic and every target must exist and be accessible.", "operationId": "createSavedAction", "tags": ["actions"], "requestBody": { "required": true, "content": { "application/json": { "schema": { "type": "object", "required": ["name", "steps"], "properties": { "name": { "type": "string", "minLength": 1, "maxLength": 200 }, "description": { "type": "string", "maxLength": 1000 }, "steps": { "type": "array", "minItems": 1, "maxItems": 50, "items": { "type": "object", "required": ["command", "target"], "properties": { "name": { "type": "string", "maxLength": 200 }, "command": { "type": "string", "minLength": 1, "maxLength": 4096 }, "target": { "type": "string", "minLength": 1, "maxLength": 255 } } } } } } } } }, "responses": { "201": { "description": "Created" }, "400": { "description": "Invalid action or nonexistent target" }, "403": { "description": "At least one target is inaccessible" }, "409": { "description": "Per-actor capacity reached" }, "503": { "description": "Saved action service unavailable" } } }
    },
    "/api/v2/actions/{id}": {
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string", "minLength": 1, "maxLength": 255 } }],
      "get":    { "summary": "Get saved action",     "description": "Scope: actions:read. Returns 404 when any target is missing or inaccessible.",  "operationId": "getSavedAction",    "tags": ["actions"], "responses": { "200": { "description": "Action" }, "404": { "description": "Action absent or not wholly accessible" } } },
      "delete": { "summary": "Delete saved action",  "description": "Scope: actions:write. Returns 404 when any target is missing or inaccessible.", "operationId": "deleteSavedAction", "tags": ["actions"], "responses": { "200": { "description": "Deleted" }, "404": { "description": "Action absent or not wholly accessible" } } }
    },
    "/api/v2/actions/{id}/run": {
      "post": { "summary": "Run saved action", "description": "Scopes: actions:exec and assets:exec. Every target is authorized before any command dispatch. Runs are sequential and bounded to two minutes.", "operationId": "runSavedAction", "tags": ["actions"], "responses": { "200": { "description": "Per-step execution results" }, "404": { "description": "Action absent or not wholly accessible" }, "409": { "description": "Stored action is invalid" }, "503": { "description": "Saved action execution unavailable" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string", "minLength": 1, "maxLength": 255 } }]
    },

    "/api/v2/schedules": {
      "get":  { "summary": "List schedules",  "description": "Scope: schedules:read.",  "operationId": "listSchedules",  "tags": ["schedules"], "responses": { "200": { "description": "Schedule list" } } },
      "post": { "summary": "Create schedule", "description": "Scope: schedules:write; enabled schedules also require actions:exec.", "operationId": "createSchedule", "tags": ["schedules"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/schedules/{id}": {
      "get":    { "summary": "Get schedule",    "description": "Scope: schedules:read.",  "operationId": "getSchedule",    "tags": ["schedules"], "responses": { "200": { "description": "Schedule" } } },
      "patch":  { "summary": "Update schedule", "description": "Scope: schedules:write; an enabled result also requires actions:exec.", "operationId": "updateSchedule", "tags": ["schedules"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete schedule", "description": "Scope: schedules:write.", "operationId": "deleteSchedule", "tags": ["schedules"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/groups": {
      "get":  { "summary": "List groups",  "description": "Scope: groups:read.",  "operationId": "listGroups",  "tags": ["groups"], "responses": { "200": { "description": "Group list" } } },
      "post": { "summary": "Create group", "description": "Scope: groups:write.", "operationId": "createGroup", "tags": ["groups"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/groups/{id}": {
      "get":    { "summary": "Get group",    "description": "Scope: groups:read.",  "operationId": "getGroup",    "tags": ["groups"], "responses": { "200": { "description": "Group" } } },
      "put":    { "summary": "Update group", "description": "Scope: groups:write.", "operationId": "updateGroup", "tags": ["groups"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update group", "description": "Scope: groups:write.", "operationId": "patchGroup", "tags": ["groups"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete group", "description": "Scope: groups:write.", "operationId": "deleteGroup", "tags": ["groups"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/groups/{id}/move": {
	  "put": { "summary": "Move a group", "description": "Scope: groups:write.", "operationId": "moveGroup", "tags": ["groups"], "responses": { "200": { "description": "Moved" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/groups/{id}/reorder": {
	  "put": { "summary": "Reorder a group", "description": "Scope: groups:write.", "operationId": "reorderGroup", "tags": ["groups"], "responses": { "200": { "description": "Reordered" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/docker/hosts": {
	  "get": { "summary": "List Docker hosts", "description": "Scope: docker:read.", "operationId": "listDockerHosts", "tags": ["docker"], "responses": { "200": { "description": "Host list" } } }
    },
    "/api/v2/docker/hosts/{id}": {
	  "get": { "summary": "Get Docker host", "description": "Scope: docker:read.", "operationId": "getDockerHost", "tags": ["docker"], "responses": { "200": { "description": "Host" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/docker/hosts/{id}/containers": {
	  "get": { "summary": "List host containers", "description": "Scope: docker:read.", "operationId": "listDockerHostContainers", "tags": ["docker"], "responses": { "200": { "description": "Container list" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/docker/hosts/{id}/images": {
	  "get": { "summary": "List host images", "description": "Scope: docker:read.", "operationId": "listDockerHostImages", "tags": ["docker"], "responses": { "200": { "description": "Image list" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/docker/hosts/{id}/stacks": {
	  "get": { "summary": "List host stacks", "description": "Scope: docker:read.", "operationId": "listDockerHostStacks", "tags": ["docker"], "responses": { "200": { "description": "Stack list" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/docker/hosts/{id}/action": {
	  "post": { "summary": "Execute a Docker host action", "description": "Scope: docker:write.", "operationId": "executeDockerHostAction", "tags": ["docker"], "responses": { "200": { "description": "Action result" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
    "/api/v2/docker/containers/{id}": {
	  "get": { "summary": "Get Docker container", "description": "Scope: docker:read.", "operationId": "getDockerContainer", "tags": ["docker"], "responses": { "200": { "description": "Container details" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/docker/containers/{id}/stats": {
	  "get": { "summary": "Get Docker container statistics", "description": "Scope: docker:read.", "operationId": "getDockerContainerStats", "tags": ["docker"], "responses": { "200": { "description": "Container statistics" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/docker/containers/{id}/logs": {
	  "get": { "summary": "Get bounded Docker container logs", "description": "Scope: docker:read.", "operationId": "getDockerContainerLogs", "tags": ["docker"], "responses": { "200": { "description": "Container logs" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/docker/containers/{id}/action": {
	  "post": { "summary": "Execute a Docker container action", "description": "Scope: docker:write.", "operationId": "executeDockerContainerAction", "tags": ["docker"], "responses": { "200": { "description": "Action result" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/docker/stacks/{id}/action": {
	  "post": { "summary": "Execute a Docker stack action", "description": "Scope: docker:write.", "operationId": "executeDockerStackAction", "tags": ["docker"], "responses": { "200": { "description": "Action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/updates/plans": {
      "get":  { "summary": "List update plans",  "description": "Scope: updates:read.",  "operationId": "listUpdatePlans",  "tags": ["assets"], "responses": { "200": { "description": "Plan list" } } },
      "post": { "summary": "Create update plan", "description": "Scope: updates:write.", "operationId": "createUpdatePlan", "tags": ["assets"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/updates/plans/{id}": {
      "get":    { "summary": "Get update plan",    "description": "Scope: updates:read.",  "operationId": "getUpdatePlan",    "tags": ["assets"], "responses": { "200": { "description": "Plan" } } },
      "delete": { "summary": "Delete update plan", "description": "Scope: updates:write. Deletion removes terminal run history but is rejected while an associated run is queued or running.", "operationId": "deleteUpdatePlan", "tags": ["assets"], "responses": { "200": { "description": "Deleted" }, "409": { "description": "The plan has queued or running update work" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/updates/plans/{id}/execute": {
	  "post": { "summary": "Execute update plan", "description": "Scope: updates:write.", "operationId": "executeUpdatePlan", "tags": ["assets"], "responses": { "202": { "description": "Queued" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
    "/api/v2/updates/runs": {
      "get": { "summary": "List update runs", "description": "Scope: updates:read.", "operationId": "listUpdateRuns", "tags": ["assets"], "responses": { "200": { "description": "Run list" } } }
    },
    "/api/v2/updates/runs/{id}": {
      "get":    { "summary": "Get update run",    "description": "Scope: updates:read.",  "operationId": "getUpdateRun",    "tags": ["assets"], "responses": { "200": { "description": "Run" } } },
	  "delete": { "summary": "Delete update run record", "description": "Scope: updates:write.", "operationId": "deleteUpdateRun", "tags": ["assets"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/file-transfers": {
      "get":  { "summary": "List file transfers", "description": "Scope: files:read. Returns only transfers owned by the authenticated actor, newest-first. The data object contains transfers, total, limit, and offset.", "operationId": "listFileTransfers", "tags": ["files"], "parameters": [{ "name": "status", "in": "query", "required": false, "schema": { "type": "string", "enum": ["pending", "in_progress", "completed", "failed"] } }, { "name": "limit", "in": "query", "required": false, "schema": { "type": "integer", "minimum": 1, "maximum": 100, "default": 50 } }, { "name": "offset", "in": "query", "required": false, "schema": { "type": "integer", "minimum": 0, "maximum": 10000, "default": 0 } }], "responses": { "200": { "description": "Actor-scoped transfer page" }, "400": { "description": "Invalid filter or pagination" }, "403": { "description": "Missing files:read scope" }, "503": { "description": "File-transfer persistence unavailable" } } },
      "post": { "summary": "Start a file transfer",                     "description": "Scopes: files:read and files:write.", "operationId": "startFileTransfer", "tags": ["files"], "responses": { "202": { "description": "Accepted" } } }
    },
    "/api/v2/file-transfers/{id}": {
      "get":    { "summary": "Get file transfer status", "description": "Scope: files:read.",  "operationId": "getFileTransfer",    "tags": ["files"], "responses": { "200": { "description": "Transfer record" } } },
      "delete": { "summary": "Cancel file transfer",     "description": "Scope: files:write.", "operationId": "cancelFileTransfer", "tags": ["files"], "responses": { "200": { "description": "Cancelled" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/bulk/file-push": {
      "post": {
        "summary": "Bulk file push to multiple destinations",
        "description": "Pushes a file from one connection to N targets in parallel. Scope: files:write.",
        "operationId": "bulkFilePush",
        "tags": ["bulk", "files"],
        "responses": { "200": { "description": "Per-target results" } }
      }
    },
    "/api/v2/bulk/service-action": {
      "post": {
        "summary": "Bulk systemctl action across assets",
        "description": "Runs a systemctl action (start, stop, restart, etc.) on multiple assets. Scope: bulk:*.",
        "operationId": "bulkServiceAction",
        "tags": ["bulk"],
        "responses": { "200": { "description": "Per-target results" } }
      }
    },

    "/api/v2/alerts": {
      "get": {
        "summary": "List active alert instances",
        "description": "Scope: alerts:read.",
        "operationId": "listAlerts",
        "tags": ["alerts"],
        "responses": { "200": { "description": "Alert instance list" } }
      }
    },
    "/api/v2/alerts/{id}": {
      "get":    { "summary": "Get alert instance",        "description": "Scope: alerts:read.",  "operationId": "getAlert",    "tags": ["alerts"], "responses": { "200": { "description": "Alert" } } },
	  "delete": { "summary": "Delete alert instance", "description": "Scope: alerts:write.", "operationId": "deleteAlert", "tags": ["alerts"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/alerts/{id}/ack": {
	  "post": { "summary": "Acknowledge alert instance", "description": "Scope: alerts:write.", "operationId": "acknowledgeAlert", "tags": ["alerts"], "responses": { "200": { "description": "Acknowledged" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/alerts/{id}/resolve": {
	  "post": { "summary": "Resolve alert instance", "description": "Scope: alerts:write.", "operationId": "resolveAlert", "tags": ["alerts"], "responses": { "200": { "description": "Resolved" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
    "/api/v2/alerts/rules": {
      "get":  { "summary": "List alert rules",  "description": "Scope: alerts:read.",  "operationId": "listAlertRules",  "tags": ["alerts"], "responses": { "200": { "description": "Rule list" } } },
      "post": { "summary": "Create alert rule", "description": "Scope: alerts:write.", "operationId": "createAlertRule", "tags": ["alerts"], "responses": { "201": { "description": "Created" } } }
    },
	"/api/v2/alerts/rules/{id}": {
	  "get":    { "summary": "Get alert rule", "description": "Scope: alerts:read.", "operationId": "getAlertRule", "tags": ["alerts"], "responses": { "200": { "description": "Rule" } } },
	  "put":    { "summary": "Update alert rule", "description": "Scope: alerts:write.", "operationId": "updateAlertRule", "tags": ["alerts"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update alert rule", "description": "Scope: alerts:write.", "operationId": "patchAlertRule", "tags": ["alerts"], "responses": { "200": { "description": "Updated" } } },
	  "delete": { "summary": "Delete alert rule", "description": "Scope: alerts:write.", "operationId": "deleteAlertRule", "tags": ["alerts"], "responses": { "200": { "description": "Deleted" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/alerts/rules/{id}/test": {
	  "post": { "summary": "Evaluate alert rule on demand", "description": "Scope: alerts:write.", "operationId": "testAlertRule", "tags": ["alerts"], "responses": { "200": { "description": "Evaluation" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/alerts/rules/{id}/evaluations": {
	  "get": { "summary": "List alert rule evaluations", "description": "Scope: alerts:read.", "operationId": "listAlertRuleEvaluations", "tags": ["alerts"], "responses": { "200": { "description": "Evaluation list" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
    "/api/v2/incidents": {
      "get":  { "summary": "List incidents",  "description": "Scope: alerts:read.",  "operationId": "listIncidents",  "tags": ["incidents"], "responses": { "200": { "description": "Incident list" } } },
      "post": { "summary": "Create incident", "description": "Scope: alerts:write.", "operationId": "createIncident", "tags": ["incidents"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/incidents/{id}": {
      "get":    { "summary": "Get incident",    "description": "Scope: alerts:read.",  "operationId": "getIncident",    "tags": ["incidents"], "responses": { "200": { "description": "Incident" } } },
      "put":    { "summary": "Update incident", "description": "Scope: alerts:write.", "operationId": "updateIncident", "tags": ["incidents"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update incident", "description": "Scope: alerts:write.", "operationId": "patchIncident", "tags": ["incidents"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Close incident",  "description": "Scope: alerts:write.", "operationId": "deleteIncident", "tags": ["incidents"], "responses": { "200": { "description": "Closed" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/incidents/{id}/link-alert": {
	  "post": { "summary": "Link an alert to an incident", "description": "Scope: alerts:write.", "operationId": "linkIncidentAlert", "tags": ["incidents"], "responses": { "201": { "description": "Linked" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/alerts": {
	  "get": { "summary": "List incident alert links", "description": "Scope: alerts:read.", "operationId": "listIncidentAlerts", "tags": ["incidents"], "responses": { "200": { "description": "Alert links" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/timeline": {
	  "get": { "summary": "Get incident timeline", "description": "Scope: alerts:read.", "operationId": "getIncidentTimeline", "tags": ["incidents"], "responses": { "200": { "description": "Timeline" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/unlink-alert/{linkId}": {
	  "delete": { "summary": "Unlink an alert from an incident", "description": "Scope: alerts:write.", "operationId": "unlinkIncidentAlert", "tags": ["incidents"], "responses": { "200": { "description": "Unlinked" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "linkId", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/link-asset": {
	  "post": { "summary": "Link an asset to an incident", "description": "Scope: alerts:write.", "operationId": "linkIncidentAsset", "tags": ["incidents"], "responses": { "201": { "description": "Linked" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/assets": {
	  "get": { "summary": "List incident asset links", "description": "Scope: alerts:read.", "operationId": "listIncidentAssets", "tags": ["incidents"], "responses": { "200": { "description": "Asset links" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/unlink-asset/{linkId}": {
	  "delete": { "summary": "Unlink an asset from an incident", "description": "Scope: alerts:write.", "operationId": "unlinkIncidentAsset", "tags": ["incidents"], "responses": { "200": { "description": "Unlinked" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "linkId", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/incidents/{id}/export": {
	  "get": { "summary": "Export incident postmortem", "description": "Scope: alerts:read. Returns Markdown.", "operationId": "exportIncident", "tags": ["incidents"], "responses": { "200": { "description": "Markdown postmortem" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/connectors": {
      "get": {
        "summary": "List connectors",
        "description": "Returns all configured external connectors (Proxmox, TrueNAS, PBS, Portainer, etc.). Scope: connectors:read.",
        "operationId": "listConnectors",
        "tags": ["connectors"],
        "responses": { "200": { "description": "Connector list" } }
      }
    },
	"/api/v2/connectors/{id}/test": {
	  "post": { "summary": "Test connector", "description": "Scope: connectors:write.", "operationId": "testConnector", "tags": ["connectors"], "responses": { "200": { "description": "Test result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/connectors/{id}/discover": {
	  "get": { "summary": "Discover connector assets", "description": "Scope: connectors:read.", "operationId": "discoverConnectorAssets", "tags": ["connectors"], "responses": { "200": { "description": "Discovered assets" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/connectors/{id}/health": {
	  "get": { "summary": "Get connector health", "description": "Scope: connectors:read.", "operationId": "getConnectorHealth", "tags": ["connectors"], "responses": { "200": { "description": "Health" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/connectors/{id}/actions": {
	  "get": { "summary": "List connector actions", "description": "Scope: connectors:read.", "operationId": "listConnectorActions", "tags": ["connectors"], "responses": { "200": { "description": "Action descriptors" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/connectors/{id}/actions/{actionId}/execute": {
	  "post": { "summary": "Execute connector action", "description": "Scope: connectors:write.", "operationId": "executeConnectorAction", "tags": ["connectors"], "responses": { "200": { "description": "Action result" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "actionId", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/proxmox/cluster/status": {
      "get": { "summary": "Proxmox cluster status",    "description": "Scope: connectors:read.", "operationId": "getProxmoxClusterStatus",    "tags": ["proxmox"], "responses": { "200": { "description": "Cluster status" } } }
    },
    "/api/v2/proxmox/cluster/resources": {
      "get": { "summary": "Proxmox cluster resources",  "description": "Scope: connectors:read.", "operationId": "getProxmoxClusterResources", "tags": ["proxmox"], "responses": { "200": { "description": "Resource list" } } }
    },
    "/api/v2/proxmox/assets/{id}": {
      "get": { "summary": "Proxmox VM/container details", "description": "Scope: connectors:read (GET), connectors:write (POST).", "operationId": "proxmoxAssetActions", "tags": ["proxmox"], "responses": { "200": { "description": "Asset details" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/proxmox/nodes/{id}": {
      "get": { "summary": "Proxmox node details", "description": "Scope: connectors:read.", "operationId": "proxmoxNodeRoutes", "tags": ["proxmox"], "responses": { "200": { "description": "Node details" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/proxmox/ceph/status": {
      "get": { "summary": "Proxmox Ceph cluster status", "description": "Scope: connectors:read.", "operationId": "getProxmoxCephStatus", "tags": ["proxmox"], "responses": { "200": { "description": "Ceph status" } } }
    },
    "/api/v2/proxmox/tasks/{id}": {
      "get": { "summary": "Proxmox task status", "description": "Scope: connectors:read.", "operationId": "proxmoxTaskRoutes", "tags": ["proxmox"], "responses": { "200": { "description": "Task status" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/truenas/assets/{id}": {
      "get": { "summary": "TrueNAS asset details", "description": "Scope: connectors:read (GET), connectors:write (POST).", "operationId": "truenasAssetActions", "tags": ["truenas"], "responses": { "200": { "description": "Asset details" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/pbs/assets/{id}": {
      "get": { "summary": "PBS datastore/asset details", "description": "Scope: connectors:read (GET), connectors:write (POST).", "operationId": "pbsAssetActions", "tags": ["pbs"], "responses": { "200": { "description": "Asset details" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/pbs/tasks/{id}": {
      "get": { "summary": "PBS task status", "description": "Scope: connectors:read.", "operationId": "pbsTaskRoutes", "tags": ["pbs"], "responses": { "200": { "description": "Task status" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/portainer/assets/{id}": {
      "get": { "summary": "Portainer environment details", "description": "Scope: connectors:read.", "operationId": "portainerAssetActions", "tags": ["portainer"], "responses": { "200": { "description": "Asset details" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/homeassistant/entities": {
      "get":  { "summary": "List Home Assistant entities",   "description": "Scope: homeassistant:read.",  "operationId": "listHAEntities",   "tags": ["homeassistant"], "responses": { "200": { "description": "Entity list" } } },
      "post": { "summary": "Control a Home Assistant entity", "description": "Scope: homeassistant:write.", "operationId": "controlHAEntity", "tags": ["homeassistant"], "responses": { "200": { "description": "Action result" } } }
    },
    "/api/v2/homeassistant/entities/{id}": {
      "get": { "summary": "Get Home Assistant entity", "description": "Requires homeassistant:read scope.", "operationId": "haEntityActions", "tags": ["homeassistant"], "responses": { "200": { "description": "Entity" } } },
      "post": { "summary": "Control a Home Assistant entity", "description": "Requires admin role and homeassistant:write scope.", "operationId": "controlHAEntityById", "tags": ["homeassistant"], "responses": { "200": { "description": "Action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/homeassistant/automations": {
      "get": { "summary": "List Home Assistant automations", "description": "Scope: homeassistant:read.", "operationId": "listHAAutomations", "tags": ["homeassistant"], "responses": { "200": { "description": "Automation list" } } }
    },
    "/api/v2/homeassistant/scenes": {
      "get": { "summary": "List Home Assistant scenes", "description": "Scope: homeassistant:read.", "operationId": "listHAScenes", "tags": ["homeassistant"], "responses": { "200": { "description": "Scene list" } } }
    },

    "/api/v2/credentials/profiles": {
      "get":  { "summary": "List credential profiles",  "description": "Scope: credentials:read.",  "operationId": "listCredentialProfiles",  "tags": ["credentials"], "responses": { "200": { "description": "Profile list" } } },
      "post": { "summary": "Create credential profile", "description": "Scope: credentials:write.", "operationId": "createCredentialProfile", "tags": ["credentials"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/credentials/profiles/{id}": {
      "get":    { "summary": "Get credential profile",    "description": "Scope: credentials:read.",  "operationId": "getCredentialProfile",    "tags": ["credentials"], "responses": { "200": { "description": "Profile" } } },
      "delete": { "summary": "Delete credential profile", "description": "Scope: credentials:write.", "operationId": "deleteCredentialProfile", "tags": ["credentials"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/credentials/profiles/{id}/rotate": {
	  "post": { "summary": "Rotate credential profile secret", "description": "Scope: credentials:write.", "operationId": "rotateCredentialProfile", "tags": ["credentials"], "responses": { "200": { "description": "Rotated" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/terminal/sessions": {
      "get":  { "summary": "List terminal sessions",  "description": "Scope: terminal:read.",  "operationId": "listTerminalSessions",  "tags": ["terminal"], "responses": { "200": { "description": "Session list" } } },
      "post": { "summary": "Create terminal session", "description": "Scope: terminal:write.", "operationId": "createTerminalSession", "tags": ["terminal"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/terminal/history": {
      "get": { "summary": "Recent command history", "description": "Scope: terminal:read.", "operationId": "getTerminalHistory", "tags": ["terminal"], "responses": { "200": { "description": "Command history" } } }
    },
    "/api/v2/terminal/history/{id}": {
      "delete": { "summary": "Delete a completed command-history record", "description": "Scope: terminal:write. The authenticated actor may delete only their own records unless they are the owner; active commands return 409.", "operationId": "deleteTerminalHistoryRecord", "tags": ["terminal"], "responses": { "200": { "description": "Deleted" }, "404": { "description": "Command not found" }, "409": { "description": "Command is still active" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/terminal/snippets": {
      "get":  { "summary": "List terminal snippets",  "description": "Scope: terminal:read.",  "operationId": "listTerminalSnippets",  "tags": ["terminal"], "responses": { "200": { "description": "Snippet list" } } },
      "post": { "summary": "Create terminal snippet", "description": "Scope: terminal:write.", "operationId": "createTerminalSnippet", "tags": ["terminal"], "responses": { "201": { "description": "Created" } } }
    },
	"/api/v2/terminal/snippets/{id}": {
	  "get":    { "summary": "Get terminal snippet", "description": "Scope: terminal:read.", "operationId": "getTerminalSnippet", "tags": ["terminal"], "responses": { "200": { "description": "Snippet" } } },
	  "put":    { "summary": "Update terminal snippet", "description": "Scope: terminal:write.", "operationId": "updateTerminalSnippet", "tags": ["terminal"], "responses": { "200": { "description": "Updated" } } },
	  "delete": { "summary": "Delete terminal snippet", "description": "Scope: terminal:write.", "operationId": "deleteTerminalSnippet", "tags": ["terminal"], "responses": { "200": { "description": "Deleted" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/agents": {
      "get": { "summary": "List connected agents", "description": "Scope: agents:read.", "operationId": "listAgents", "tags": ["agents"], "responses": { "200": { "description": "Agent list" } } }
    },
	"/api/v2/agents/{id}/settings": {
	  "get":   { "summary": "Get effective agent settings", "description": "Scope: agents:read.", "operationId": "getAgentSettings", "tags": ["agents"], "responses": { "200": { "description": "Settings" } } },
	  "patch": { "summary": "Update agent settings", "description": "Scope: agents:write.", "operationId": "patchAgentSettings", "tags": ["agents"], "responses": { "200": { "description": "Updated" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/agents/{id}/settings/reset": {
	  "post": { "summary": "Reset agent settings", "description": "Scope: agents:write.", "operationId": "resetAgentSettings", "tags": ["agents"], "responses": { "200": { "description": "Reset" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/agents/{id}/settings/test-docker": {
	  "post": { "summary": "Test agent Docker settings", "description": "Scope: agents:write.", "operationId": "testAgentDockerSettings", "tags": ["agents"], "responses": { "200": { "description": "Test result" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/agents/{id}/settings/history": {
	  "get": { "summary": "Get agent settings history", "description": "Scope: agents:read.", "operationId": "getAgentSettingsHistory", "tags": ["agents"], "responses": { "200": { "description": "History" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/agents/{id}/settings/update-agent": {
	  "post": { "summary": "Request agent binary update", "description": "Scope: agents:write.", "operationId": "updateAgentBinary", "tags": ["agents"], "responses": { "202": { "description": "Update requested" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
    "/api/v2/agents/pending": {
      "get": { "summary": "List pending (unapproved) agents", "description": "Scope: agents:read.", "operationId": "listPendingAgents", "tags": ["agents"], "responses": { "200": { "description": "Pending agent list" } } }
    },
    "/api/v2/agents/pending/approve": {
      "post": { "summary": "Approve a pending agent", "description": "Scope: agents:write.", "operationId": "approvePendingAgent", "tags": ["agents"], "responses": { "200": { "description": "Approved" } } }
    },
    "/api/v2/agents/pending/reject": {
      "post": { "summary": "Reject a pending agent", "description": "Scope: agents:write.", "operationId": "rejectPendingAgent", "tags": ["agents"], "responses": { "200": { "description": "Rejected" } } }
    },

    "/api/v2/hub/status": {
      "get": { "summary": "Hub runtime status", "description": "Returns hub status and connected agent count. Scope: hub:read.", "operationId": "getHubStatus", "tags": ["hub"], "responses": { "200": { "description": "Status" } } }
    },
    "/api/v2/hub/agents": {
      "get": { "summary": "Agent presence details", "description": "Scope: hub:read.", "operationId": "getHubAgents", "tags": ["hub"], "responses": { "200": { "description": "Agents" } } }
    },
    "/api/v2/hub/tls": {
      "get":    { "summary": "Get TLS settings",        "description": "Scope: hub:read.",  "operationId": "getHubTLS",    "tags": ["hub"], "responses": { "200": { "description": "TLS status" } } },
      "post":   { "summary": "Upload TLS cert/key",     "description": "Scope: hub:admin.", "operationId": "uploadHubTLS", "tags": ["hub"], "responses": { "200": { "description": "Applied" } } },
      "delete": { "summary": "Clear uploaded TLS cert", "description": "Scope: hub:admin.", "operationId": "clearHubTLS",  "tags": ["hub"], "responses": { "200": { "description": "Cleared" } } }
    },
    "/api/v2/hub/tls/renew": {
      "post": {
        "summary": "Renew the active TLS certificate",
        "description": "Renews the built-in or Tailscale TLS certificate. Returns 422 for uploaded/external certs. Scope: settings:write.",
        "operationId": "renewHubTLS",
        "tags": ["hub"],
        "responses": {
          "200": { "description": "Certificate renewed" },
          "422": { "description": "Renewal not supported for this TLS source" },
          "500": { "description": "Renewal failed" }
        }
      }
    },
    "/api/v2/hub/tailscale": {
	  "get":  { "summary": "Tailscale serve state", "description": "Scope: hub:read.", "operationId": "getHubTailscale", "tags": ["hub"], "responses": { "200": { "description": "State" } } },
	  "post": { "summary": "Update Tailscale serve state", "description": "Scope: hub:admin.", "operationId": "updateHubTailscale", "tags": ["hub"], "responses": { "200": { "description": "Updated" } } }
    },

    "/api/v2/web-services": {
      "get":  { "summary": "List discovered web services", "description": "Scope: web-services:read.",  "operationId": "listWebServices", "tags": ["web-services"], "responses": { "200": { "description": "Service list" } } },
      "post": { "summary": "Add web service manually",     "description": "Scope: web-services:write.", "operationId": "createWebService", "tags": ["web-services"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/web-services/sync": {
      "post": { "summary": "Trigger web service re-sync", "description": "Scope: web-services:write.", "operationId": "syncWebServices", "tags": ["web-services"], "responses": { "200": { "description": "Sync started" } } }
    },
	"/api/v2/web-services/{id}": {
	  "put":    { "summary": "Update manual web service", "description": "Scope: web-services:write.", "operationId": "updateManualWebService", "tags": ["web-services"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update manual web service", "description": "Scope: web-services:write.", "operationId": "patchManualWebService", "tags": ["web-services"], "responses": { "200": { "description": "Updated" } } },
	  "delete": { "summary": "Delete manual web service", "description": "Scope: web-services:write.", "operationId": "deleteManualWebService", "tags": ["web-services"], "responses": { "204": { "description": "Deleted" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/collectors": {
      "get":  { "summary": "List hub collectors",  "description": "Scope: collectors:read.",  "operationId": "listCollectors",  "tags": ["collectors"], "responses": { "200": { "description": "Collector list" } } },
      "post": { "summary": "Create hub collector", "description": "Scope: collectors:write.", "operationId": "createCollector", "tags": ["collectors"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/collectors/{id}": {
      "get":    { "summary": "Get collector",    "description": "Scope: collectors:read.",  "operationId": "getCollector",    "tags": ["collectors"], "responses": { "200": { "description": "Collector" } } },
      "put":    { "summary": "Update collector", "description": "Scope: collectors:write.", "operationId": "updateCollector", "tags": ["collectors"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update collector", "description": "Scope: collectors:write.", "operationId": "patchCollector", "tags": ["collectors"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete collector", "description": "Scope: collectors:write.", "operationId": "deleteCollector", "tags": ["collectors"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/collectors/{id}/run": {
	  "post": { "summary": "Run collector now", "description": "Scope: collectors:write.", "operationId": "runCollector", "tags": ["collectors"], "responses": { "202": { "description": "Started" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/notifications/channels": {
	  "get":  { "summary": "List notification channels", "description": "Scope: notifications:read.", "operationId": "listNotificationChannels", "tags": ["notifications"], "responses": { "200": { "description": "Channel list" } } },
	  "post": { "summary": "Create notification channel", "description": "Scope: notifications:write.", "operationId": "createNotificationChannel", "tags": ["notifications"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/notifications/history": {
      "get": { "summary": "Notification delivery history", "description": "Scope: notifications:read.", "operationId": "getNotificationHistory", "tags": ["notifications"], "responses": { "200": { "description": "History" } } }
    },

    "/api/v2/synthetic-checks": {
      "get":  { "summary": "List synthetic checks",  "description": "Scope: assets:read.",  "operationId": "listSyntheticChecks",  "tags": ["synthetic"], "responses": { "200": { "description": "Check list" } } },
      "post": { "summary": "Create synthetic check", "description": "Scope: assets:write.", "operationId": "createSyntheticCheck", "tags": ["synthetic"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/synthetic-checks/{id}": {
      "get":    { "summary": "Get synthetic check",    "description": "Scope: assets:read.",  "operationId": "getSyntheticCheck",    "tags": ["synthetic"], "responses": { "200": { "description": "Check" } } },
      "put":    { "summary": "Update synthetic check", "description": "Scope: assets:write.", "operationId": "updateSyntheticCheck", "tags": ["synthetic"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update synthetic check", "description": "Scope: assets:write.", "operationId": "patchSyntheticCheck", "tags": ["synthetic"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete synthetic check", "description": "Scope: assets:write.", "operationId": "deleteSyntheticCheck", "tags": ["synthetic"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/discovery/run": {
      "post": { "summary": "Trigger a network discovery scan", "description": "Scope: discovery:write.", "operationId": "runDiscovery", "tags": ["discovery"], "responses": { "200": { "description": "Scan initiated" } } }
    },
    "/api/v2/discovery/proposals": {
      "get": { "summary": "List discovery proposals", "description": "Unaccepted assets found by discovery. Scope: discovery:read.", "operationId": "listDiscoveryProposals", "tags": ["discovery"], "responses": { "200": { "description": "Proposal list" } } }
    },
	"/api/v2/discovery/proposals/{id}/accept": {
	  "post": { "summary": "Accept discovery proposal", "description": "Scope: discovery:write.", "operationId": "acceptDiscoveryProposal", "tags": ["discovery"], "responses": { "200": { "description": "Accepted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/discovery/proposals/{id}/dismiss": {
	  "post": { "summary": "Dismiss discovery proposal", "description": "Scope: discovery:write.", "operationId": "dismissDiscoveryProposal", "tags": ["discovery"], "responses": { "200": { "description": "Dismissed" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/dependencies": {
      "get":  { "summary": "List service dependencies",  "description": "Scope: topology:read.",  "operationId": "listDependencies",  "tags": ["dependencies"], "responses": { "200": { "description": "Dependency list" } } },
      "post": { "summary": "Create service dependency",  "description": "Scope: topology:write.", "operationId": "createDependency", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/dependencies/{id}": {
      "get":    { "summary": "Get dependency",    "description": "Scope: topology:read.",  "operationId": "getDependency",    "tags": ["dependencies"], "responses": { "200": { "description": "Dependency" } } },
      "delete": { "summary": "Delete dependency", "description": "Scope: topology:write.", "operationId": "deleteDependency", "tags": ["dependencies"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/dependencies/batch": {
	  "get": { "summary": "List dependencies for multiple assets", "description": "Scope: topology:read. Every requested asset must be allowed by the authenticated principal.", "operationId": "listDependenciesBatch", "tags": ["dependencies"], "responses": { "200": { "description": "Dependency list" } } }
	},
	"/api/v2/dependencies/graph": {
	  "get": { "summary": "Get dependency graph around an asset", "description": "Scope: topology:read.", "operationId": "getDependencyGraph", "tags": ["dependencies"], "responses": { "200": { "description": "Dependency graph" } } }
	},
    "/api/v2/edges": {
      "get":  { "summary": "List topology edges",  "description": "Scope: topology:read.",  "operationId": "listEdges",  "tags": ["dependencies"], "responses": { "200": { "description": "Edge list" } } },
      "post": { "summary": "Create topology edge",  "description": "Scope: topology:write.", "operationId": "createEdge", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
	"/api/v2/edges/{id}": {
	  "get":    { "summary": "Get topology edge", "description": "Scope: topology:read.", "operationId": "getEdge", "tags": ["dependencies"], "responses": { "200": { "description": "Edge" } } },
	  "patch":  { "summary": "Update topology edge", "description": "Scope: topology:write.", "operationId": "updateEdge", "tags": ["dependencies"], "responses": { "200": { "description": "Updated" } } },
	  "delete": { "summary": "Delete topology edge", "description": "Scope: topology:write.", "operationId": "deleteEdge", "tags": ["dependencies"], "responses": { "200": { "description": "Deleted" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/edges/tree": {
	  "get": { "summary": "Get descendant edge tree", "description": "Scope: topology:read.", "operationId": "getEdgeTree", "tags": ["dependencies"], "responses": { "200": { "description": "Descendant tree" } } }
	},
	"/api/v2/edges/ancestors": {
	  "get": { "summary": "Get edge ancestor chain", "description": "Scope: topology:read.", "operationId": "getEdgeAncestors", "tags": ["dependencies"], "responses": { "200": { "description": "Ancestor chain" } } }
	},
    "/api/v2/composites": {
      "post": { "summary": "Create composite service",  "description": "Scope: topology:write.", "operationId": "createComposite", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/composites/{id}": {
      "get":    { "summary": "Get composite",    "description": "Scope: topology:read.",  "operationId": "getComposite",    "tags": ["dependencies"], "responses": { "200": { "description": "Composite" } } },
	  "patch":  { "summary": "Change composite primary asset", "description": "Scope: topology:write.", "operationId": "updateComposite", "tags": ["dependencies"], "responses": { "200": { "description": "Updated" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/composites/{id}/members/{assetId}": {
	  "delete": { "summary": "Detach a composite member", "description": "Scope: topology:write.", "operationId": "detachCompositeMember", "tags": ["dependencies"], "responses": { "200": { "description": "Detached" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "assetId", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/topology": {
      "get": { "summary": "Full topology canvas state", "description": "Returns zones, members, connections, and viewport. Scope: topology:read.", "operationId": "getTopology", "tags": ["topology"], "responses": { "200": { "description": "Topology layout" } } }
    },
    "/api/v2/topology/zones": {
      "post": { "summary": "Create topology zone", "description": "Scope: topology:write.", "operationId": "createTopologyZone", "tags": ["topology"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/topology/zones/{id}": {
      "put":    { "summary": "Update zone", "description": "Scope: topology:write.", "operationId": "updateTopologyZone", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete zone", "description": "Scope: topology:write.", "operationId": "deleteTopologyZone", "tags": ["topology"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/topology/zones/{id}/members": {
	  "put": { "summary": "Replace topology zone membership", "description": "Scope: topology:write.", "operationId": "setTopologyZoneMembers", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/topology/zones/reorder": {
	  "put": { "summary": "Reorder topology zones", "description": "Scope: topology:write.", "operationId": "reorderTopologyZones", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } }
	},
    "/api/v2/topology/connections": {
      "post": { "summary": "Create topology connection", "description": "Scope: topology:write.", "operationId": "createTopologyConnection", "tags": ["topology"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/topology/connections/{id}": {
      "put":    { "summary": "Update connection", "description": "Scope: topology:write.", "operationId": "updateTopologyConnection", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete connection", "description": "Scope: topology:write.", "operationId": "deleteTopologyConnection", "tags": ["topology"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/topology/viewport": {
      "put": { "summary": "Save canvas viewport",  "description": "Scope: topology:write.", "operationId": "saveTopologyViewport", "tags": ["topology"], "responses": { "200": { "description": "Saved" } } }
    },
    "/api/v2/topology/unsorted": {
      "get": { "summary": "List unsorted (unplaced) assets", "description": "Scope: topology:read.", "operationId": "getTopologyUnsorted", "tags": ["topology"], "responses": { "200": { "description": "Unsorted asset list" } } }
    },
    "/api/v2/topology/auto-place": {
      "post": { "summary": "Auto-place unsorted assets into zones", "description": "Scope: topology:write.", "operationId": "topologyAutoPlace", "tags": ["topology"], "responses": { "200": { "description": "Placement results" } } }
    },
    "/api/v2/topology/reset": {
      "post": { "summary": "Reset topology layout to defaults", "description": "Scope: topology:write.", "operationId": "topologyReset", "tags": ["topology"], "responses": { "200": { "description": "Layout reset" } } }
    },
    "/api/v2/topology/dismiss": {
      "post": { "summary": "Dismiss an unsorted asset from topology", "description": "Scope: topology:write.", "operationId": "topologyDismiss", "tags": ["topology"], "responses": { "200": { "description": "Dismissed" } } }
    },
    "/api/v2/topology/dismiss/{id}": {
      "delete": { "summary": "Un-dismiss a previously dismissed asset", "description": "Scope: topology:write.", "operationId": "topologyUndismiss", "tags": ["topology"], "responses": { "200": { "description": "Un-dismissed" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/failover-pairs": {
      "get":  { "summary": "List failover pairs",  "description": "Scope: failover:read.",  "operationId": "listFailoverPairs",  "tags": ["failover"], "responses": { "200": { "description": "Pair list" } } },
      "post": { "summary": "Create failover pair", "description": "Scope: failover:write.", "operationId": "createFailoverPair", "tags": ["failover"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/failover-pairs/{id}": {
      "get":    { "summary": "Get failover pair",    "description": "Scope: failover:read.",  "operationId": "getFailoverPair",    "tags": ["failover"], "responses": { "200": { "description": "Pair" } } },
      "put":    { "summary": "Update failover pair", "description": "Scope: failover:write.", "operationId": "updateFailoverPair", "tags": ["failover"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update failover pair", "description": "Scope: failover:write.", "operationId": "patchFailoverPair", "tags": ["failover"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete failover pair", "description": "Scope: failover:write.", "operationId": "deleteFailoverPair", "tags": ["failover"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/dead-letters": {
      "get": { "summary": "List dead-letter queue entries", "description": "Scope: dead-letters:read.", "operationId": "listDeadLetters", "tags": ["dead-letters"], "responses": { "200": { "description": "Dead letter list" } } }
    },

    "/api/v2/audit/events": {
      "get": { "summary": "Query audit event log", "description": "Scope: audit:read. Results are newest-first.", "operationId": "listAuditEvents", "tags": ["audit"], "parameters": [{ "name": "limit", "in": "query", "required": false, "schema": { "type": "integer", "minimum": 1, "maximum": 1000, "default": 100 } }, { "name": "offset", "in": "query", "required": false, "schema": { "type": "integer", "minimum": 0, "default": 0 } }], "responses": { "200": { "description": "Audit events" } } }
    },

    "/api/v2/logs/views": {
      "get":  { "summary": "List saved log views",  "description": "Scope: logs:read.",  "operationId": "listLogViews",  "tags": ["logs"], "responses": { "200": { "description": "View list" } } },
      "post": { "summary": "Create saved log view", "description": "Scope: logs:write.", "operationId": "createLogView", "tags": ["logs"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/logs/views/{id}": {
      "get":    { "summary": "Get log view",    "description": "Scope: logs:read.",  "operationId": "getLogView",    "tags": ["logs"], "responses": { "200": { "description": "View" } } },
      "put":    { "summary": "Update log view", "description": "Scope: logs:write.", "operationId": "updateLogView", "tags": ["logs"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update log view", "description": "Scope: logs:write.", "operationId": "patchLogView", "tags": ["logs"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete log view", "description": "Scope: logs:write.", "operationId": "deleteLogView", "tags": ["logs"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/settings/prometheus": {
      "get": { "summary": "Get Prometheus settings",    "description": "Scope: settings:read.",  "operationId": "getPrometheusSettings",    "tags": ["settings"], "responses": { "200": { "description": "Settings" } } },
	  "patch": { "summary": "Update Prometheus settings", "description": "Scope: settings:write.", "operationId": "updatePrometheusSettings", "tags": ["settings"], "responses": { "200": { "description": "Updated" } } }
    },
    "/api/v2/settings/prometheus/test": {
      "post": {
        "summary": "Test Prometheus remote_write connection",
        "description": "Scope: settings:write. Rate-limited.",
        "operationId": "testPrometheusConnection",
        "tags": ["settings"],
        "responses": {
          "200": { "description": "Test result" },
          "400": { "description": "Bad request" }
        }
      }
    },

    "/api/v2/keys": {
      "get":  { "summary": "List API keys",  "description": "Admin-only. Requires admin authentication.", "operationId": "listAPIKeys",  "tags": ["keys"], "responses": { "200": { "description": "Key list" } } },
      "post": { "summary": "Create API key", "description": "Admin-only. Requires admin authentication.", "operationId": "createAPIKey", "tags": ["keys"], "responses": { "201": { "description": "Created (includes secret — shown only once)" } } }
    },
    "/api/v2/keys/{id}": {
      "get":    { "summary": "Get API key metadata",  "description": "Admin-only.", "operationId": "getAPIKey",    "tags": ["keys"], "responses": { "200": { "description": "Key metadata" } } },
	  "patch":  { "summary": "Update API key metadata and access", "description": "Admin-only.", "operationId": "updateAPIKey", "tags": ["keys"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Revoke API key",         "description": "Admin-only.", "operationId": "deleteAPIKey", "tags": ["keys"], "responses": { "200": { "description": "Revoked" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/webhooks": {
      "get":  { "summary": "List webhook configurations",  "description": "Scope: webhooks:read.",  "operationId": "listWebhooks",  "tags": ["webhooks"], "responses": { "200": { "description": "Webhook list" } } },
      "post": { "summary": "Create webhook configuration", "description": "Scope: webhooks:write.", "operationId": "createWebhook", "tags": ["webhooks"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/webhooks/{id}": {
      "get":    { "summary": "Get webhook",    "description": "Scope: webhooks:read.",  "operationId": "getWebhook",    "tags": ["webhooks"], "responses": { "200": { "description": "Webhook" } } },
      "put":    { "summary": "Update webhook", "description": "Scope: webhooks:write.", "operationId": "updateWebhook", "tags": ["webhooks"], "responses": { "200": { "description": "Updated" } } },
      "patch":  { "summary": "Patch webhook",  "description": "Scope: webhooks:write.", "operationId": "patchWebhook",  "tags": ["webhooks"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete webhook", "description": "Scope: webhooks:write.", "operationId": "deleteWebhook", "tags": ["webhooks"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/events/stream": {
      "get": {
        "summary": "Real-time event stream (WebSocket upgrade)",
        "description": "Upgrades to WebSocket for real-time hub events. Scope: events:subscribe.",
        "operationId": "eventStream",
        "tags": ["events"],
        "responses": {
          "101": { "description": "Switching protocols (WebSocket)" },
          "200": { "description": "SSE fallback stream" }
        }
      }
    }

  }
}`
