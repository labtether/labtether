package main

// v2OpenAPIOperationPaths documents this domain of the v2 API.
const v2OpenAPIOperationPaths = `    "/api/v2/metrics/overview": {
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

`
