package main

// v2OpenAPIAssetPaths documents this domain of the v2 API.
const v2OpenAPIAssetPaths = `    "/api/v2/assets": {
      "get":  { "summary": "List assets",  "description": "Scope: assets:read.",  "operationId": "listAssets",  "tags": ["assets"], "responses": { "200": { "description": "Asset list" } } },
      "post": { "summary": "Create asset", "description": "Scope: assets:write.", "operationId": "createAsset", "tags": ["assets"], "responses": { "201": { "description": "Created" } } }
    },
    "/api/v2/assets/{id}": {
      "get":    { "summary": "Get asset",    "description": "Scope: assets:read.",  "operationId": "getAsset",    "tags": ["assets"], "responses": { "200": { "description": "Asset" } } },
      "put":    { "summary": "Update asset", "description": "Scope: assets:write.", "operationId": "updateAsset", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
	  "patch":  { "summary": "Partially update asset", "description": "Scope: assets:write.", "operationId": "patchAsset", "tags": ["assets"], "responses": { "200": { "description": "Updated" } } },
      "delete": { "summary": "Delete asset", "description": "Scope: assets:write.", "operationId": "deleteAsset", "tags": ["assets"], "responses": { "200": { "description": "Deleted" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/exec": {
      "post": { "summary": "Execute a command on one asset", "description": "Scope: assets:exec. The asset must be allowed by the authenticated principal and have a connected agent.", "operationId": "execAsset", "tags": ["exec"], "responses": { "200": { "description": "Command result" }, "409": { "description": "Asset agent is offline" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files": {
      "get": { "summary": "List a directory", "description": "Scope: files:read. Supply the absolute path in the path query parameter.", "operationId": "listAssetFiles", "tags": ["files"], "parameters": [{ "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }], "responses": { "200": { "description": "Directory entries" } } },
      "delete": { "summary": "Delete a file or directory", "description": "Scope: files:write. Supply the absolute path in the path query parameter.", "operationId": "deleteAssetFile", "tags": ["files"], "parameters": [{ "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }], "responses": { "200": { "description": "Delete result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/read": {
      "get": { "summary": "Download a file", "description": "Scope: files:read. Streams the file response rather than wrapping it in the JSON response envelope.", "operationId": "readAssetFile", "tags": ["files"], "parameters": [{ "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }], "responses": { "200": { "description": "File bytes" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/write": {
      "post": { "summary": "Upload a file", "description": "Scope: files:write.", "operationId": "writeAssetFile", "tags": ["files"], "responses": { "200": { "description": "Upload result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/mkdir": {
      "post": { "summary": "Create a directory", "description": "Scope: files:write.", "operationId": "createAssetDirectory", "tags": ["files"], "responses": { "200": { "description": "Create result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/rename": {
      "post": { "summary": "Rename or move a file", "description": "Scope: files:write.", "operationId": "renameAssetFile", "tags": ["files"], "responses": { "200": { "description": "Rename result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/files/copy": {
      "post": { "summary": "Copy a file", "description": "Scope: files:write.", "operationId": "copyAssetFile", "tags": ["files"], "responses": { "200": { "description": "Copy result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/processes": {
      "get": { "summary": "List running processes", "description": "Scope: processes:read.", "operationId": "listAssetProcesses", "tags": ["system"], "responses": { "200": { "description": "Process list" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/processes/kill": {
      "post": { "summary": "Terminate a process", "description": "Scope: processes:kill.", "operationId": "killAssetProcess", "tags": ["system"], "responses": { "200": { "description": "Termination result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services": {
      "get": { "summary": "List system services", "description": "Scope: services:read.", "operationId": "listAssetServices", "tags": ["system"], "responses": { "200": { "description": "Service list" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services/{name}/start": {
      "post": { "summary": "Start a system service", "description": "Scope: services:write.", "operationId": "startAssetService", "tags": ["system"], "responses": { "200": { "description": "Service action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services/{name}/stop": {
      "post": { "summary": "Stop a system service", "description": "Scope: services:write.", "operationId": "stopAssetService", "tags": ["system"], "responses": { "200": { "description": "Service action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/services/{name}/restart": {
      "post": { "summary": "Restart a system service", "description": "Scope: services:write.", "operationId": "restartAssetService", "tags": ["system"], "responses": { "200": { "description": "Service action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }, { "name": "name", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/network": {
      "get": { "summary": "List network interfaces", "description": "Scope: network:read.", "operationId": "listAssetNetwork", "tags": ["system"], "responses": { "200": { "description": "Network interfaces" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/disks": {
      "get": { "summary": "List disks and filesystems", "description": "Scope: disks:read.", "operationId": "listAssetDisks", "tags": ["system"], "responses": { "200": { "description": "Disk inventory" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages": {
      "get": { "summary": "List installed packages", "description": "Scope: packages:read.", "operationId": "listAssetPackages", "tags": ["system"], "responses": { "200": { "description": "Installed packages" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/upgradable": {
      "get": { "summary": "List upgradable packages", "description": "Scope: packages:read.", "operationId": "listAssetUpgradablePackages", "tags": ["system"], "responses": { "200": { "description": "Upgradable packages" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/install": {
      "post": { "summary": "Install a package", "description": "Scope: packages:write.", "operationId": "installAssetPackage", "tags": ["system"], "responses": { "200": { "description": "Package action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/update": {
      "post": { "summary": "Update package metadata", "description": "Scope: packages:write.", "operationId": "updateAssetPackageMetadata", "tags": ["system"], "responses": { "200": { "description": "Package action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/packages/upgrade": {
      "post": { "summary": "Upgrade packages", "description": "Scope: packages:write.", "operationId": "upgradeAssetPackages", "tags": ["system"], "responses": { "200": { "description": "Package action result" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/cron": {
      "get": { "summary": "List scheduled operating-system jobs", "description": "Scope: cron:read.", "operationId": "listAssetCron", "tags": ["system"], "responses": { "200": { "description": "Scheduled jobs" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/users": {
      "get": { "summary": "List operating-system users", "description": "Scope: users:read.", "operationId": "listAssetUsers", "tags": ["system"], "responses": { "200": { "description": "User list" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/logs": {
      "get": { "summary": "Query asset logs", "description": "Scope: logs:read.", "operationId": "queryAssetLogs", "tags": ["logs"], "responses": { "200": { "description": "Log entries" } } },
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }]
    },
    "/api/v2/assets/{id}/wake": {
      "post": {
        "summary": "Wake an asset",
        "description": "Scope: assets:power. Sends Wake-on-LAN directly from the hub or queues delivery through a correlated online relay agent.",
        "operationId": "wakeAsset",
        "tags": ["assets"],
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "202": { "description": "Magic packet sent directly or queued through an agent relay" },
          "422": { "description": "No valid MAC address is known" }
        }
      }
    },
    "/api/v2/assets/{id}/reboot": {
      "post": {
        "summary": "Reboot an asset",
        "description": "Scope: assets:power. Sends a typed power.action to the connected agent and returns success only after a strictly correlated power.result reports that the operating system accepted the reboot request.",
        "operationId": "rebootAsset",
        "tags": ["assets"],
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "202": { "description": "Operating system accepted the reboot request" },
          "409": { "description": "Agent offline or request rejected" },
          "422": { "description": "Power action unsupported on the agent platform" },
          "429": { "description": "Global power-action concurrency limit reached" },
          "502": { "description": "Delivery or operating-system execution failed" },
          "504": { "description": "Timed out waiting for a correlated agent result" }
        }
      }
    },
    "/api/v2/assets/{id}/shutdown": {
      "post": {
        "summary": "Shut down an asset",
        "description": "Scope: assets:power. Sends a typed power.action to the connected agent and returns success only after a strictly correlated power.result reports that the operating system accepted the shutdown request.",
        "operationId": "shutdownAsset",
        "tags": ["assets"],
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "202": { "description": "Operating system accepted the shutdown request" },
          "409": { "description": "Agent offline or request rejected" },
          "422": { "description": "Power action unsupported on the agent platform" },
          "429": { "description": "Global power-action concurrency limit reached" },
          "502": { "description": "Delivery or operating-system execution failed" },
          "504": { "description": "Timed out waiting for a correlated agent result" }
        }
      }
    },
    "/api/v2/assets/{id}/metrics": {
      "get": {
        "summary": "Asset metric time-series",
        "description": "Scope: metrics:read.",
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
        "description": "Scope: metrics:read.",
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
`
