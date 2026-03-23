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
    { "name": "metrics",        "description": "Telemetry and metric time-series" },
    { "name": "exec",           "description": "Remote command execution" },
    { "name": "actions",        "description": "Saved actions (reusable command templates)" },
    { "name": "schedules",      "description": "Scheduled task execution" },
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
      "delete": { "summary": "Delete asset", "description": "Scope: assets:write.", "operationId": "deleteAsset", "tags": ["assets"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
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
      "get":  { "summary": "List saved actions",  "description": "Scope: actions:read.",  "operationId": "listSavedActions",  "tags": ["actions"], "responses": { "200": { "description": "Action list" } } },
      "post": { "summary": "Create saved action", "description": "Scope: actions:write.", "operationId": "createSavedAction", "tags": ["actions"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/actions/{id}": {
      "get":    { "summary": "Get saved action",     "description": "Scope: actions:read.",  "operationId": "getSavedAction",    "tags": ["actions"], "responses": { "200": { "description": "Action" } } },
      "delete": { "summary": "Delete saved action",  "description": "Scope: actions:write.", "operationId": "deleteSavedAction", "tags": ["actions"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/schedules": {
      "get":  { "summary": "List schedules",  "description": "Scope: schedules:read.",  "operationId": "listSchedules",  "tags": ["schedules"], "responses": { "200": { "description": "Schedule list" } } },
      "post": { "summary": "Create schedule", "description": "Scope: schedules:write.", "operationId": "createSchedule", "tags": ["schedules"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/schedules/{id}": {
      "get":    { "summary": "Get schedule",    "description": "Scope: schedules:read.",  "operationId": "getSchedule",    "tags": ["schedules"], "responses": { "200": { "description": "Schedule" } } },
      "put":    { "summary": "Update schedule", "description": "Scope: schedules:write.", "operationId": "updateSchedule", "tags": ["schedules"], "responses": { "200": { "description": "Updated" } } },
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
      "delete": { "summary": "Delete group", "description": "Scope: groups:write.", "operationId": "deleteGroup", "tags": ["groups"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/docker/hosts": {
      "get":  { "summary": "List Docker hosts",     "description": "Scope: docker:read.",  "operationId": "listDockerHosts",  "tags": ["docker"], "responses": { "200": { "description": "Host list" } } },
      "post": { "summary": "Register Docker host",  "description": "Scope: docker:write.", "operationId": "createDockerHost", "tags": ["docker"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/docker/hosts/{id}": {
      "get":    { "summary": "Get Docker host",    "description": "Scope: docker:read.",  "operationId": "getDockerHost",    "tags": ["docker"], "responses": { "200": { "description": "Host" } } },
      "put":    { "summary": "Update Docker host", "description": "Scope: docker:write.", "operationId": "updateDockerHost", "tags": ["docker"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Remove Docker host", "description": "Scope: docker:write.", "operationId": "deleteDockerHost", "tags": ["docker"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/docker/containers/{id}": {
      "get": { "summary": "Get or act on a Docker container", "description": "Scope: docker:read (GET), docker:write (POST/PUT/DELETE).", "operationId": "dockerContainerActions", "tags": ["docker"], "responses": { "200": { "description": "Container details or action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/docker/stacks/{id}": {
      "get": { "summary": "Get or act on a Docker stack", "description": "Scope: docker:read (GET), docker:write (POST/PUT/DELETE).", "operationId": "dockerStackActions", "tags": ["docker"], "responses": { "200": { "description": "Stack details or action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/updates/plans": {
      "get":  { "summary": "List update plans",  "description": "Scope: updates:read.",  "operationId": "listUpdatePlans",  "tags": ["assets"], "responses": { "200": { "description": "Plan list" } } },
      "post": { "summary": "Create update plan", "description": "Scope: updates:write.", "operationId": "createUpdatePlan", "tags": ["assets"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/updates/plans/{id}": {
      "get":    { "summary": "Get update plan",    "description": "Scope: updates:read.",  "operationId": "getUpdatePlan",    "tags": ["assets"], "responses": { "200": { "description": "Plan" } } },
      "put":    { "summary": "Update plan",        "description": "Scope: updates:write.", "operationId": "updateUpdatePlan", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete update plan", "description": "Scope: updates:write.", "operationId": "deleteUpdatePlan", "tags": ["assets"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/updates/runs": {
      "get": { "summary": "List update runs", "description": "Scope: updates:read.", "operationId": "listUpdateRuns", "tags": ["assets"], "responses": { "200": { "description": "Run list" } } }
    },
    "/api/v2/updates/runs/{id}": {
      "get":    { "summary": "Get update run",    "description": "Scope: updates:read.",  "operationId": "getUpdateRun",    "tags": ["assets"], "responses": { "200": { "description": "Run" } } },
      "put":    { "summary": "Update run",        "description": "Scope: updates:write.", "operationId": "updateUpdateRun", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Cancel update run", "description": "Scope: updates:write.", "operationId": "deleteUpdateRun", "tags": ["assets"], "responses": { "200": { "description": "Cancelled" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/file-transfers": {
      "get":  { "summary": "List file transfers (not yet implemented)", "description": "Scope: files:read.",  "operationId": "listFileTransfers",  "tags": ["files"], "responses": { "501": { "description": "Not implemented" } } },
      "post": { "summary": "Start a file transfer",                     "description": "Scope: files:write.", "operationId": "startFileTransfer", "tags": ["files"], "responses": { "202": { "description": "Accepted" } } }
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
      "put":    { "summary": "Acknowledge/update alert",  "description": "Scope: alerts:write.", "operationId": "updateAlert", "tags": ["alerts"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Resolve/dismiss alert",     "description": "Scope: alerts:write.", "operationId": "deleteAlert", "tags": ["alerts"], "responses": { "200": { "description": "Resolved" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/alerts/rules": {
      "get":  { "summary": "List alert rules",  "description": "Scope: alerts:read.",  "operationId": "listAlertRules",  "tags": ["alerts"], "responses": { "200": { "description": "Rule list" } } },
      "post": { "summary": "Create alert rule", "description": "Scope: alerts:write.", "operationId": "createAlertRule", "tags": ["alerts"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/incidents": {
      "get":  { "summary": "List incidents",  "description": "Scope: alerts:read.",  "operationId": "listIncidents",  "tags": ["incidents"], "responses": { "200": { "description": "Incident list" } } },
      "post": { "summary": "Create incident", "description": "Scope: alerts:write.", "operationId": "createIncident", "tags": ["incidents"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/incidents/{id}": {
      "get":    { "summary": "Get incident",    "description": "Scope: alerts:read.",  "operationId": "getIncident",    "tags": ["incidents"], "responses": { "200": { "description": "Incident" } } },
      "put":    { "summary": "Update incident", "description": "Scope: alerts:write.", "operationId": "updateIncident", "tags": ["incidents"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Close incident",  "description": "Scope: alerts:write.", "operationId": "deleteIncident", "tags": ["incidents"], "responses": { "200": { "description": "Closed" } } },
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
    "/api/v2/connectors/{id}": {
      "get":    { "summary": "Get connector",    "description": "Scope: connectors:read.",  "operationId": "getConnector",    "tags": ["connectors"], "responses": { "200": { "description": "Connector" } } },
      "put":    { "summary": "Update connector", "description": "Scope: connectors:write.", "operationId": "updateConnector", "tags": ["connectors"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete connector", "description": "Scope: connectors:write.", "operationId": "deleteConnector", "tags": ["connectors"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
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
      "get": { "summary": "Get Home Assistant entity", "description": "Scope: homeassistant:read (GET), homeassistant:write (POST/PUT/DELETE).", "operationId": "haEntityActions", "tags": ["homeassistant"], "responses": { "200": { "description": "Entity" } } },
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
      "put":    { "summary": "Update credential profile", "description": "Scope: credentials:write.", "operationId": "updateCredentialProfile", "tags": ["credentials"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete credential profile", "description": "Scope: credentials:write.", "operationId": "deleteCredentialProfile", "tags": ["credentials"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/terminal/sessions": {
      "get":  { "summary": "List terminal sessions",  "description": "Scope: terminal:read.",  "operationId": "listTerminalSessions",  "tags": ["terminal"], "responses": { "200": { "description": "Session list" } } },
      "post": { "summary": "Create terminal session", "description": "Scope: terminal:write.", "operationId": "createTerminalSession", "tags": ["terminal"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/terminal/history": {
      "get": { "summary": "Recent command history", "description": "Scope: terminal:read.", "operationId": "getTerminalHistory", "tags": ["terminal"], "responses": { "200": { "description": "Command history" } } }
    },
    "/api/v2/terminal/snippets": {
      "get":  { "summary": "List terminal snippets",  "description": "Scope: terminal:read.",  "operationId": "listTerminalSnippets",  "tags": ["terminal"], "responses": { "200": { "description": "Snippet list" } } },
      "post": { "summary": "Create terminal snippet", "description": "Scope: terminal:write.", "operationId": "createTerminalSnippet", "tags": ["terminal"], "responses": { "201": { "description": "Created" } } }
    },

    "/api/v2/agents": {
      "get": { "summary": "List connected agents", "description": "Scope: agents:read.", "operationId": "listAgents", "tags": ["agents"], "responses": { "200": { "description": "Agent list" } } }
    },
    "/api/v2/agents/{id}": {
      "get":    { "summary": "Get agent details",  "description": "Scope: agents:read.",  "operationId": "getAgent",    "tags": ["agents"], "responses": { "200": { "description": "Agent" } } },
      "put":    { "summary": "Update agent",       "description": "Scope: agents:write.", "operationId": "updateAgent", "tags": ["agents"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Remove agent",       "description": "Scope: agents:write.", "operationId": "deleteAgent", "tags": ["agents"], "responses": { "200": { "description": "Removed" } } },
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
      "get": { "summary": "Tailscale serve state", "description": "Scope: hub:read.", "operationId": "getHubTailscale", "tags": ["hub"], "responses": { "200": { "description": "State" } } }
    },

    "/api/v2/web-services": {
      "get":  { "summary": "List discovered web services", "description": "Scope: web-services:read.",  "operationId": "listWebServices", "tags": ["web-services"], "responses": { "200": { "description": "Service list" } } },
      "post": { "summary": "Add web service manually",     "description": "Scope: web-services:write.", "operationId": "createWebService", "tags": ["web-services"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/web-services/sync": {
      "post": { "summary": "Trigger web service re-sync", "description": "Scope: web-services:write.", "operationId": "syncWebServices", "tags": ["web-services"], "responses": { "200": { "description": "Sync started" } } }
    },

    "/api/v2/collectors": {
      "get":  { "summary": "List hub collectors",  "description": "Scope: collectors:read.",  "operationId": "listCollectors",  "tags": ["collectors"], "responses": { "200": { "description": "Collector list" } } },
      "post": { "summary": "Create hub collector", "description": "Scope: collectors:write.", "operationId": "createCollector", "tags": ["collectors"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/collectors/{id}": {
      "get":    { "summary": "Get collector",    "description": "Scope: collectors:read.",  "operationId": "getCollector",    "tags": ["collectors"], "responses": { "200": { "description": "Collector" } } },
      "put":    { "summary": "Update collector", "description": "Scope: collectors:write.", "operationId": "updateCollector", "tags": ["collectors"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete collector", "description": "Scope: collectors:write.", "operationId": "deleteCollector", "tags": ["collectors"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/notifications/channels": {
      "get": { "summary": "List notification channels", "description": "Scope: notifications:read.", "operationId": "listNotificationChannels", "tags": ["notifications"], "responses": { "200": { "description": "Channel list" } } }
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
      "delete": { "summary": "Delete synthetic check", "description": "Scope: assets:write.", "operationId": "deleteSyntheticCheck", "tags": ["synthetic"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/discovery/run": {
      "post": { "summary": "Trigger a network discovery scan", "description": "Scope: discovery:write.", "operationId": "runDiscovery", "tags": ["discovery"], "responses": { "200": { "description": "Scan initiated" } } }
    },
    "/api/v2/discovery/proposals": {
      "get": { "summary": "List discovery proposals", "description": "Unaccepted assets found by discovery. Scope: discovery:read.", "operationId": "listDiscoveryProposals", "tags": ["discovery"], "responses": { "200": { "description": "Proposal list" } } }
    },
    "/api/v2/discovery/proposals/{id}": {
      "get":  { "summary": "Get discovery proposal",             "description": "Scope: discovery:read.",  "operationId": "getDiscoveryProposal",    "tags": ["discovery"], "responses": { "200": { "description": "Proposal" } } },
      "post": { "summary": "Accept or reject discovery proposal", "description": "Scope: discovery:write.", "operationId": "actOnDiscoveryProposal", "tags": ["discovery"], "responses": { "200": { "description": "Action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/dependencies": {
      "get":  { "summary": "List service dependencies",  "description": "Scope: topology:read.",  "operationId": "listDependencies",  "tags": ["dependencies"], "responses": { "200": { "description": "Dependency list" } } },
      "post": { "summary": "Create service dependency",  "description": "Scope: topology:write.", "operationId": "createDependency", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/dependencies/{id}": {
      "get":    { "summary": "Get dependency",    "description": "Scope: topology:read.",  "operationId": "getDependency",    "tags": ["dependencies"], "responses": { "200": { "description": "Dependency" } } },
      "put":    { "summary": "Update dependency", "description": "Scope: topology:write.", "operationId": "updateDependency", "tags": ["dependencies"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete dependency", "description": "Scope: topology:write.", "operationId": "deleteDependency", "tags": ["dependencies"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/edges": {
      "get":  { "summary": "List topology edges",  "description": "Scope: topology:read.",  "operationId": "listEdges",  "tags": ["dependencies"], "responses": { "200": { "description": "Edge list" } } },
      "post": { "summary": "Create topology edge",  "description": "Scope: topology:write.", "operationId": "createEdge", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/composites": {
      "get":  { "summary": "List composite services",  "description": "Scope: topology:read.",  "operationId": "listComposites",  "tags": ["dependencies"], "responses": { "200": { "description": "Composite list" } } },
      "post": { "summary": "Create composite service",  "description": "Scope: topology:write.", "operationId": "createComposite", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/composites/{id}": {
      "get":    { "summary": "Get composite",    "description": "Scope: topology:read.",  "operationId": "getComposite",    "tags": ["dependencies"], "responses": { "200": { "description": "Composite" } } },
      "put":    { "summary": "Update composite", "description": "Scope: topology:write.", "operationId": "updateComposite", "tags": ["dependencies"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete composite", "description": "Scope: topology:write.", "operationId": "deleteComposite", "tags": ["dependencies"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/topology": {
      "get": { "summary": "Full topology canvas state", "description": "Returns zones, members, connections, and viewport. Scope: topology:read.", "operationId": "getTopology", "tags": ["topology"], "responses": { "200": { "description": "Topology layout" } } }
    },
    "/api/v2/topology/zones": {
      "get":  { "summary": "List topology zones",  "description": "Scope: topology:read.",  "operationId": "listTopologyZones",  "tags": ["topology"], "responses": { "200": { "description": "Zone list" } } },
      "post": { "summary": "Create topology zone", "description": "Scope: topology:write.", "operationId": "createTopologyZone", "tags": ["topology"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/topology/zones/{id}": {
      "get":    { "summary": "Get zone",    "description": "Scope: topology:read.",  "operationId": "getTopologyZone",    "tags": ["topology"], "responses": { "200": { "description": "Zone" } } },
      "put":    { "summary": "Update zone", "description": "Scope: topology:write.", "operationId": "updateTopologyZone", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete zone", "description": "Scope: topology:write.", "operationId": "deleteTopologyZone", "tags": ["topology"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/topology/connections": {
      "get":  { "summary": "List topology connections",  "description": "Scope: topology:read.",  "operationId": "listTopologyConnections",  "tags": ["topology"], "responses": { "200": { "description": "Connection list" } } },
      "post": { "summary": "Create topology connection", "description": "Scope: topology:write.", "operationId": "createTopologyConnection", "tags": ["topology"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/topology/connections/{id}": {
      "get":    { "summary": "Get connection",    "description": "Scope: topology:read.",  "operationId": "getTopologyConnection",    "tags": ["topology"], "responses": { "200": { "description": "Connection" } } },
      "put":    { "summary": "Update connection", "description": "Scope: topology:write.", "operationId": "updateTopologyConnection", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete connection", "description": "Scope: topology:write.", "operationId": "deleteTopologyConnection", "tags": ["topology"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/topology/viewport": {
      "get": { "summary": "Get canvas viewport",  "description": "Scope: topology:read.",  "operationId": "getTopologyViewport",  "tags": ["topology"], "responses": { "200": { "description": "Viewport state" } } },
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
      "delete": { "summary": "Delete failover pair", "description": "Scope: failover:write.", "operationId": "deleteFailoverPair", "tags": ["failover"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/dead-letters": {
      "get": { "summary": "List dead-letter queue entries", "description": "Scope: dead-letters:read.", "operationId": "listDeadLetters", "tags": ["dead-letters"], "responses": { "200": { "description": "Dead letter list" } } }
    },

    "/api/v2/audit/events": {
      "get": { "summary": "Query audit event log", "description": "Scope: audit:read.", "operationId": "listAuditEvents", "tags": ["audit"], "responses": { "200": { "description": "Audit events" } } }
    },

    "/api/v2/logs/views": {
      "get":  { "summary": "List saved log views",  "description": "Scope: logs:read.",  "operationId": "listLogViews",  "tags": ["logs"], "responses": { "200": { "description": "View list" } } },
      "post": { "summary": "Create saved log view", "description": "Scope: logs:write.", "operationId": "createLogView", "tags": ["logs"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/logs/views/{id}": {
      "get":    { "summary": "Get log view",    "description": "Scope: logs:read.",  "operationId": "getLogView",    "tags": ["logs"], "responses": { "200": { "description": "View" } } },
      "put":    { "summary": "Update log view", "description": "Scope: logs:write.", "operationId": "updateLogView", "tags": ["logs"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete log view", "description": "Scope: logs:write.", "operationId": "deleteLogView", "tags": ["logs"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

    "/api/v2/settings/prometheus": {
      "get": { "summary": "Get Prometheus settings",    "description": "Scope: settings:read.",  "operationId": "getPrometheusSettings",    "tags": ["settings"], "responses": { "200": { "description": "Settings" } } },
      "put": { "summary": "Update Prometheus settings", "description": "Scope: settings:write.", "operationId": "updatePrometheusSettings", "tags": ["settings"], "responses": { "200": { "description": "Updated" } } }
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
