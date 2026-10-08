package main

// v2OpenAPIAccessAdministrationPaths documents this domain of the v2 API.
const v2OpenAPIAccessAdministrationPaths = `    "/api/v2/credentials/profiles": {
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

`
