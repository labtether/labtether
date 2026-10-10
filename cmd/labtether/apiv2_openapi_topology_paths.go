package main

// v2OpenAPITopologyPaths documents this domain of the v2 API.
const v2OpenAPITopologyPaths = `    "/api/v2/discovery/run": {
      "post": { "summary": "Analyze known assets for relationship links", "description": "Runs synchronously on already known assets; may create links or suggestions, and does not scan the network. Scope: discovery:write.", "operationId": "runDiscovery", "tags": ["discovery"], "responses": { "200": { "description": "Relationship analysis completed" } } }
    },
    "/api/v2/discovery/proposals": {
      "get": { "summary": "List discovery proposals", "description": "Unaccepted assets found by discovery. Scope: discovery:read.", "operationId": "listDiscoveryProposals", "tags": ["discovery"], "responses": { "200": { "description": "Proposal list" } } }
    },
	"/api/v2/discovery/proposals/{id}/accept": {
	  "post": { "summary": "Accept discovery proposal", "description": "Scope: discovery:write.", "operationId": "acceptDiscoveryProposal", "tags": ["discovery"], "responses": { "200": { "description": "Accepted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/discovery/proposals/{id}/dismiss": {
	  "post": { "summary": "Dismiss discovery proposal", "description": "Scope: discovery:write.", "operationId": "dismissDiscoveryProposal", "tags": ["discovery"], "responses": { "200": { "description": "Dismissed" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/dependencies": {
      "get":  { "summary": "List service dependencies",  "description": "Scope: topology:read.",  "operationId": "listDependencies",  "tags": ["dependencies"], "responses": { "200": { "description": "Dependency list" } } },
      "post": { "summary": "Create service dependency",  "description": "Scope: topology:write.", "operationId": "createDependency", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/dependencies/{id}": {
      "get":    { "summary": "Get dependency",    "description": "Scope: topology:read.",  "operationId": "getDependency",    "tags": ["dependencies"], "responses": { "200": { "description": "Dependency" } } },
      "delete": { "summary": "Delete dependency", "description": "Scope: topology:write.", "operationId": "deleteDependency", "tags": ["dependencies"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/dependencies/batch": {
	  "get": { "summary": "List dependencies for multiple assets", "description": "Scope: topology:read. Every requested asset must be allowed by the authenticated principal.", "operationId": "listDependenciesBatch", "tags": ["dependencies"], "responses": { "200": { "description": "Dependency list" } } }
	},
	"/api/v2/dependencies/graph": {
	  "get": { "summary": "Get dependency graph around an asset", "description": "Scope: topology:read.", "operationId": "getDependencyGraph", "tags": ["dependencies"], "responses": { "200": { "description": "Dependency graph" } } }
	},
    "/api/v2/edges": {
      "get":  { "summary": "List topology edges",  "description": "Scope: topology:read.",  "operationId": "listEdges",  "tags": ["dependencies"], "responses": { "200": { "description": "Edge list" } } },
      "post": { "summary": "Create topology edge",  "description": "Scope: topology:write.", "operationId": "createEdge", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
	"/api/v2/edges/{id}": {
	  "get":    { "summary": "Get topology edge", "description": "Scope: topology:read.", "operationId": "getEdge", "tags": ["dependencies"], "responses": { "200": { "description": "Edge" } } },
	  "patch":  { "summary": "Update topology edge", "description": "Scope: topology:write.", "operationId": "updateEdge", "tags": ["dependencies"], "responses": { "200": { "description": "Updated" } } },
	  "delete": { "summary": "Delete topology edge", "description": "Scope: topology:write.", "operationId": "deleteEdge", "tags": ["dependencies"], "responses": { "200": { "description": "Deleted" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/edges/tree": {
	  "get": { "summary": "Get descendant edge tree", "description": "Scope: topology:read.", "operationId": "getEdgeTree", "tags": ["dependencies"], "responses": { "200": { "description": "Descendant tree" } } }
	},
	"/api/v2/edges/ancestors": {
	  "get": { "summary": "Get edge ancestor chain", "description": "Scope: topology:read.", "operationId": "getEdgeAncestors", "tags": ["dependencies"], "responses": { "200": { "description": "Ancestor chain" } } }
	},
    "/api/v2/composites": {
      "post": { "summary": "Create composite service",  "description": "Scope: topology:write.", "operationId": "createComposite", "tags": ["dependencies"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/composites/{id}": {
      "get":    { "summary": "Get composite",    "description": "Scope: topology:read.",  "operationId": "getComposite",    "tags": ["dependencies"], "responses": { "200": { "description": "Composite" } } },
	  "patch":  { "summary": "Change composite primary asset", "description": "Scope: topology:write.", "operationId": "updateComposite", "tags": ["dependencies"], "responses": { "200": { "description": "Updated" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/composites/{id}/members/{assetId}": {
	  "delete": { "summary": "Detach a composite member", "description": "Scope: topology:write.", "operationId": "detachCompositeMember", "tags": ["dependencies"], "responses": { "200": { "description": "Detached" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "assetId", "in": "path", "required": true, "schema": { "type": "string" } }]
	},

    "/api/v2/topology": {
      "get": { "summary": "Full topology canvas state", "description": "Returns zones, members, connections, and viewport. Scope: topology:read.", "operationId": "getTopology", "tags": ["topology"], "responses": { "200": { "description": "Topology layout" } } }
    },
    "/api/v2/topology/zones": {
      "post": { "summary": "Create topology zone", "description": "Scope: topology:write.", "operationId": "createTopologyZone", "tags": ["topology"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/topology/zones/{id}": {
      "put":    { "summary": "Update zone", "description": "Scope: topology:write.", "operationId": "updateTopologyZone", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete zone", "description": "Scope: topology:write.", "operationId": "deleteTopologyZone", "tags": ["topology"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
	"/api/v2/topology/zones/{id}/members": {
	  "put": { "summary": "Replace topology zone membership", "description": "Scope: topology:write.", "operationId": "setTopologyZoneMembers", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
	  "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
	},
	"/api/v2/topology/zones/reorder": {
	  "put": { "summary": "Reorder topology zones", "description": "Scope: topology:write.", "operationId": "reorderTopologyZones", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } }
	},
    "/api/v2/topology/connections": {
      "post": { "summary": "Create topology connection", "description": "Scope: topology:write.", "operationId": "createTopologyConnection", "tags": ["topology"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/topology/connections/{id}": {
      "put":    { "summary": "Update connection", "description": "Scope: topology:write.", "operationId": "updateTopologyConnection", "tags": ["topology"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete connection", "description": "Scope: topology:write.", "operationId": "deleteTopologyConnection", "tags": ["topology"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/topology/viewport": {
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
	  "patch":  { "summary": "Partially update failover pair", "description": "Scope: failover:write.", "operationId": "patchFailoverPair", "tags": ["failover"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete failover pair", "description": "Scope: failover:write.", "operationId": "deleteFailoverPair", "tags": ["failover"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },

`
