package main

// v2OpenAPIContainerAndTransferPaths documents this domain of the v2 API.
const v2OpenAPIContainerAndTransferPaths = `    "/api/v2/docker/hosts": {
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

`
