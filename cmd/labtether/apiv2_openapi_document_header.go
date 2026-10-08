package main

// v2OpenAPIDocumentHeader documents this domain of the v2 API.
const v2OpenAPIDocumentHeader = `{
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

`
