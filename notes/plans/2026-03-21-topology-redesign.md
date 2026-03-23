# Topology Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the auto-inferred topology tab (~39 files, 4 views) with a user-defined zoned canvas with expandable containment cards, hybrid connections, and a derived tree view.

**Architecture:** Backend-first approach. New Postgres tables + Go store + v2 API handlers, then frontend canvas built on React Flow. Old topology code deleted once new system is stable behind a feature flag. Two clean subsystems: backend (Go CRUD + auto-seed) and frontend (React Flow canvas + tree).

**Tech Stack:** Go 1.26, Postgres (migrations in `internal/persistence`), Next.js App Router, React Flow (`@xyflow/react`), TypeScript

**Spec:** `notes/specs/2026-03-21-topology-redesign-design.md`

---

## File Structure

### Backend (Go)

| File | Responsibility |
|------|---------------|
| `internal/persistence/postgres_schema_migrations.go` | Modify: add migration for topology tables |
| `internal/topology/types.go` | Create: TopologyLayout, Zone, ZoneMember, TopologyConnection, DismissedAsset structs |
| `internal/topology/store.go` | Create: Store interface |
| `internal/topology/postgres_store.go` | Create: Postgres implementation of Store |
| `internal/topology/postgres_store_test.go` | Create: Integration tests for store |
| `internal/topology/seed.go` | Create: Auto-seed logic (first-load zone generation from existing data) |
| `internal/topology/seed_test.go` | Create: Tests for auto-seed |
| `internal/topology/suggest.go` | Create: Placement suggestion logic for unsorted inbox |
| `internal/topology/suggest_test.go` | Create: Tests for suggestion logic |
| `internal/topology/connection_merge.go` | Create: Merge topology_connections + asset_edges with conflict resolution |
| `internal/topology/connection_merge_test.go` | Create: Tests for merge logic |
| `cmd/labtether/apiv2_topology.go` | Create: v2 API handlers for all topology endpoints |
| `cmd/labtether/apiv2_topology_test.go` | Create: HTTP handler tests |
| `cmd/labtether/http_handlers.go` | Modify: register topology routes |
| `cmd/labtether/server_types.go` | Modify: add TopologyStore to apiServer |

### Frontend (TypeScript/React)

| File | Responsibility |
|------|---------------|
| `web/console/app/api/topology/[[...path]]/route.ts` | Create: API proxy catch-all for `/api/v2/topology/*` |
| `web/console/app/[locale]/(console)/topology/page.tsx` | Modify: feature flag switch between old/new |
| `web/console/app/[locale]/(console)/topology/TopologyCanvasPage.tsx` | Create: New topology page orchestrator |
| `web/console/app/[locale]/(console)/topology/useTopologyData.ts` | Create: Data fetching + mutation hooks |
| `web/console/app/[locale]/(console)/topology/topologyCanvasTypes.ts` | Create: TypeScript types for new topology |
| `web/console/app/[locale]/(console)/topology/TopologyToolbar.tsx` | Modify: replace old toolbar with canvas/tree toggle + new controls |
| `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx` | Create: React Flow canvas with zones, cards, connections |
| `web/console/app/[locale]/(console)/topology/ZoneNode.tsx` | Create: Custom React Flow node for zones |
| `web/console/app/[locale]/(console)/topology/ContainmentCard.tsx` | Create: Expandable asset card with containment layers |
| `web/console/app/[locale]/(console)/topology/ContainmentLayer.tsx` | Create: Collapsible layer within a containment card (VMs, Containers, etc.) |
| `web/console/app/[locale]/(console)/topology/TopologyConnections.tsx` | Create: Connection line rendering + drawing interaction |
| `web/console/app/[locale]/(console)/topology/TopologyInbox.tsx` | Create: Unsorted inbox sidebar panel |
| `web/console/app/[locale]/(console)/topology/TopologyInspector.tsx` | Modify: replace float-over with slide-in panel |
| `web/console/app/[locale]/(console)/topology/TopologyTreeView.tsx` | Create: Derived tree outline from canvas state |
| `web/console/app/[locale]/(console)/topology/TopologyContextMenus.tsx` | Create: Right-click context menus for canvas/zone/asset/connection |
| `web/console/app/[locale]/(console)/topology/TopologySearch.tsx` | Create: Search overlay with auto-expand + pan-to |
| `web/console/app/[locale]/(console)/topology/ConnectToDialog.tsx` | Create: "Connect to..." searchable asset picker |
| `web/console/app/[locale]/(console)/topology/useTopologyUndo.ts` | Create: Client-side undo/redo stack |
| `web/console/app/[locale]/(console)/topology/topologySmartDefaults.ts` | Create: Connection type inference from asset types |

---

## Phase 1: Backend Foundation

### Task 1: Database Migration

**Files:**
- Modify: `internal/persistence/postgres_schema_migrations.go`

- [ ] **Step 1: Read the existing migrations file to find the current version number**

Run: `grep -n "Version:" internal/persistence/postgres_schema_migrations.go | tail -5`

Note the last version number. The new migration will be version N+1.

- [ ] **Step 2: Add the topology tables migration**

Add a new `schemaMigration` entry at the end of the `postgresSchemaMigrations()` slice. Follow the exact pattern of existing migrations (IF NOT EXISTS, TIMESTAMPTZ, snake_case):

```go
{
    Version: N, // replace N with next version number
    Name:    "topology_canvas",
    Statements: []string{
        `CREATE TABLE IF NOT EXISTS topology_layouts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            name TEXT NOT NULL DEFAULT 'My Homelab',
            viewport JSONB NOT NULL DEFAULT '{"x":0,"y":0,"zoom":1}',
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`,
        `CREATE TABLE IF NOT EXISTS topology_zones (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            topology_id UUID NOT NULL REFERENCES topology_layouts(id) ON DELETE CASCADE,
            parent_zone_id UUID REFERENCES topology_zones(id) ON DELETE SET NULL,
            label TEXT NOT NULL,
            color TEXT NOT NULL DEFAULT 'blue',
            icon TEXT NOT NULL DEFAULT '',
            position JSONB NOT NULL DEFAULT '{"x":0,"y":0}',
            size JSONB NOT NULL DEFAULT '{"width":300,"height":200}',
            collapsed BOOLEAN NOT NULL DEFAULT false,
            sort_order INT NOT NULL DEFAULT 0,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`,
        `CREATE INDEX IF NOT EXISTS idx_topology_zones_topology ON topology_zones(topology_id)`,
        `CREATE INDEX IF NOT EXISTS idx_topology_zones_parent ON topology_zones(parent_zone_id)`,
        `CREATE TABLE IF NOT EXISTS zone_members (
            zone_id UUID NOT NULL REFERENCES topology_zones(id) ON DELETE CASCADE,
            asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            position JSONB NOT NULL DEFAULT '{"x":0,"y":0}',
            sort_order INT NOT NULL DEFAULT 0,
            PRIMARY KEY (zone_id, asset_id)
        )`,
        `CREATE UNIQUE INDEX IF NOT EXISTS idx_zone_members_asset ON zone_members(asset_id)`,
        `CREATE TABLE IF NOT EXISTS topology_connections (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            topology_id UUID NOT NULL REFERENCES topology_layouts(id) ON DELETE CASCADE,
            source_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            target_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            relationship TEXT NOT NULL,
            user_defined BOOLEAN NOT NULL DEFAULT true,
            label TEXT NOT NULL DEFAULT '',
            deleted BOOLEAN NOT NULL DEFAULT false,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`,
        `CREATE UNIQUE INDEX IF NOT EXISTS idx_topology_connections_unique
            ON topology_connections(topology_id, source_asset_id, target_asset_id, relationship)
            WHERE deleted = false`,
        `CREATE INDEX IF NOT EXISTS idx_topology_connections_topology ON topology_connections(topology_id)`,
        `CREATE TABLE IF NOT EXISTS dismissed_assets (
            topology_id UUID NOT NULL REFERENCES topology_layouts(id) ON DELETE CASCADE,
            asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            source TEXT NOT NULL DEFAULT '',
            type TEXT NOT NULL DEFAULT '',
            dismissed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            PRIMARY KEY (topology_id, asset_id)
        )`,
    },
},
```

- [ ] **Step 3: Verify migration compiles**

Run: `go vet ./internal/persistence/...`
Expected: no errors

- [ ] **Step 4: Test migration runs against local Postgres**

Run: `make dev-backend-bg` (or restart if already running)
Check logs at `/tmp/labtether-dev-backend.log` for migration output.
Expected: migration applies without errors.

- [ ] **Step 5: Verify tables exist**

Run: `docker exec -i $(docker ps -q -f name=postgres) psql -U labtether -c "\dt topology_*; \dt zone_*; \dt dismissed_*;"`
Expected: all 5 tables listed.

- [ ] **Step 6: Commit**

```
feat(topology): add database migration for topology canvas tables
```

---

### Task 2: Topology Store — Types and Interface

**Files:**
- Create: `internal/topology/types.go`
- Create: `internal/topology/store.go`

- [ ] **Step 1: Create the types file**

```go
// internal/topology/types.go
package topology

import "time"

type Position struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
}

type Size struct {
    Width  float64 `json:"width"`
    Height float64 `json:"height"`
}

type Viewport struct {
    X    float64 `json:"x"`
    Y    float64 `json:"y"`
    Zoom float64 `json:"zoom"`
}

type Layout struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Viewport  Viewport  `json:"viewport"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type Zone struct {
    ID           string   `json:"id"`
    TopologyID   string   `json:"topology_id"`
    ParentZoneID string   `json:"parent_zone_id,omitempty"`
    Label        string   `json:"label"`
    Color        string   `json:"color"`
    Icon         string   `json:"icon,omitempty"`
    Position     Position `json:"position"`
    Size         Size     `json:"size"`
    Collapsed    bool     `json:"collapsed"`
    SortOrder    int      `json:"sort_order"`
}

type ZoneMember struct {
    ZoneID    string   `json:"zone_id"`
    AssetID   string   `json:"asset_id"`
    Position  Position `json:"position"`
    SortOrder int      `json:"sort_order"`
}

// ValidRelationships is the set of allowed relationship types for topology connections.
var ValidRelationships = map[string]bool{
    "runs_on":      true,
    "hosted_on":    true,
    "depends_on":   true,
    "provides_to":  true,
    "connected_to": true,
    "peer_of":      true,
}

type Connection struct {
    ID            string `json:"id"`
    TopologyID    string `json:"topology_id"`
    SourceAssetID string `json:"source_asset_id"`
    TargetAssetID string `json:"target_asset_id"`
    Relationship  string `json:"relationship"`
    UserDefined   bool   `json:"user_defined"`
    Label         string `json:"label,omitempty"`
    Deleted       bool   `json:"-"` // internal, not exposed in API
}

// MergedConnection is the result of merging topology_connections with asset_edges.
// Origin indicates where the connection came from.
type MergedConnection struct {
    Connection
    Origin string `json:"origin"` // "discovered", "user", "accepted"
}

type TopologyState struct {
    ID          string             `json:"id"`       // layout ID, needed for mutations
    Name        string             `json:"name"`
    Zones       []Zone             `json:"zones"`
    Members     []ZoneMember       `json:"members"`
    Connections []MergedConnection `json:"connections"`
    Unsorted    []string           `json:"unsorted"` // asset IDs not in any zone
    Viewport    Viewport           `json:"viewport"`
}
```

- [ ] **Step 2: Create the store interface**

```go
// internal/topology/store.go
package topology

// Store defines persistence operations for the topology canvas.
type Store interface {
    // Layout
    GetOrCreateLayout() (Layout, error)
    UpdateViewport(viewport Viewport) error

    // Zones
    CreateZone(z Zone) (Zone, error)
    UpdateZone(z Zone) error
    DeleteZone(id string) error // cascades: members move to parent or become unsorted
    ListZones(topologyID string) ([]Zone, error)
    ReorderZones(updates []ZoneReorder) error

    // Members
    SetMembers(zoneID string, members []ZoneMember) error
    RemoveMember(assetID string) error
    ListMembers(topologyID string) ([]ZoneMember, error)

    // Connections
    CreateConnection(c Connection) (Connection, error)
    UpdateConnection(id string, relationship, label string) error
    DeleteConnection(id string) error // soft-delete (sets deleted=true)
    ListConnections(topologyID string) ([]Connection, error)

    // Dismissed
    DismissAsset(topologyID, assetID string) error
    UndismissAsset(topologyID, assetID string) error
    ListDismissed(topologyID string) ([]string, error)
}

type ZoneReorder struct {
    ZoneID       string `json:"zone_id"`
    ParentZoneID string `json:"parent_zone_id,omitempty"`
    SortOrder    int    `json:"sort_order"`
}
```

- [ ] **Step 3: Verify compilation**

Run: `go vet ./internal/topology/...`
Expected: no errors

- [ ] **Step 4: Commit**

```
feat(topology): add topology types and store interface
```

---

### Task 3: Topology Store — Postgres Implementation

**Files:**
- Create: `internal/topology/postgres_store.go`
- Create: `internal/topology/postgres_store_test.go`

- [ ] **Step 1: Implement the Postgres store**

Create `internal/topology/postgres_store.go` implementing the `Store` interface. Use `database/sql` with `lib/pq` (matching existing codebase patterns). Key implementation notes:

- `GetOrCreateLayout`: SELECT the single layout row; if none exists, INSERT one with defaults and return it.
- `DeleteZone`: Before deleting, UPDATE all child zones to set `parent_zone_id` to the deleted zone's parent. DELETE the zone (CASCADE handles zone_members).
- `SetMembers`: Use a transaction — DELETE existing members for the zone, then INSERT the new set. Enforce the single-zone constraint by first removing any existing membership for each asset_id.
- `DeleteConnection`: SET `deleted = true` (soft-delete so discovered edges don't reappear).
- `CreateConnection`: INSERT with ON CONFLICT on the unique index — if the connection was previously soft-deleted, UPDATE to set `deleted = false` and update fields.

- [ ] **Step 2: Write integration tests**

Create `internal/topology/postgres_store_test.go`. Tests should use a real test database (follow existing test patterns in the codebase). Cover:

- `TestGetOrCreateLayout` — creates on first call, returns same on second
- `TestZoneCRUD` — create, list, update, delete
- `TestZoneNesting` — create parent, create child, delete parent (child re-parents to nil)
- `TestMemberOperations` — set members, move asset between zones (single-zone constraint), remove member
- `TestConnectionCRUD` — create, update type, soft-delete, re-create after delete
- `TestConnectionUniqueConstraint` — same source+target+type rejected, different type allowed
- `TestDismissedAssets` — dismiss, list, undismiss

- [ ] **Step 3: Run tests**

Run: `go test ./internal/topology/... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```
feat(topology): implement postgres store with tests
```

---

### Task 4: Connection Merge Logic

**Files:**
- Create: `internal/topology/connection_merge.go`
- Create: `internal/topology/connection_merge_test.go`

- [ ] **Step 1: Implement the merge function**

```go
// internal/topology/connection_merge.go
package topology

import "github.com/labtether/labtether/internal/edges"

// MergeConnections combines topology_connections with asset_edges into a unified list.
// Priority: topology_connections wins over asset_edges for the same (source, target, relationship) tuple.
// Soft-deleted topology_connections suppress matching asset_edges.
func MergeConnections(topoConns []Connection, assetEdges []edges.Edge) []MergedConnection {
    // Build lookup of topology connections keyed by (source, target, relationship)
    type key struct{ src, tgt, rel string }
    topoByKey := make(map[key]Connection, len(topoConns))
    for _, tc := range topoConns {
        k := key{tc.SourceAssetID, tc.TargetAssetID, tc.Relationship}
        topoByKey[k] = tc
    }

    var result []MergedConnection

    // Add all non-deleted topology connections
    for _, tc := range topoConns {
        if tc.Deleted {
            continue
        }
        origin := "user"
        if !tc.UserDefined {
            origin = "accepted"
        }
        result = append(result, MergedConnection{Connection: tc, Origin: origin})
    }

    // Add discovered edges that don't conflict with topology connections
    for _, edge := range assetEdges {
        if edge.RelationshipType == "contains" {
            continue // containment handled by cards, not connections
        }
        if !ValidRelationships[edge.RelationshipType] {
            continue
        }
        k := key{edge.SourceAssetID, edge.TargetAssetID, edge.RelationshipType}
        if _, exists := topoByKey[k]; exists {
            continue // topology connection takes precedence (including soft-deletes)
        }
        result = append(result, MergedConnection{
            Connection: Connection{
                ID:            edge.ID,
                SourceAssetID: edge.SourceAssetID,
                TargetAssetID: edge.TargetAssetID,
                Relationship:  edge.RelationshipType,
                UserDefined:   false,
            },
            Origin: "discovered",
        })
    }

    return result
}
```

- [ ] **Step 2: Write tests**

Cover: topology wins over discovered for same tuple, soft-deleted suppresses discovered, `contains` edges excluded, unknown relationship types excluded, both sources contribute unique connections.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/topology/... -v -run Merge`
Expected: all pass

- [ ] **Step 4: Commit**

```
feat(topology): add connection merge logic for topology + discovered edges
```

---

### Task 5: Placement Suggestion Logic

**Files:**
- Create: `internal/topology/suggest.go`
- Create: `internal/topology/suggest_test.go`

- [ ] **Step 1: Implement suggestion logic**

```go
// internal/topology/suggest.go
package topology

// Suggestion represents a placement suggestion for an unsorted asset.
type Suggestion struct {
    AssetID  string `json:"asset_id"`
    ZoneID   string `json:"zone_id,omitempty"`
    ZoneLabel string `json:"zone_label,omitempty"`
    Reason   string `json:"reason,omitempty"` // "parent_host", "same_source", "same_type"
}

// SuggestPlacements generates placement suggestions for unsorted assets.
// Priority:
//  1. Parent host already in a zone → suggest same zone
//  2. Same source as existing zone members → suggest that zone
//  3. Same type as existing zone members → suggest that zone
//  4. No match → no suggestion
func SuggestPlacements(
    unsortedAssets []AssetInfo,
    zones []Zone,
    members []ZoneMember,
    memberAssets []AssetInfo, // asset info for assets already in zones
    parentMap map[string]string, // childAssetID → parentAssetID from contains edges
) []Suggestion {
    // ... implementation
}

// CheckDismissedForChanges compares dismissed assets against their current state.
// If an asset's source or type has changed since dismissal, it should reappear in the inbox.
// Returns asset IDs that should be undismissed.
func CheckDismissedForChanges(
    dismissed []DismissedAsset,
    currentAssets map[string]AssetInfo,
) []string {
    // Compare dismissed asset's recorded source/type against current values
    // Return IDs where source or type changed
}

// DismissedAsset tracks dismissal state with the asset's state at dismissal time.
type DismissedAsset struct {
    TopologyID  string
    AssetID     string
    Source      string // asset source at time of dismissal
    Type        string // asset type at time of dismissal
    DismissedAt string
}

// AssetInfo is the minimal asset data needed for suggestion logic.
type AssetInfo struct {
    ID     string
    Source string
    Type   string
}
```

- [ ] **Step 2: Write tests**

Cover: parent host match, source match, type match, no match, priority ordering (parent > source > type).

- [ ] **Step 3: Run tests and commit**

```
feat(topology): add placement suggestion engine for unsorted inbox
```

---

### Task 6: Auto-Seed Logic

**Files:**
- Create: `internal/topology/seed.go`
- Create: `internal/topology/seed_test.go`

- [ ] **Step 1: Implement auto-seed**

The auto-seed runs on first load (when `topology_zones` is empty for the layout). It:
1. Gets all assets from status context
2. Gets existing group assignments (asset.group_id)
3. Creates zones named after groups (or by source if no group)
4. Places assets into appropriate zones
5. Copies explicit edges from `asset_edges` into `topology_connections` with `user_defined: false`

```go
// internal/topology/seed.go
package topology

// SeedInput contains the data needed to auto-seed a new topology.
type SeedInput struct {
    Assets      []AssetInfo
    Groups      map[string]string // groupID → groupLabel
    AssetGroups map[string]string // assetID → groupID
    Edges       []EdgeInfo        // explicit edges to import as connections
}

type EdgeInfo struct {
    SourceAssetID string
    TargetAssetID string
    Relationship  string
}

// SeedResult contains the zones and members generated by auto-seed.
type SeedResult struct {
    Zones   []Zone
    Members []ZoneMember
    Connections []Connection
}

// Seed generates initial zones, members, and connections from existing data.
func Seed(input SeedInput) SeedResult {
    // 1. Create zones from groups (assets with group_id → zone named after group)
    // 2. Assets without groups → create zones by source ("Proxmox", "TrueNAS", etc.)
    // 3. Remaining assets without source → "Unsorted" (no zone)
    // 4. Place assets into zones with auto-layout positions
    // 5. Import explicit edges as connections (user_defined=false)
    // ...
}
```

- [ ] **Step 2: Write tests**

Cover: group-based zones, source-based fallback zones, edge import, empty state (no assets).

- [ ] **Step 3: Run tests and commit**

```
feat(topology): add auto-seed for first-load zone generation
```

---

### Task 7: API Handlers

**Files:**
- Create: `cmd/labtether/apiv2_topology.go`
- Modify: `cmd/labtether/http_handlers.go`
- Modify: `cmd/labtether/server_types.go`

- [ ] **Step 1: Add TopologyStore to apiServer**

In `cmd/labtether/server_types.go`, add `TopologyStore topology.Store` to the `apiServer` struct.

- [ ] **Step 2: Initialize the store**

Find where other stores are initialized (look for where EdgeStore is created) and add TopologyStore initialization with the same db connection.

- [ ] **Step 3: Create the API handler file**

Create `cmd/labtether/apiv2_topology.go` with handlers for all 12 endpoints. Follow the existing v2 handler pattern:

```go
package main

import (
    "encoding/json"
    "net/http"
    "strings"

    "github.com/labtether/hub/internal/apiv2"
    "github.com/labtether/hub/internal/topology"
)

func (s *apiServer) handleV2Topology(w http.ResponseWriter, r *http.Request) {
    scope := "topology:read"
    if apiv2.IsMutatingMethod(r.Method) {
        scope = "topology:write"
    }
    if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
        apiv2.WriteScopeForbidden(w, scope)
        return
    }

    switch r.Method {
    case http.MethodGet:
        s.handleGetTopology(w, r)
    default:
        apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET only")
    }
}

func (s *apiServer) handleGetTopology(w http.ResponseWriter, r *http.Request) {
    layout, err := s.TopologyStore.GetOrCreateLayout()
    if err != nil {
        apiv2.WriteError(w, 500, "internal", err.Error())
        return
    }

    zones, err := s.TopologyStore.ListZones(layout.ID)
    // ... list members, connections, compute unsorted, merge with edges
    // Build TopologyState response
    // Run auto-seed if zones are empty

    apiv2.WriteJSON(w, http.StatusOK, state)
}

// handleV2TopologyZones handles POST /api/v2/topology/zones
func (s *apiServer) handleV2TopologyZones(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyZone handles PUT/DELETE /api/v2/topology/zones/{id}
func (s *apiServer) handleV2TopologyZone(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyZoneMembers handles PUT /api/v2/topology/zones/{id}/members
func (s *apiServer) handleV2TopologyZoneMembers(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyZonesReorder handles PUT /api/v2/topology/zones/reorder
func (s *apiServer) handleV2TopologyZonesReorder(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyConnections handles POST /api/v2/topology/connections
func (s *apiServer) handleV2TopologyConnections(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyConnection handles PUT/DELETE /api/v2/topology/connections/{id}
// IMPORTANT: DELETE must soft-delete (set deleted=true) via store.DeleteConnection,
// NOT hard-delete. This ensures discovered edges with the same tuple don't reappear.
func (s *apiServer) handleV2TopologyConnection(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyViewport handles PUT /api/v2/topology/viewport
func (s *apiServer) handleV2TopologyViewport(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyUnsorted handles GET /api/v2/topology/unsorted
func (s *apiServer) handleV2TopologyUnsorted(w http.ResponseWriter, r *http.Request) { ... }

// handleV2TopologyAutoPlace handles POST /api/v2/topology/auto-place
func (s *apiServer) handleV2TopologyAutoPlace(w http.ResponseWriter, r *http.Request) { ... }
```

- [ ] **Step 4: Register routes in http_handlers.go**

Add to the handlers map in `buildHTTPHandlers()`:

```go
"/api/v2/topology":              s.withAuth(s.handleV2Topology),
"/api/v2/topology/zones":        s.withAuth(s.handleV2TopologyZones),
"/api/v2/topology/zones/":       s.withAuth(s.handleV2TopologyZoneActions),
"/api/v2/topology/connections":  s.withAuth(s.handleV2TopologyConnections),
"/api/v2/topology/connections/": s.withAuth(s.handleV2TopologyConnection),
"/api/v2/topology/viewport":    s.withAuth(s.handleV2TopologyViewport),
"/api/v2/topology/unsorted":    s.withAuth(s.handleV2TopologyUnsorted),
"/api/v2/topology/auto-place":  s.withAuth(s.handleV2TopologyAutoPlace),
```

**Route conflict resolution:** The `/zones/` handler dispatches internally by parsing the path suffix. This matches the existing pattern used for `/dependencies/` and `/composites/`:
- Path suffix is `"reorder"` → handle zone reorder
- Path suffix matches `"{id}/members"` → handle zone members
- Path suffix is a UUID → handle single zone PUT/DELETE
- This avoids Go's ServeMux prefix-matching issues with overlapping routes.

- [ ] **Step 5: Verify compilation**

Run: `go vet ./cmd/labtether/... && go vet ./internal/topology/...`
Expected: no errors

- [ ] **Step 6: Test endpoints manually**

Start the backend: `make dev-backend-bg`

```bash
# Get topology (should auto-seed if assets exist)
curl -s http://localhost:8080/api/v2/topology -H "Cookie: ..." | jq .

# Create a zone
curl -s -X POST http://localhost:8080/api/v2/topology/zones \
  -H "Content-Type: application/json" \
  -d '{"label":"Test Zone","color":"blue"}' | jq .
```

- [ ] **Step 7: Commit**

```
feat(topology): add v2 API handlers for topology CRUD
```

---

## Phase 2: Frontend Foundation

### Task 8: API Proxy + Feature Flag

**Files:**
- Create: `web/console/app/api/topology/[[...path]]/route.ts`
- Modify: `web/console/app/[locale]/(console)/topology/page.tsx`

- [ ] **Step 1: Create the API proxy catch-all**

Follow the existing pattern from `app/api/file-connections/[[...path]]/route.ts`. Create a catch-all route that proxies all methods to the backend:

```typescript
// web/console/app/api/topology/[[...path]]/route.ts
export const dynamic = "force-dynamic";

// Handle GET, POST, PUT, DELETE for /api/topology/*
// Proxy to backend /api/v2/topology/*
```

Forward the path suffix, query params, body, and auth headers. Support all HTTP methods.

- [ ] **Step 2: Add feature flag to topology page**

Read the existing `page.tsx` to understand the current server component structure. Add a feature flag check (use a simple env var `NEXT_PUBLIC_TOPOLOGY_V2` or a settings-based flag matching existing patterns). When enabled, render the new `TopologyCanvasPage`; when disabled, render the existing `TopologyPageClient`.

- [ ] **Step 3: Test the proxy**

Run the frontend dev server. Verify that `/api/topology` proxies correctly to the backend.

- [ ] **Step 4: Commit**

```
feat(topology): add API proxy and feature flag for v2 topology
```

---

### Task 9: Types + Data Hooks

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/topologyCanvasTypes.ts`
- Create: `web/console/app/[locale]/(console)/topology/useTopologyData.ts`

- [ ] **Step 1: Create TypeScript types**

```typescript
// topologyCanvasTypes.ts
export interface Position { x: number; y: number }
export interface Size { width: number; height: number }
export interface Viewport { x: number; y: number; zoom: number }

export interface Zone {
  id: string
  topology_id: string
  parent_zone_id: string | null
  label: string
  color: string
  icon: string
  position: Position
  size: Size
  collapsed: boolean
  sort_order: number
}

export interface ZoneMember {
  zone_id: string
  asset_id: string
  position: Position
  sort_order: number
}

export type ConnectionOrigin = "discovered" | "user" | "accepted"
export type RelationshipType = "runs_on" | "hosted_on" | "depends_on" | "provides_to" | "connected_to" | "peer_of"

export interface TopologyConnection {
  id: string
  source_asset_id: string
  target_asset_id: string
  relationship: RelationshipType
  user_defined: boolean
  label: string
  origin: ConnectionOrigin
}

export interface TopologyState {
  zones: Zone[]
  members: ZoneMember[]
  connections: TopologyConnection[]
  unsorted: string[]
  viewport: Viewport
}

export interface PlacementSuggestion {
  asset_id: string
  zone_id: string | null
  zone_label: string | null
  reason: string | null
}
```

- [ ] **Step 2: Create data hooks**

```typescript
// useTopologyData.ts
// Fetch full topology state, expose mutation functions
// Use SWR or simple fetch+state matching existing patterns in the codebase
```

The hook should expose:
- `topology: TopologyState | null` — loaded data
- `isLoading: boolean`
- `createZone(zone: Partial<Zone>): Promise<Zone>`
- `updateZone(id: string, updates: Partial<Zone>): Promise<void>`
- `deleteZone(id: string): Promise<void>`
- `setMembers(zoneID: string, members: ZoneMember[]): Promise<void>`
- `createConnection(conn: Partial<TopologyConnection>): Promise<TopologyConnection>`
- `updateConnection(id: string, updates: Partial<TopologyConnection>): Promise<void>`
- `deleteConnection(id: string): Promise<void>`
- `saveViewport(viewport: Viewport): Promise<void>` — debounced
- `dismissAsset(assetID: string): Promise<void>`
- `autoPlace(): Promise<void>`
- `refresh(): void` — re-fetch

Each mutation should optimistically update local state, call the API, and revalidate on error.

- [ ] **Step 3: Verify TypeScript compilation**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 4: Commit**

```
feat(topology): add canvas types and data hooks
```

---

### Task 10: Canvas Page Skeleton + Toolbar

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/TopologyCanvasPage.tsx`
- Modify: `web/console/app/[locale]/(console)/topology/TopologyToolbar.tsx` (or create new)

- [ ] **Step 1: Create the page orchestrator**

`TopologyCanvasPage.tsx` is a `'use client'` component that:
- Calls `useTopologyData()` to load state
- Manages view mode state (`"canvas" | "tree"`)
- Manages selection state (`selectedAssetID`, `selectedConnectionID`)
- Manages panel state (`"inbox" | "inspector" | null`)
- Renders: Toolbar + (Canvas or Tree) + side panel (Inbox or Inspector)
- Shows loading skeleton while data loads
- Shows empty state prompt when no assets exist

- [ ] **Step 2: Create/update the toolbar**

Toolbar contains:
- Canvas/Tree segment toggle
- "+ Zone" button
- "Connect" mode toggle
- "Fit View" button
- "Auto-layout" button
- Search input
- Inbox toggle with badge count

Wire up the view toggle and inbox badge to actual state. Other buttons can be no-ops initially (wired in later tasks).

- [ ] **Step 3: Verify it renders**

Run the frontend dev server with the feature flag enabled. Navigate to topology. Expected: toolbar renders, loading state shows, data loads (may be empty if no auto-seed has run yet).

- [ ] **Step 4: Commit**

```
feat(topology): add canvas page skeleton and toolbar
```

---

## Phase 3: Canvas Core

### Task 11: Zone Nodes

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/ZoneNode.tsx`
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx`

- [ ] **Step 1: Create the ZoneNode custom React Flow node**

A custom node type registered with React Flow. Renders:
- Colored border + semi-transparent background (color from zone data)
- Header bar with: chevron (collapse/expand), zone label (editable on click), asset count badge
- Resizable (React Flow's `NodeResizer`)
- Draggable via header
- When collapsed: summary pill ("3 assets, 2 sub-zones")
- Nesting: child zones render as React Flow nodes positioned inside the parent's bounds

- [ ] **Step 2: Create the TopologyCanvas component**

`TopologyCanvas.tsx` wraps `<ReactFlow>` with:
- Custom node types registered: `{ zone: ZoneNode, asset: ContainmentCard }`
- Dot grid background via `<Background variant="dots" />`
- `<MiniMap />` component
- `<Controls />` for zoom
- `fitView` on initial load
- Pan/zoom with viewport save (debounced via `onMoveEnd`)
- Convert topology zones to React Flow nodes
- Convert topology members to React Flow nodes positioned within zones

- [ ] **Step 3: Test that zones render on the canvas**

With auto-seeded data, zones should appear as colored rectangles on the canvas. Verify dragging, resizing, and the minimap.

- [ ] **Step 4: Commit**

```
feat(topology): add zone nodes and canvas renderer
```

---

### Task 12: Expandable Containment Cards

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/ContainmentCard.tsx`
- Create: `web/console/app/[locale]/(console)/topology/ContainmentLayer.tsx`

- [ ] **Step 1: Create ContainmentCard**

Custom React Flow node for assets. Three states:
- **Collapsed**: chevron + status dot + icon + name + type + summary badge ("4 VMs · 11 CT"). Connection handles on left/right.
- **Expanded**: header row + containment layers below. Card grows vertically. Zone auto-resizes.
- **Leaf**: no chevron, simple single-line card.

Uses the existing hierarchy/edge data from `StatusContext` to determine children. Builds the containment tree from `contains` edges in asset_edges.

Source badges shown when asset has multiple sources (Proxmox + Agent).

- [ ] **Step 2: Create ContainmentLayer**

Renders a collapsible section within a ContainmentCard:
- Layer header: chevron + icon + label ("Virtual Machines") + source badge ("Proxmox") + count
- Child rows: status dot + name + type/port info
- Each child row shows connection handles on hover (for drag-to-connect)
- Depth control: default 1 level, "+N more" overflow at 10 items

- [ ] **Step 3: Wire card expand/collapse to zone auto-resize**

When a card expands, calculate the new height. If it exceeds the zone bounds, resize the zone. Notify React Flow to re-layout.

- [ ] **Step 4: Test with real data**

Navigate to topology with a Proxmox host that has VMs with containers. Verify: collapsed shows summary, expanded shows VM layer with Docker containers inside, source badges render.

- [ ] **Step 5: Commit**

```
feat(topology): add expandable containment cards with source layers
```

---

### Task 13: Connections — Rendering + Drawing

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/TopologyConnections.tsx`
- Create: `web/console/app/[locale]/(console)/topology/topologySmartDefaults.ts`

- [ ] **Step 1: Create connection rendering**

Convert `TopologyConnection[]` to React Flow edges. Styling by origin:
- `"user"`: solid line, colored by relationship type
- `"accepted"`: solid line, same colors
- `"discovered"`: dashed line, muted gray

Color scheme (matching existing):
- `runs_on` / `hosted_on`: green
- `depends_on`: orange
- `provides_to`: blue
- `connected_to` / `peer_of`: gray

Handle connections to collapsed cards: if the target asset is inside a collapsed containment card, route the edge to the card's boundary node.

- [ ] **Step 2: Create smart defaults**

```typescript
// topologySmartDefaults.ts
export function inferRelationshipType(
  sourceType: string,
  targetType: string,
): RelationshipType {
  // Container/VM → Host = runs_on
  // Service → Database/Storage = depends_on
  // Service → Service = depends_on
  // Host → Network = connected_to
  // Fallback = connected_to
}
```

- [ ] **Step 3: Implement drag-to-draw**

Use React Flow's `onConnect` and `onConnectStart`/`onConnectEnd` handlers:
- On connect: create connection via API with inferred type
- Auto-expand collapsed target cards when dragging toward them (detect proximity)
- Show connection type preview during drag

- [ ] **Step 4: Test connection drawing**

Draw a connection between two assets. Verify: line appears, correct type inferred, API call persists it, line styles correctly by origin.

- [ ] **Step 5: Commit**

```
feat(topology): add connection rendering, smart defaults, and drag-to-draw
```

---

## Phase 4: Panels & Secondary Views

### Task 14: Unsorted Inbox

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/TopologyInbox.tsx`

- [ ] **Step 1: Create inbox panel**

Right-side panel (same slot as inspector, only one visible at a time):
- Header: "Unsorted Inbox" + count badge
- List of unsorted assets, each showing:
  - Asset name, type, status dot
  - Placement suggestion (if any): "Suggest: [Zone Name]" with reason
  - Accept button (places asset in suggested zone)
  - Dismiss button (calls dismiss API)
- Drag support: assets can be dragged from inbox onto zones on the canvas
- "Auto-place all" button at bottom

- [ ] **Step 2: Wire drag-from-inbox to canvas**

Use React Flow's drop handling or HTML5 drag-and-drop. When an asset is dropped onto a zone node, call `setMembers` to add it.

- [ ] **Step 3: Test inbox**

Verify: unsorted assets appear, suggestions show, accept places asset in zone, dismiss hides asset, drag-to-zone works.

- [ ] **Step 4: Commit**

```
feat(topology): add unsorted inbox with placement suggestions
```

---

### Task 15: Inspector Panel

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/TopologyInspector.tsx`

- [ ] **Step 1: Rewrite inspector as slide-in panel**

Replace the existing float-over inspector with a right-side panel. Two modes:

**Asset mode** (when an asset is selected):
- Identity: name, type, source badges, IP, uptime
- Health gauges: CPU, RAM, disk (reuse existing gauge components)
- Zone: current zone name (or "Unsorted")
- Connections list: each with type badge, target name, origin indicator
- Children summary: count by type

**Connection mode** (when a connection is selected):
- Source → Target names
- Relationship type dropdown (editable)
- Origin badge (user/accepted/discovered)
- Label text input (editable)
- Delete button

- [ ] **Step 2: Wire selection to panel visibility**

Select asset on canvas → open inspector, close inbox. Select connection → switch to connection mode. Deselect → close panel. Click inbox toggle → close inspector, open inbox.

- [ ] **Step 3: Test inspector**

Click an asset on canvas. Verify: inspector slides in, shows correct data. Click a connection line. Verify: switches to connection mode. Edit relationship type. Verify: API call updates.

- [ ] **Step 4: Commit**

```
feat(topology): rewrite inspector as slide-in panel with asset and connection modes
```

---

### Task 16: Tree View

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/TopologyTreeView.tsx`

- [ ] **Step 1: Create tree view component**

Derived read-only outline from canvas state. Renders:

1. Zone hierarchy (from topology zones, nested by parent_zone_id)
2. Assets within each zone (from zone_members)
3. Discovered children within each asset (from contains edges / status context hierarchy)
4. "Unsorted" pseudo-zone at bottom with count

Each row: indent + chevron (if expandable) + icon + label + status dot + type badge

Zone rows colored by zone color. Asset rows default text color. Child rows muted.

Search bar at top filters the tree.

- [ ] **Step 2: Wire selection state**

Click a tree item → update shared selection state → canvas highlights the corresponding node. Switch to canvas view → selected item is centered.

- [ ] **Step 3: Add context menus**

Right-click on tree items mirrors canvas actions: Move to zone, Remove from zone, Connect to..., View details.

- [ ] **Step 4: Test tree view**

Toggle to tree view. Verify: zone hierarchy matches canvas. Expand an asset → shows discovered children. Search filters correctly. Click item → switch to canvas → item highlighted.

- [ ] **Step 5: Commit**

```
feat(topology): add derived tree view with search and context menus
```

---

## Phase 5: Interactions & Polish

### Task 17: Context Menus

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/TopologyContextMenus.tsx`

- [ ] **Step 1: Create context menu component**

Four context menus triggered by right-click on different canvas areas:

- **Canvas background**: New Zone, Fit View, Auto-layout
- **Zone header**: Rename, Recolor (color picker), Collapse/Expand, Delete
- **Asset card**: Connect to..., Move to zone (submenu listing all zones), Remove from zone, View details
- **Connection line**: Change type (submenu), Add/edit label, Delete

Use a shared `<ContextMenu>` component with `position: fixed` at cursor coordinates. Close on click outside or Escape.

- [ ] **Step 2: Wire actions to API mutations**

Each menu action calls the appropriate `useTopologyData` mutation. Zone delete shows a confirmation dialog.

- [ ] **Step 3: Commit**

```
feat(topology): add right-click context menus for canvas, zones, assets, connections
```

---

### Task 18: Connect-To Dialog

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/ConnectToDialog.tsx`

- [ ] **Step 1: Create the dialog**

Modal dialog opened via right-click "Connect to..." or a keyboard shortcut:
- Search input at top (auto-focused)
- Filterable list of all assets (from status context), showing name, type, zone
- Click an asset to select as target
- Relationship type selector with smart default pre-selected
- "Connect" button creates the connection

- [ ] **Step 2: Wire to context menu and API**

"Connect to..." context menu item opens dialog with source pre-filled. On confirm, call `createConnection`.

- [ ] **Step 3: Commit**

```
feat(topology): add Connect To dialog with searchable asset picker
```

---

### Task 19: Canvas Search

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/TopologySearch.tsx`

- [ ] **Step 1: Create search overlay**

Triggered by Cmd+F or clicking the search input in the toolbar:
- Input overlay at top of canvas
- As user types, matches asset names (fuzzy or substring)
- Results shown as a dropdown list
- Selecting a result:
  1. If asset is in a collapsed zone → expand the zone
  2. If asset is in a collapsed containment card → expand the card
  3. Pan and zoom the canvas to center on the asset
  4. Select the asset (opens inspector)
  5. Highlight with a pulsing ring animation

- [ ] **Step 2: Wire keyboard shortcut**

Cmd+F / Ctrl+F focuses the search input. Escape closes search.

- [ ] **Step 3: Commit**

```
feat(topology): add canvas search with auto-expand and pan-to
```

---

### Task 20: Undo/Redo

**Files:**
- Create: `web/console/app/[locale]/(console)/topology/useTopologyUndo.ts`

- [ ] **Step 1: Create undo/redo hook**

```typescript
// useTopologyUndo.ts
// Client-side undo stack, max 50 operations.
// Each operation stores: type, forward action, reverse action (compensating API call).
// Covers: zone moves/resizes, asset placement/removal, connection create/delete.
// Does NOT cover: renames, connection type changes.
```

Operations recorded:
- `zone_move`: stores old position → undo restores old position via `updateZone`
- `zone_resize`: stores old size → undo restores
- `zone_delete`: stores full zone data + members → undo re-creates zone and members
- `member_add`: stores zone_id + asset_id → undo calls `removeMember`
- `member_remove`: stores zone_id + asset_id + position → undo calls `setMembers` to re-add
- `connection_create`: stores connection ID → undo calls `deleteConnection`
- `connection_delete`: stores full connection data → undo calls `createConnection`

- [ ] **Step 2: Wire Ctrl+Z / Ctrl+Shift+Z**

Add keyboard event listeners. Call `undo()` / `redo()` from the hook.

- [ ] **Step 3: Commit**

```
feat(topology): add undo/redo for canvas operations
```

---

### Task 21: Keyboard Shortcuts + Final Polish

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvasPage.tsx`
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx`

- [ ] **Step 1: Wire all keyboard shortcuts**

Add `useEffect` with `keydown` listener:
- `Delete` / `Backspace` → remove selected element
- `Ctrl+A` → select all assets in focused zone
- `Escape` → deselect all, close panels
- `Space` → toggle expand/collapse on selected card
- `Ctrl+G` → create zone from multi-selection: remove selected assets from their current zones, create a new zone encompassing their bounding box, add all selected assets as members of the new zone
- Arrow keys → navigate between assets (defer to v2 if complex — free-form canvas spatial navigation is non-trivial)

Also enable React Flow's `selectionOnDrag` prop for rubber-band lasso multi-select.

**Deferred from spec:** Canvas "Paste" context menu item — copy/paste for zones/assets is deferred to a future iteration.

- [ ] **Step 2: Auto-layout button**

Implement a simple auto-layout that:
- Sorts zones by size (largest first)
- Arranges them in a grid pattern with padding
- Does not change zone contents or nesting
- Animates the transition

- [ ] **Step 3: Viewport persistence**

Wire `onMoveEnd` from React Flow to save viewport via debounced API call (500ms). On load, restore viewport from topology state.

- [ ] **Step 4: Final TypeScript check**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 5: Commit**

```
feat(topology): add keyboard shortcuts, auto-layout, and viewport persistence
```

---

## Phase 6: Cleanup

### Task 22: Delete Old Topology Code

**Files:**
- Delete: ~39 files in the old topology directory (once new system is stable)
- Modify: `web/console/app/[locale]/(console)/topology/page.tsx` — remove feature flag, make new topology the default

- [ ] **Step 1: List all old topology files that will be removed**

These are the files from the old system that are no longer needed:
- `topologyGraphModel.tsx`, `topologyTreeModel.tsx`, `topologyGraphEdges.ts`, `topologyGraphEdgeRecords.ts`, `topologyGraphLanePlan.ts`, `topologyGraphSelectors.ts`
- `topologyTreeHierarchy.ts`, `topologyTreeEdges.ts`, `topologyTreeEdgeRecords.ts`, `topologyTreeLayout.ts`, `topologyTreeNodes.tsx`
- `topologyHierarchy.ts`, `topologyPageTypes.ts`, `topologyTypes.ts`, `topologyUtils.ts`
- `TopologyPageClient.tsx`, `TopologyContentSwitch.tsx`, `TopologyControlsCard.tsx`
- `TopologyGraphPanel.tsx`, `TopologyTreePanel.tsx`, `TopologyTreeCard.tsx`, `TopologyDeepTree.tsx`
- `TopologyListView.tsx`, `TopologyListAssetRow.tsx`, `TopologyListHeader.tsx`, `TopologyListLaneSection.tsx`
- `TopologyLegend.tsx`, `CompoundNode.tsx`, `AssetCardNode.tsx`, `GroupBoxNode.tsx`, `EdgeInspector.tsx`
- `useTopologyFilters.ts`, `useTopologyListSections.ts`, `useTopologySceneData.ts`, `useTopologySelectionData.ts`, `useAssetDependencies.ts`

- [ ] **Step 2: Remove old files and feature flag**

Only do this once the new topology is confirmed stable. Remove the feature flag check from `page.tsx`. Delete old files.

- [ ] **Step 3: Verify no broken imports**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors (no other page should import from old topology files)

- [ ] **Step 4: Commit**

```
refactor(topology): remove old topology system (~39 files)
```

---

### Task 23: Documentation Updates

**Files:**
- Modify: `docs/internal/ADR.md`
- Modify: `notes/PROGRESS_LOG.md`
- Modify: `notes/TODO.md`

- [ ] **Step 1: Add ADR entry**

Add an entry to `docs/internal/ADR.md` documenting the topology redesign decision:
- Decision: Replace auto-inferred topology with user-defined zoned canvas
- Context: Current system generates topology from discovery data, doesn't match operator mental model
- Consequences: ~39 files deleted, new canvas + tree views, new Postgres tables, hybrid connection model

- [ ] **Step 2: Update PROGRESS_LOG.md**

Add entry for the topology redesign work with completion status.

- [ ] **Step 3: Update TODO.md**

Remove any topology-related TODOs that are now resolved. Add any deferred items (copy/paste, arrow key navigation, multi-topology).

- [ ] **Step 4: Commit**

```
docs: add ADR and update tracking notes for topology redesign
```

---

## Task Dependency Graph

```
Phase 1 (Backend):
  Task 1 (migration) → Task 2 (types) → Task 3 (store) → Task 4 (merge) ─┐
                                                          Task 5 (suggest) ─┤
                                                          Task 6 (seed) ────┤
                                                                            ↓
                                                          Task 7 (API handlers)

Phase 2 (Frontend Foundation):
  Task 7 → Task 8 (proxy + flag) → Task 9 (types + hooks) → Task 10 (page skeleton)

Phase 3 (Canvas Core):
  Task 10 → Task 11 (zones) → Task 12 (cards) → Task 13 (connections)

Phase 4 (Panels):
  Task 13 → Task 14 (inbox)
  Task 13 → Task 15 (inspector)
  Task 13 → Task 16 (tree view)

Phase 5 (Polish):
  Task 14-16 → Task 17 (context menus) → Task 18 (connect dialog)
  Task 14-16 → Task 19 (search)
  Task 14-16 → Task 20 (undo/redo)
  Task 17-20 → Task 21 (shortcuts + polish)

Phase 6 (Cleanup + Docs):
  Task 21 → Task 22 (delete old code) → Task 23 (docs)
```

**Parallelizable tasks within each phase:**
- Phase 1: Tasks 4, 5, 6 can run in parallel (all depend on Task 3)
- Phase 4: Tasks 14, 15, 16 can run in parallel (all depend on Task 13)
- Phase 5: Tasks 17-20 can mostly run in parallel
