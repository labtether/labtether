package main

// v2OpenAPIConnectorPaths documents this domain of the v2 API.
const v2OpenAPIConnectorPaths = `    "/api/v2/connectors": {
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

`
