package main

// v2OpenAPIAlertIncidentPaths documents this domain of the v2 API.
const v2OpenAPIAlertIncidentPaths = `    "/api/v2/alerts": {
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

`
