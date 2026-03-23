# Topology Redesign: Zoned Canvas with Expandable Containment

**Date:** 2026-03-21
**Status:** Design approved, pending implementation plan

## Problem

The current topology tab is a ~39-file system with 4 view modes (graph/tree/list/deep-tree), lane-based grouping, compound nodes, edge deduplication, and hierarchy inference. It auto-generates topology from discovered data and doesn't match how operators actually think about their infrastructure. The user can't express their mental model — they're stuck viewing the system's guess.

## Solution

Replace the entire topology system with a **user-defined zoned canvas**. The operator creates named, nested zones (groups), drags assets into them, and draws connections between them. Topology becomes a persisted, user-owned document — the source of truth for how infrastructure is organized — not a derived view.

## Core Principles

1. **Topology organization is user-defined; connections are hybrid.** Zones (the organizational structure) are fully user-owned. Connections combine auto-discovery with user curation — discovered relationships render automatically, the user adds/removes/overrides as needed.
2. **Auto-populate with user override.** New assets auto-land in an unsorted inbox with placement suggestions. The user accepts, rejects, or manually places them.
3. **Two hierarchies, cleanly separated.** Zones (user-defined organizational grouping) wrap containment cards (auto-discovered infrastructure hierarchy). You organize at the host level; infrastructure truth unfolds within.
4. **Smart defaults, full control.** Connection types auto-pick based on asset types but can be changed. Layout auto-arranges but can be overridden.

## Data Model

### Coordinate System
All positions are stored as **absolute canvas coordinates**. When a zone is nested inside another zone, both have absolute `{x, y}` positions. When a zone is moved, all children (sub-zones and asset positions) are translated by the same delta. When a zone is un-nested, positions remain absolute — no recalculation needed. Asset positions within zones are also absolute canvas coordinates; the zone's bounds simply define the visual containment.

### Topology Layout
One per hub, stored in Postgres. The v1 API is single-topology (no topology ID in URL paths). If multi-topology is needed later, endpoints will be versioned to include a topology ID.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `name` | TEXT | e.g. "My Homelab" |
| `viewport` | JSONB | Saved zoom/pan `{x, y, zoom}` |
| `created_at` | TIMESTAMP | |
| `updated_at` | TIMESTAMP | |

### Zone
Replaces lanes, groups, and hierarchy — one concept for organizational grouping.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `topology_id` | UUID | FK to topology_layouts |
| `parent_zone_id` | UUID | Nullable, self-referential FK for nesting |
| `label` | TEXT | User-defined name |
| `color` | TEXT | Accent color for zone border/header |
| `icon` | TEXT | Optional icon |
| `position` | JSONB | `{x, y}` absolute canvas coordinates |
| `size` | JSONB | `{width, height}` with auto-resize |
| `collapsed` | BOOLEAN | Whether nested content is collapsed |
| `sort_order` | INT | Ordering within parent |

### Zone Membership
Which assets are in which zone. An asset can only belong to one zone.

| Column | Type | Description |
|--------|------|-------------|
| `zone_id` | UUID | FK to zones |
| `asset_id` | TEXT | FK to assets (assets use TEXT primary keys) |
| `position` | JSONB | `{x, y}` absolute canvas coordinates (within parent zone bounds) |
| `sort_order` | INT | Fallback ordering |

### Topology Connection
User-defined connections. Separate from the existing `edges` table which continues to serve discovery data.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `topology_id` | UUID | FK to topology_layouts |
| `source_asset_id` | TEXT | FK to assets (any level, including children) |
| `target_asset_id` | TEXT | FK to assets |
| `relationship` | TEXT | See Relationship Type Vocabulary below |
| `user_defined` | BOOLEAN | true = user drew it, false = auto-suggested and accepted |
| `label` | TEXT | Optional user annotation |
| `deleted` | BOOLEAN | Soft-delete marker (suppresses discovered edge reappearance) |
| `created_at` | TIMESTAMP | |

### Dismissed Assets
Tracks assets the user has explicitly dismissed from the inbox.

| Column | Type | Description |
|--------|------|-------------|
| `topology_id` | UUID | FK to topology_layouts |
| `asset_id` | TEXT | FK to assets |
| `source` | TEXT | Asset source at time of dismissal (for change detection) |
| `type` | TEXT | Asset type at time of dismissal (for change detection) |
| `dismissed_at` | TIMESTAMP | When the user dismissed it |

Dismissed assets don't appear in the inbox but are still unsorted. They reappear if the user clears dismissals or if the asset's source/type changes (indicating something materially changed).

### Relationship Type Vocabulary

| Type | Meaning | Migrated from existing |
|------|---------|----------------------|
| `runs_on` | Asset executes on the target (container on host, VM on hypervisor) | Yes — `runs_on` |
| `hosted_on` | Asset is hosted by the target (weaker than runs_on, e.g. DNS hosted on provider) | Yes — `hosted_on` |
| `depends_on` | Asset requires the target to function | Yes — `depends_on` |
| `provides_to` | Asset provides a service/resource to the target | Yes — `provides_to` |
| `connected_to` | Generic network/logical connection | Yes — `connected_to` |
| `peer_of` | Bidirectional peer relationship (cluster members, HA pairs) | Yes — `peer_of` |

**Removed from topology connections:** `contains` — containment is now expressed through zones (organizational) and expandable cards (infrastructure). The `contains` edge type in the existing `edges` table is used by the containment card renderer to build the infrastructure hierarchy, but is not exposed as a user-drawable connection type.

**Unique constraint:** `UNIQUE(topology_id, source_asset_id, target_asset_id, relationship)` — the same asset pair CAN have multiple connections with different types (e.g., `runs_on` and `depends_on`). Duplicate connections with the same type are prevented.

### Unsorted Inbox
Computed as: all assets NOT in any `zone_members` row AND NOT in `dismissed_assets`. Placement suggestions computed at query time from discovery data.

## Views

### Canvas (Primary)
Infinite pan/zoom canvas built on React Flow (same library as today, used differently).

**Canvas elements:**
- **Zones** — bordered, labeled rectangles with colored headers. Draggable, resizable, nestable. Zones within zones for hierarchy.
- **Asset cards** — expandable containment cards within zones. Show host identity + status when collapsed, full containment tree when expanded.
- **Connection lines** — typed, colored lines between any assets (top-level or nested children). Auto-routed with handle-side detection.
- **Minimap** — corner overview for orientation on large canvases.
- **Dot grid** — background grid for spatial orientation.

**Toolbar:**
- Canvas / Tree toggle (two-segment)
- New Zone button
- Connect mode toggle
- Fit View
- Auto-layout (re-arranges zones on the canvas to eliminate overlaps and optimize spacing; does not change zone contents or nesting)
- Search input (Cmd+F / Ctrl+F also opens it)
- Inbox toggle with unsorted count badge

**Panel layout:** Inspector and inbox both dock to the right edge. Only one is visible at a time — selecting an asset opens inspector and collapses inbox; clicking the inbox toggle opens inbox and closes inspector. Deselecting closes inspector.

### Tree (Secondary)
Auto-generated read-only outline derived from canvas state. Shows both hierarchies merged:

```
Proxmox Cluster              ← zone (user-defined)
  Production VMs              ← zone (user-defined)
    proxmox-node              ← asset (placed by user)
      ubuntu-prod             ← VM (discovered, Proxmox)
        nginx                 ← container (discovered, Docker)
        postgres              ← container (discovered, Docker)
        redis                 ← container (discovered, Docker)
      db-server               ← VM (discovered)
      app-server              ← VM (discovered)
Network                       ← zone (user-defined)
  core-switch                 ← asset (placed by user)
  router                      ← asset
Unsorted                      ← pseudo-zone
  new-device                  ← not yet placed
```

**Tree interactions:**
- Expand/collapse zones and assets
- Click to select (shared selection state with canvas)
- Search/filter bar at top
- Right-click context menus mirror canvas actions (Move to zone, Remove from zone, Connect to..., View details)
- No drag-and-drop — spatial editing happens on canvas

### Views Removed
- List view (subsumed by tree)
- Deep-tree view (redundant)
- Graph view (canvas replaces it)
- Lane grouping selector (category/group/source/type) — replaced by zones
- Density controls — zones auto-size
- Relationship filter — you see what you've placed

## Expandable Containment Cards

Asset cards on the canvas are expandable trees showing the full infrastructure hierarchy discovered beneath each host. The user only places top-level hosts into zones — everything below auto-populates from discovery data.

### Containment Layers by Source

| Source | Layer Types |
|--------|------------|
| **Proxmox** | Virtual Machines, LXC Containers, Storage Pools |
| **TrueNAS** | ZFS Pools → Datasets → Shares/Snapshots, Jails, Disks |
| **Docker** | Compose Stacks → Containers, Standalone Containers |
| **Portainer** | Stacks → Containers, Endpoints |
| **Home Assistant** | Integrations → Entities, Automations, Add-ons |
| **PBS** | Datastores → Backup Groups |
| **Agent** | Services (systemd), Docker discovery, Interfaces |
| **Network/Manual** | Leaf nodes or shallow (interfaces) |

### Card States

**Collapsed (default on canvas):**
Single line: chevron + status dot + icon + name + type badge + summary badge ("4 VMs · 11 CT")

**Expanded:**
Shows containment layers with source badges. Each layer is a labeled, collapsible section. Children render as compact rows with status dot, name, type, and port info.

**Leaf (no children):**
No chevron, no badge. Simple single-line card.

### Source Deduplication
The same asset often appears from multiple sources (Proxmox VM + Agent = same machine). Cards merge sources and show multiple source badges. Richest data wins for display (Agent provides CPU/RAM, Proxmox provides VM config).

### Depth Control
Default: show 1 level of children (direct children only). User can expand deeper on demand per card. For sources with potentially hundreds of children (HA entities, TrueNAS snapshots), cap at 10 visible with a "+N more" overflow that expands on click.

## Connections

### Three Creation Methods

1. **Discovered (automatic)** — connections from the existing edges/dependencies system render automatically when both endpoints are visible. Covers 80-90% of connections. Zero user effort.

2. **Visual drag-to-draw (any level)** — hover any asset (top-level or nested child when expanded) to see connection handles. Click handle, drag to target. If target is in a collapsed card, auto-expand that card to reveal children. Works everywhere the user can see a handle.

3. **Right-click "Connect to..." (shortcut)** — right-click any asset → "Connect to..." → searchable dropdown of all assets → pick target → auto-types relationship. Fast path for when the target isn't visible or is deep in another card.

All three look identical once created — a typed, colored line on the canvas.

### Smart Default Types

| Source → Target | Auto-type |
|----------------|-----------|
| Container/VM → Host | `runs_on` |
| Service → Database/Storage | `depends_on` |
| Service → Service | `depends_on` |
| Host → Network device | `connected_to` |
| Anything → Anything (fallback) | `connected_to` |

User can change the type via click on the connection line → popover.

### Connection Rendering

- **Both endpoints visible (expanded cards):** Line connects directly to the child rows.
- **One or both endpoints collapsed:** Line connects to the card boundary. Badge on the card shows "N connections inside".
- **Cross-zone connections:** Lines route between zone boundaries.
- **Connection re-routing:** Anchor points update smoothly on card expand/collapse transitions.

### Connection Resolution (topology_connections vs asset_edges)

Two tables contain relationship data:
- `asset_edges` — populated by discovery (agents, integrations). Continuously updated. Never modified by the topology UI.
- `topology_connections` — populated by user actions (drawing, accepting suggestions). Modified via topology API.

**`GET /api/v2/topology` returns a pre-merged view.** The backend merges both tables into a single `connections` array in the response. Each connection has an `origin` field:
- `"discovered"` — exists only in `asset_edges` (auto-rendered, lighter/dashed style)
- `"user"` — exists in `topology_connections` with `user_defined: true`
- `"accepted"` — exists in `topology_connections` with `user_defined: false` (user accepted a discovered suggestion)

**Conflict resolution:** If the same `(source, target, relationship)` tuple exists in both tables, the `topology_connections` entry wins. If a user deletes a connection that also exists as a discovered edge, a soft-delete marker is stored in `topology_connections` so the discovered edge doesn't reappear.

**Accepting a discovered connection** copies it from `asset_edges` into `topology_connections` with `user_defined: false`. This persists the user's intent — the connection survives even if the discovered edge is removed in a future sync.

### Connection Constraints
Unique constraint per `(source_asset_id, target_asset_id, relationship)` within a topology. The same pair CAN have multiple connections with different relationship types. Enforced at API level.

## Unsorted Inbox

Collapsible sidebar panel on the right edge of the canvas.

**Behavior:**
- New discovered assets that aren't in any zone auto-land here
- Badge count on the inbox toggle shows unsorted count
- Each inbox item shows the asset with placement suggestions

**Placement Suggestion Logic:**
1. Parent host already in a zone? → suggest same zone
2. Same source as existing zone members? → suggest that zone
3. Same type as existing zone members? → suggest that zone
4. No match? → no suggestion, manual drag only

**Actions per item:**
- Accept suggestion (places asset in suggested zone)
- Dismiss (hides from inbox, persisted in `dismissed_assets` table, reappears if asset materially changes)
- Drag to a zone manually

**Bulk action:** "Auto-place all" attempts to place every unsorted asset based on suggestion logic.

## Inspector Panel

Slides in from the right when an asset or connection is selected. Replaces the current float-over inspector.

**Asset inspector shows:**
- Identity: name, type, source badges, IP, uptime
- Health: CPU, RAM, disk usage
- Zone membership
- Connections list (with type badges)
- Discovered children summary

**Connection inspector shows:**
- Source and target asset names
- Relationship type (editable)
- Origin (user-defined vs discovered)
- Label (editable)
- Delete action

## Canvas Interactions

### Zone Operations
- **Create:** Right-click canvas → "New Zone" or toolbar button
- **Rename:** Click header text to edit inline
- **Recolor:** Color picker in zone header
- **Reposition:** Drag header
- **Resize:** Drag edges (auto-resize also kicks in as content changes)
- **Nest:** Drag zone onto another zone → becomes child
- **Un-nest:** Drag zone out of parent
- **Collapse/Expand:** Chevron in header. Collapsed shows summary pill: "3 assets, 2 sub-zones"
- **Delete:** Right-click → Delete. Assets move to parent zone (or Unsorted if top-level). Sub-zones become children of parent.

### Asset Operations
- **Place:** Drag from inbox into zone
- **Move:** Drag between zones
- **Multi-select:** Shift+click or lasso → drag batch
- **Snap:** Assets snap to grid within zones
- **Expand/Collapse:** Click chevron to show/hide containment tree
- **Remove:** Right-click → "Remove from zone" (goes back to Unsorted)

### Connection Drawing
- **Start:** Hover asset → handles appear (dots on edges)
- **Draw:** Click handle, drag to target
- **Auto-expand:** Dragging toward a collapsed card auto-expands it
- **Edit:** Click connection line → popover (change type, add label)
- **Delete:** Select + Delete key, or right-click → Delete

### Canvas Navigation
- **Pan:** Click and drag background, or scroll
- **Zoom:** Scroll wheel or pinch
- **Fit View:** Toolbar button (zooms to fit all content)
- **Search:** Cmd+F → find asset by name → auto-expand parents, pan to center, highlight
- **Viewport persistence:** Saved on debounce (500ms after last pan/zoom)

### Right-Click Context Menus
- **Canvas background:** New Zone, Paste, Fit View, Auto-layout
- **Zone header:** Rename, Recolor, Collapse/Expand, Delete
- **Asset card:** Connect to..., Move to zone (submenu), Remove from zone, View details
- **Connection line:** Change type, Add label, Delete

### Keyboard Shortcuts
All `Ctrl` shortcuts map to `Cmd` on macOS (standard platform convention).

- `Ctrl+Z` / `Ctrl+Shift+Z` — Undo / Redo
- `Delete` / `Backspace` — Remove selected element
- `Ctrl+A` — Select all in focused zone
- `Escape` — Deselect all
- `Space` — Toggle expand/collapse on selected card
- `Ctrl+G` — Create zone from selection
- `Ctrl+F` — Focus search input
- Arrow keys — Navigate between assets

**Undo/Redo scope:** Client-side stack, max 50 operations, cleared on page reload. Covers: zone moves/resizes, asset placement/removal, connection creation/deletion. Undo of server-persisted operations issues a compensating API call (e.g., undoing a zone delete re-creates it). Zone renames and connection type changes are NOT undoable (too granular, low cost to redo manually).

## API Design

New v2 topology endpoints under `/api/v2/topology/`. Existing edge/dependency endpoints remain untouched.

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `GET` | `/api/v2/topology` | Full state (zones, connections, unsorted, viewport) |
| `POST` | `/api/v2/topology/zones` | Create zone |
| `PUT` | `/api/v2/topology/zones/{id}` | Update zone (rename, recolor, reposition, resize, collapse) |
| `DELETE` | `/api/v2/topology/zones/{id}` | Delete zone (assets move to parent or unsorted) |
| `PUT` | `/api/v2/topology/zones/{id}/members` | Add/remove/reorder assets in zone |
| `PUT` | `/api/v2/topology/zones/reorder` | Batch reorder/nest zones |
| `POST` | `/api/v2/topology/connections` | Create connection |
| `PUT` | `/api/v2/topology/connections/{id}` | Update connection (change type, label) |
| `DELETE` | `/api/v2/topology/connections/{id}` | Delete connection |
| `PUT` | `/api/v2/topology/viewport` | Save viewport state (debounced) |
| `GET` | `/api/v2/topology/unsorted` | Assets not in any zone + placement suggestions |
| `POST` | `/api/v2/topology/auto-place` | Bulk auto-place unsorted assets |

## Migration Strategy

### First Load Auto-Seed
The first time a user opens the new topology, the system generates initial zones from:
- Existing group assignments → zones named after groups
- Source categories → fallback zones by source type
- Current hierarchy data → assets placed in generated zones

This gives an immediate starting point, not a blank canvas.

### Edge Migration
All current explicit edges are pre-loaded as connections with `user_defined: false`. They appear as lighter/dashed lines. User can accept (solidifies) or delete.

### Backend: Additive Only
New Postgres tables: `topology_layouts`, `topology_zones`, `zone_members`, `topology_connections`. Existing `edges` and `dependencies` tables untouched — they continue feeding discovery and the inbox suggester.

### Frontend: Clean Replacement
The current 35-file topology system is too intertwined to incrementally migrate. Build new canvas + tree from scratch, ship behind a feature flag, delete old code once stable.

### Feature Flag
Ship behind a feature flag for toggling between old and new topology during development.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| **Empty state (no assets)** | Canvas with centered prompt: "Add your first device to get started" |
| **Asset deleted from hub** | Disappears from zone. Connections to it soft-deleted (orphaned) |
| **Zone deleted** | Assets move to parent zone or Unsorted. Sub-zones re-parent |
| **Asset in different zone than its host** | Valid. Organizational model doesn't have to match physical hierarchy |
| **Large topology (100+ assets)** | Zones collapse to summary pills. Minimap essential. React Flow handles virtualization |
| **Concurrent editing** | Not needed now (single operator). API design supports future websocket sync |
| **Offline/disconnected assets** | Stay in zone with red status dot. Topology = intended infrastructure, not just alive |
| **Duplicate connections** | Prevented at API level. One per source+target+type triple |
| **Connection to collapsed card child** | Line routes to card boundary with badge |
| **Very deep containment (TrueNAS 4 levels)** | Default 1 level, expand deeper on demand. Cap at 10 visible children |
| **HA with hundreds of entities** | "+N more" overflow, expand on click |
| **Drag-to-connect to collapsed card** | Auto-expand the target card to reveal children |

## What We're Killing

| Current | Replacement |
|---------|-------------|
| 4 view modes (graph/tree/list/deep-tree) | 2 views (canvas + tree) |
| Lane grouping (category/group/source/type) | User-defined zones |
| Compound nodes | Expandable containment cards |
| Auto-inferred hierarchy | User-placed hosts with discovered children |
| Float-over inspectors | Slide-in inspector panel |
| Density controls | Auto-sizing zones |
| Relationship filter (all/explicit/inferred) | All connections visible, user-curated |
| Edge deduplication pipeline | Unique per source+target+type, enforced at API |
| ~39 topology source files | New implementation (~15-20 files estimated) |
