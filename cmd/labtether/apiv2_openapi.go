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

// v2OpenAPISpec is a minimal static OpenAPI 3.0 document covering the v2 API
// surface. Kept as a Go string constant — no build tooling required.
// Extend this when new stable endpoints ship.
const v2OpenAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "LabTether Hub API v2",
    "version": "2.0.0",
    "description": "REST API for the LabTether homelab control plane. All endpoints except /api/v2/openapi.json require a Bearer API key or a session cookie."
  },
  "servers": [
    { "url": "/", "description": "This hub instance" }
  ],
  "components": {
    "securitySchemes": {
      "bearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "API key"
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
  "security": [{ "bearerAuth": [] }],
  "paths": {
    "/api/v2/openapi.json": {
      "get": {
        "summary": "OpenAPI specification",
        "description": "Returns this OpenAPI 3.0 document. No authentication required.",
        "security": [],
        "operationId": "getOpenAPI",
        "tags": ["meta"],
        "responses": {
          "200": { "description": "OpenAPI specification document" }
        }
      }
    },
    "/api/v2/assets": {
      "get":  { "summary": "List assets",   "operationId": "listAssets",   "tags": ["assets"], "responses": { "200": { "description": "Asset list" } } },
      "post": { "summary": "Create asset",  "operationId": "createAsset",  "tags": ["assets"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/assets/{id}": {
      "get":    { "summary": "Get asset",    "operationId": "getAsset",    "tags": ["assets"], "responses": { "200": { "description": "Asset" } } },
      "put":    { "summary": "Update asset", "operationId": "updateAsset", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete asset", "operationId": "deleteAsset", "tags": ["assets"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/metrics": {
      "get": {
        "summary": "Asset metric time-series",
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
      "get": { "summary": "Metrics overview (all assets)", "operationId": "getMetricsOverview", "tags": ["metrics"], "responses": { "200": { "description": "Per-asset snapshots" } } }
    },
    "/api/v2/metrics/query": {
      "get": {
        "summary": "Cross-asset metric query",
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
    "/api/v2/file-transfers": {
      "get":  { "summary": "List file transfers (not yet implemented)", "operationId": "listFileTransfers",  "tags": ["files"], "responses": { "501": { "description": "Not implemented" } } },
      "post": { "summary": "Start a file transfer",                     "operationId": "startFileTransfer", "tags": ["files"], "responses": { "202": { "description": "Accepted" } } }
    },
    "/api/v2/file-transfers/{id}": {
      "get":    { "summary": "Get file transfer status", "operationId": "getFileTransfer",    "tags": ["files"], "responses": { "200": { "description": "Transfer record" } } },
      "delete": { "summary": "Cancel file transfer",     "operationId": "cancelFileTransfer", "tags": ["files"], "responses": { "200": { "description": "Cancelled" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/bulk/file-push": {
      "post": {
        "summary": "Bulk file push to multiple destinations",
        "operationId": "bulkFilePush",
        "tags": ["bulk", "files"],
        "responses": { "200": { "description": "Per-target results" } }
      }
    },
    "/api/v2/bulk/service-action": {
      "post": { "summary": "Bulk systemctl action", "operationId": "bulkServiceAction", "tags": ["bulk"], "responses": { "200": { "description": "Per-target results" } } }
    },
    "/api/v2/settings/prometheus": {
      "get": { "summary": "Get Prometheus settings",    "operationId": "getPrometheusSettings",    "tags": ["settings"], "responses": { "200": { "description": "Settings" } } },
      "put": { "summary": "Update Prometheus settings", "operationId": "updatePrometheusSettings", "tags": ["settings"], "responses": { "200": { "description": "Updated" } } }
    },
    "/api/v2/settings/prometheus/test": {
      "post": {
        "summary": "Test Prometheus remote_write connection",
        "operationId": "testPrometheusConnection",
        "tags": ["settings"],
        "responses": {
          "200": { "description": "Test result" },
          "400": { "description": "Bad request" }
        }
      }
    },
    "/api/v2/hub/tls": {
      "get":    { "summary": "Get TLS settings",        "operationId": "getHubTLS",    "tags": ["hub"], "responses": { "200": { "description": "TLS status" } } },
      "post":   { "summary": "Upload TLS cert/key",     "operationId": "uploadHubTLS", "tags": ["hub"], "responses": { "200": { "description": "Applied" } } },
      "delete": { "summary": "Clear uploaded TLS cert", "operationId": "clearHubTLS",  "tags": ["hub"], "responses": { "200": { "description": "Cleared" } } }
    },
    "/api/v2/hub/tls/renew": {
      "post": {
        "summary": "Renew the active TLS certificate",
        "operationId": "renewHubTLS",
        "tags": ["hub"],
        "responses": {
          "200": { "description": "Certificate renewed" },
          "422": { "description": "Renewal not supported for this TLS source" },
          "500": { "description": "Renewal failed" }
        }
      }
    },
    "/api/v2/hub/status":    { "get": { "summary": "Hub status",           "operationId": "getHubStatus",    "tags": ["hub"],  "responses": { "200": { "description": "Status" } } } },
    "/api/v2/hub/agents":    { "get": { "summary": "Connected agents",      "operationId": "getHubAgents",    "tags": ["hub"],  "responses": { "200": { "description": "Agents" } } } },
    "/api/v2/hub/tailscale": { "get": { "summary": "Tailscale serve state", "operationId": "getHubTailscale", "tags": ["hub"],  "responses": { "200": { "description": "State" } } } },
    "/api/v2/keys":          { "get": { "summary": "List API keys",         "operationId": "listAPIKeys",     "tags": ["keys"], "responses": { "200": { "description": "Keys" } } },
                               "post": { "summary": "Create API key",       "operationId": "createAPIKey",   "tags": ["keys"], "responses": { "201": { "description": "Created" } } } },
    "/api/v2/whoami":        { "get": { "summary": "Current principal",     "operationId": "whoami",          "tags": ["auth"], "responses": { "200": { "description": "Principal" } } } },
    "/api/v2/search":        { "get": { "summary": "Cross-resource search", "operationId": "search",          "tags": ["meta"], "responses": { "200": { "description": "Results" } } } },
    "/api/v2/exec":          { "post": { "summary": "Execute command on asset", "operationId": "execMulti",  "tags": ["exec"], "responses": { "200": { "description": "Results" } } } }
  }
}`
