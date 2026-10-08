package main

// v2OpenAPIOperatorSettingsPaths documents this domain of the v2 API.
const v2OpenAPIOperatorSettingsPaths = `    "/api/v2/dead-letters": {
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
