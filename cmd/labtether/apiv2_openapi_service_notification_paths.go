package main

// v2OpenAPIServiceNotificationPaths documents this domain of the v2 API.
const v2OpenAPIServiceNotificationPaths = `    "/api/v2/web-services": {
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

`
