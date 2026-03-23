# Topology Canvas Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the topology canvas so it shows a readable infrastructure topology (5-10 host nodes with expandable children) instead of a flat dump of 98+ asset cards.

**Architecture:** Frontend-first approach. The hierarchy data already exists (`topologyHierarchy.ts`), and `ContainmentCard` already supports expand/collapse with children. The core fix is filtering child assets from the top-level canvas rendering, then fixing zone sizing/colors and connection noise. A backend auto-seed improvement makes new/reset topologies create host-based zones instead of source-based zones.

**Tech Stack:** TypeScript, React, React Flow (`@xyflow/react`), Go 1.26

**Root Cause Analysis:** 5 root causes identified: (1) zones named after data sources not hosts, (2) all 98 assets rendered as top-level nodes despite hierarchy data existing, (3) zones overflow (46/52 truenas members outside bounds), (4) all zones same color, (5) 50 connection edges create spaghetti.

---

## File Structure

| File | Change | Responsibility |
|------|--------|----------------|
| `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx` | Modify | Filter child assets from ReactFlow nodes; fix zone assetCount; collapse connections to visible nodes; auto-size zones |
| `web/console/app/[locale]/(console)/topology/ZoneNode.tsx` | Modify | Map hex colors to named palette via deterministic hash fallback |
| `internal/topology/seed.go` | Modify | Create host-based zones instead of source-based; assign varied colors; skip child assets as members |
| `internal/topology/seed_test.go` | Modify | Update tests for host-based seeding |
| `internal/topology/store.go` | Modify | Add `ClearTopology` method to Store interface |
| `internal/topology/postgres_store.go` | Modify | Implement `ClearTopology` |
| `cmd/labtether/apiv2_topology.go` | Modify | Add reset endpoint |
| `web/console/app/[locale]/(console)/topology/TopologyContextMenus.tsx` | Modify | Add "Reset Layout" option |
| `web/console/app/[locale]/(console)/topology/useTopologyData.ts` | Modify | Add `resetTopology` mutation |

---

## Phase 1: Filter Child Assets from Canvas + Fix Zone Badge (Biggest Impact)

### Task 1: Filter child assets from top-level ReactFlow rendering and fix zone assetCount

This single change reduces ~98 visible cards to ~7-10 host-level nodes. The ContainmentCard already shows children when expanded via the hierarchy data. Also fixes the zone header badge to show only the top-level member count.

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx`

**Important context:**
- `buildAssetNodes` (line ~115) already receives `hierarchy` as a parameter — use `hierarchy.childIDs` directly, no signature change needed.
- `buildZoneNodes` (line ~57) computes `memberCountByZone` from ALL members — this must also be filtered.

- [ ] **Step 1: Understand the current flow**

Read `TopologyCanvas.tsx`. The `buildAssetNodes` function (line ~130) creates a ReactFlow node for every `topology.member`. The hierarchy data (`buildHierarchy`) already identifies which assets are children via `hierarchy.childIDs: Set<string>`. The fix: skip members whose `asset_id` is in `hierarchy.childIDs`.

- [ ] **Step 2: Add childIDs filtering to buildAssetNodes**

In `TopologyCanvas.tsx`, inside `buildAssetNodes`, add a filter before the `.map()` call. The function already receives `hierarchy` so no signature change is needed:

```typescript
// Inside buildAssetNodes, after the entryByHostID loop, before the return:

// Filter out members that are children in the hierarchy —
// they appear inside their parent's ContainmentCard instead.
const topLevelMembers = topology.members.filter(
  (m) => !hierarchy.childIDs.has(m.asset_id),
);

return topLevelMembers.map((m) => {
  // ... rest of existing mapping code unchanged
```

Change `topology.members.map` to `topLevelMembers.map`.

- [ ] **Step 3: Fix zone assetCount badge**

In `buildZoneNodes` (line ~57), the `memberCountByZone` loop counts ALL members including children. After filtering, a zone with 52 truenas members might show "52" in the header but only 1-2 cards are visible. Fix: pass `childIDs` and filter the count.

Update the `buildZoneNodes` signature to accept `childIDs: Set<string>`:

```typescript
function buildZoneNodes(
  topology: TopologyState,
  callbacks: { ... },
  renamingZoneId?: string | null,
  childIDs?: Set<string>,
): Node[] {
  const memberCountByZone = new Map<string, number>();
  for (const m of topology.members) {
    if (childIDs?.has(m.asset_id)) continue; // Skip children
    memberCountByZone.set(m.zone_id, (memberCountByZone.get(m.zone_id) || 0) + 1);
  }
  // ... rest unchanged
```

Update the call site in `TopologyCanvasInner` (the `useMemo` at ~line 275) to pass `hierarchy.childIDs` as the 4th argument. Add `hierarchy.childIDs` to the dependency array (the existing eslint-disable comment covers this).

- [ ] **Step 4: Verify the filtering works**

Run the dev server (`cd web/console && npm run dev`), navigate to `/topology`, and confirm:
- Far fewer cards visible (expect ~7-15 instead of ~98)
- Host cards like OmegaNAS, DeltaServer, ContainerVM, Portainer still visible
- Individual datasets, containers, shares, disks NOT visible as top-level cards
- Zone header badges show accurate counts (not inflated by hidden children)
- Expanding a host card (click chevron) still shows its children in the ContainmentCard layers

- [ ] **Step 5: Run type check**

```bash
cd web/console && npm run -s tsc -- --noEmit
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/topology/TopologyCanvas.tsx
git commit -m "fix(topology): filter child assets from canvas — only show host-level nodes

Child assets (containers, datasets, shares, services, disks) now only
appear inside their parent's expandable ContainmentCard. Reduces ~98
visible cards to ~7-15 host-level nodes. Zone header badge counts
are also filtered to match visible nodes."
```

---

## Phase 2: Fix Zone Rendering (Colors, Sizing)

### Task 2: Fix zone colors — deterministic hash fallback for hex values

All zones currently use `#4f6bed` (a hex value). `ZoneNode.tsx` expects named colors (blue, green, purple, etc.) from `ZONE_COLORS` and falls back to blue for all. Fix: when color is not a named key, hash the zoneId to pick a deterministic color from the palette.

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/ZoneNode.tsx:7-14,34`

- [ ] **Step 1: Add resolveZoneColor function**

In `ZoneNode.tsx`, add a helper function above the component and use it:

```typescript
/** When color is not a named key (e.g. a hex value), pick deterministically from the palette. */
function resolveZoneColor(color: string, zoneId: string): typeof ZONE_COLORS[keyof typeof ZONE_COLORS] {
  if (ZONE_COLORS[color]) return ZONE_COLORS[color];
  const names = Object.keys(ZONE_COLORS);
  let hash = 0;
  for (let i = 0; i < zoneId.length; i++) hash = ((hash << 5) - hash + zoneId.charCodeAt(i)) | 0;
  return ZONE_COLORS[names[Math.abs(hash) % names.length]];
}
```

- [ ] **Step 2: Replace the color lookup in the component**

```typescript
// Before:
const colors = ZONE_COLORS[d.color] || ZONE_COLORS.blue;

// After:
const colors = resolveZoneColor(d.color, d.zoneId);
```

- [ ] **Step 3: Verify in browser**

Navigate to `/topology`. Confirm zones now have different colors instead of all blue.

- [ ] **Step 4: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/topology/ZoneNode.tsx
git commit -m "fix(topology): vary zone colors — deterministic hash fallback for hex colors"
```

### Task 3: Auto-size zones based on top-level member count

Zones are all 640x300 regardless of content. After child filtering, zones may have 1-3 members. Calculate a reasonable size.

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx:57-100` (`buildZoneNodes` function, already modified in Task 1)

**Note:** `ContainmentCard` uses `style={{ width: 260 }}` (line 49 of `ContainmentCard.tsx`). The CARD_W constant below uses 280 (260 + 20px padding between cards).

- [ ] **Step 1: Add dynamic size calculation to buildZoneNodes**

In `buildZoneNodes` (already modified in Task 1 to accept `childIDs`), add auto-sizing when zones have the seed-default dimensions:

```typescript
return topology.zones.map((z) => {
  const memberCount = memberCountByZone.get(z.id) || 0; // Already filtered in Task 1

  // Auto-size: if zone has seed-default dimensions (640x300), compute from member count
  const CARD_W = 280; // ContainmentCard width (260) + 20px gap
  const CARD_H = 56;
  const PADDING = 40;
  const HEADER = 36;
  const isDefaultSize = z.size.width === 640 && z.size.height === 300;
  const cols = Math.min(memberCount, 2);
  const rows = Math.max(1, Math.ceil(memberCount / 2));
  const autoWidth = cols * CARD_W + PADDING;
  const autoHeight = HEADER + rows * CARD_H + PADDING;
  const size = isDefaultSize
    ? { width: Math.max(300, autoWidth), height: Math.max(120, autoHeight) }
    : z.size;

  return {
    id: `zone-${z.id}`,
    // ... existing fields ...
    style: { width: size.width, height: size.height },
    // ...
  };
});
```

- [ ] **Step 2: Verify in browser**

Zones should now be reasonably sized for their content. A zone with 1 host should be compact. A zone with 3 hosts should be wider.

- [ ] **Step 3: Run type check**

```bash
cd web/console && npm run -s tsc -- --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/topology/TopologyCanvas.tsx
git commit -m "fix(topology): auto-size zones based on top-level member count"
```

---

## Phase 3: Collapse Connections to Visible Nodes

### Task 4: Only render connections between visible (top-level) nodes

50 connection edges between leaf assets (containers, datasets) create visual noise. Connections where one or both endpoints are hidden children are hidden entirely.

**Design tradeoff:** This uses a simple "hide" strategy, not "collapse to parent." Inter-host connections that exist only through child-to-child edges will be completely invisible. This is acceptable for v1 — a future enhancement could aggregate child edges up to their parent host.

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvas.tsx:219-234` (`buildConnectionEdges` function) and the `TopologyCanvasInner` component.

- [ ] **Step 1: Filter connections to visible nodes only**

Modify `buildConnectionEdges` to accept the set of visible (top-level) asset IDs and skip connections where either endpoint is not visible:

```typescript
function buildConnectionEdges(
  connections: TopologyConnection[],
  visibleAssetIDs: Set<string>,
): Edge[] {
  return connections
    .filter((conn) =>
      visibleAssetIDs.has(conn.source_asset_id) &&
      visibleAssetIDs.has(conn.target_asset_id)
    )
    .map((conn) => ({
      id: `conn-${conn.id}`,
      source: `asset-${conn.source_asset_id}`,
      target: `asset-${conn.target_asset_id}`,
      // ... rest of existing mapping unchanged
    }));
}
```

- [ ] **Step 2: Build the visible asset ID set and pass it**

In `TopologyCanvasInner`, add a `visibleAssetIDs` memo and update the `connectionEdges` memo:

```typescript
const visibleAssetIDs = useMemo(() => {
  const ids = new Set<string>();
  for (const m of topology.members) {
    if (!hierarchy.childIDs.has(m.asset_id)) ids.add(m.asset_id);
  }
  return ids;
}, [topology.members, hierarchy.childIDs]);

const connectionEdges = useMemo(
  () => buildConnectionEdges(topology.connections, visibleAssetIDs),
  [topology.connections, visibleAssetIDs],
);
```

- [ ] **Step 3: Verify in browser**

Connections should only appear between host-level nodes. No more spaghetti lines to invisible children.

- [ ] **Step 4: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/topology/TopologyCanvas.tsx
git commit -m "fix(topology): only render connections between visible top-level nodes

Hides connections where either endpoint is a child asset not visible
on the canvas. Uses hide strategy (not collapse-to-parent) — inter-host
connections via child-only edges are hidden. Future enhancement can
aggregate these up to parent hosts."
```

---

## Phase 4: Fix Unsorted Inbox Count

### Task 5: Filter children from unsorted count

The inbox shows "25 unsorted" but many may be child assets that belong inside a parent. Filter them.

**Files:**
- Modify: `web/console/app/[locale]/(console)/topology/TopologyCanvasPage.tsx:77`

- [ ] **Step 1: Verify after Phase 1 — is this needed?**

After Phase 1 filtering, visually verify the inbox. If the unsorted items are mainly orphan assets (not children of placed hosts), the count is correct and no change needed. Mark this task as N/A if so.

The `TopologyInbox` component receives only `unsortedAssetIDs: string[]` and would need hierarchy data plumbed in to filter. This adds complexity — defer unless the count is misleadingly high.

- [ ] **Step 2: If needed, filter inline**

In `TopologyCanvasPage`, if hierarchy data is available (it's computed inside `TopologyCanvas` currently), the cleanest approach is to lift `hierarchy` up or compute a filtered unsorted list in the page component. Otherwise, defer to a follow-up.

- [ ] **Step 3: Commit (if changes made)**

```bash
git add web/console/app/\[locale\]/\(console\)/topology/TopologyCanvasPage.tsx
git commit -m "fix(topology): filter child assets from unsorted inbox count"
```

---

## Phase 5: Backend — Improve Auto-Seed for New/Reset Topologies

### Task 6: Create host-based zones instead of source-based zones

The auto-seed in `internal/topology/seed.go` creates zones named after sources (truenas, portainer, docker). It should create zones per infrastructure host and only place top-level (non-child) assets as zone members.

**Files:**
- Modify: `internal/topology/seed.go`
- Modify: `internal/topology/seed_test.go`

**Important context:**
- There is NO `isInfraHost` function in the `internal/topology` package. The Go-side equivalent is `IsInfraHostAsset` in `internal/hubapi/resources/assets_groups_cascade.go`. Either import that function or reimplement a simplified host-type check in `seed.go` using the `Type` field of `AssetInfo`.
- The current `AssetInfo` struct (line 10-15 of `seed.go`) has `ID`, `Label`, `Source`, `Type`. This is sufficient for a simplified type-based check (e.g., type in {"host", "server", "hypervisor", "nas", "container-host", "hypervisor-node", "storage-controller", "connector-cluster"}).
- **3-4 existing tests will break:** `TestSeed_AssetsWithSource`, `TestSeed_Mixed`, `TestSeed_ZoneGridPositions`, and `TestSeed_MemberPositions` all assert source-based zone creation. These must be rewritten.
- Zone colors should cycle through named palette: `["blue", "green", "purple", "amber", "rose", "cyan"]`.

- [ ] **Step 1: Read the current seed logic and tests**

Read `internal/topology/seed.go` and `internal/topology/seed_test.go` to understand the current algorithm and what tests exist.

- [ ] **Step 2: Write a failing test for host-based seeding**

In `seed_test.go`, add a test:

```go
func TestAutoSeedHostBasedZones(t *testing.T) {
    // Create test assets:
    //   - 1 NAS host (type=nas, source=truenas, label=OmegaNAS) with 5 child datasets
    //   - 1 hypervisor (type=hypervisor-node, source=proxmox, label=DeltaServer) with 2 VMs
    //   - 3 orphan services (no parent host)
    // Run Seed()
    // Assert: 2 zones created (labeled "OmegaNAS", "DeltaServer"), not source-based
    // Assert: only the 2 host assets + 3 orphans are zone members (not datasets or VMs)
    // Assert: zone colors differ (not all "blue")
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/topology/ -run TestAutoSeedHostBasedZones -v
```

Expected: FAIL

- [ ] **Step 4: Modify seed logic**

Key changes to `seed.go`:
1. Add a `isInfraHostType(assetType string) bool` helper that checks against known host types: `{"host", "server", "hypervisor", "nas", "container-host", "hypervisor-node", "storage-controller", "connector-cluster"}`
2. First pass: identify infra hosts from `AssetInfo` list
3. Second pass: match non-hosts to parents using edges/group containment (if edge data is available) or source+metadata matching
4. Create one zone per infra host; remaining orphan assets go into a catch-all zone or stay unsorted
5. Cycle colors: `colors := []string{"blue", "green", "purple", "amber", "rose", "cyan"}` and assign `colors[i % len(colors)]`
6. Zone size: compute from member count instead of hardcoded 640x300

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/topology/ -run TestAutoSeedHostBasedZones -v
```

Expected: PASS

- [ ] **Step 6: Fix broken existing tests**

Run the full test suite — expect 3-4 failures in tests that assert source-based zones:

```bash
go test ./internal/topology/... -v
```

Rewrite `TestSeed_AssetsWithSource`, `TestSeed_Mixed`, `TestSeed_ZoneGridPositions`, and `TestSeed_MemberPositions` to match the new host-based behavior.

- [ ] **Step 7: Run full test suite and go vet**

```bash
go test ./internal/topology/... -v && go vet ./internal/topology/...
```

Expected: All pass

- [ ] **Step 8: Commit**

```bash
git add internal/topology/seed.go internal/topology/seed_test.go
git commit -m "fix(topology): auto-seed creates host-based zones instead of source-based

Zones are now named after infrastructure hosts (e.g., 'OmegaNAS',
'DeltaServer') instead of data sources ('truenas', 'proxmox').
Child assets are excluded from zone membership — they appear inside
their parent's ContainmentCard. Zone colors cycle through the palette.

Uses simplified isInfraHostType check based on asset type field."
```

### Task 7: Add a "Reset Topology" action for existing users

Existing users already have source-based zones. They need a way to re-seed with the improved algorithm.

**Files:**
- Modify: `internal/topology/store.go` (add `ClearTopology` method to Store interface)
- Modify: `internal/topology/postgres_store.go` (implement `ClearTopology`)
- Modify: `cmd/labtether/apiv2_topology.go` (add reset endpoint handler)
- Modify: `web/console/app/[locale]/(console)/topology/TopologyContextMenus.tsx` (add "Reset Layout" to canvas context menu)
- Modify: `web/console/app/[locale]/(console)/topology/useTopologyData.ts` (add `resetTopology` mutation)

- [ ] **Step 1: Add ClearTopology to Store interface**

In `internal/topology/store.go`, add to the `Store` interface:

```go
// ClearTopology deletes all zones, members, connections, and dismissed assets
// for the given topology, allowing a clean re-seed.
ClearTopology(ctx context.Context, topologyID string) error
```

- [ ] **Step 2: Implement ClearTopology in postgres_store.go**

```go
func (s *PostgresStore) ClearTopology(ctx context.Context, topologyID string) error {
    // Delete in dependency order: members first, then zones, connections, dismissed
    queries := []string{
        `DELETE FROM topology_zone_members WHERE zone_id IN (SELECT id FROM topology_zones WHERE topology_id = $1)`,
        `DELETE FROM topology_zones WHERE topology_id = $1`,
        `DELETE FROM topology_connections WHERE topology_id = $1`,
        `DELETE FROM topology_dismissed WHERE topology_id = $1`,
    }
    for _, q := range queries {
        if _, err := s.db.ExecContext(ctx, q, topologyID); err != nil {
            return fmt.Errorf("clear topology: %w", err)
        }
    }
    return nil
}
```

- [ ] **Step 3: Add backend reset endpoint**

In `cmd/labtether/apiv2_topology.go`, add a `POST /api/v2/topology/reset` handler that:
1. Calls `store.ClearTopology(topologyID)`
2. Re-runs `topologyAutoSeed` with the same asset data
3. Returns the fresh topology state (re-fetch via `handleV2Topology` logic)

- [ ] **Step 4: Add frontend reset mutation**

In `useTopologyData.ts`, add:

```typescript
const resetTopology = useCallback(async (): Promise<void> => {
  await mutate("/reset", "POST", {});
  await fetchTopology();
}, [mutate, fetchTopology]);
```

Return `resetTopology` from the hook.

- [ ] **Step 5: Add "Reset Layout" to canvas context menu**

In `TopologyContextMenus.tsx`, add a "Reset Layout" option to the canvas (background) context menu. Since this is destructive (deletes user-arranged zones), use `window.confirm` or an inline confirmation step before proceeding.

- [ ] **Step 6: Test end-to-end**

1. Right-click canvas background
2. Click "Reset Layout"
3. Confirm the dialog
4. Verify new host-based zones appear with proper colors and sizing
5. Verify old zones are gone

- [ ] **Step 7: Run all checks**

```bash
cd web/console && npm run -s tsc -- --noEmit
go test ./internal/topology/... -v
go vet ./...
```

- [ ] **Step 8: Commit**

```bash
git add internal/topology/store.go \
       internal/topology/postgres_store.go \
       cmd/labtether/apiv2_topology.go \
       web/console/app/\[locale\]/\(console\)/topology/TopologyContextMenus.tsx \
       web/console/app/\[locale\]/\(console\)/topology/useTopologyData.ts
git commit -m "feat(topology): add Reset Layout action to re-seed with host-based zones

Adds ClearTopology to Store interface and POST /api/v2/topology/reset
endpoint. Canvas context menu offers 'Reset Layout' with confirmation.
Destructive: replaces all user-arranged zones with fresh auto-seeded
host-based zones."
```

---

## Verification Checklist

After all tasks are complete:

- [ ] Navigate to `/topology` — see ~5-10 host nodes, not 98 cards
- [ ] Zone header badges show accurate top-level counts
- [ ] Zones have different colors
- [ ] Zone sizes match their content
- [ ] Connections only show between visible host-level nodes
- [ ] Expanding a ContainmentCard shows children (containers, datasets, etc.)
- [ ] Tree view still works and shows full hierarchy (unaffected by canvas filtering)
- [ ] Right-click canvas > "Reset Layout" re-seeds with host-based zones
- [ ] `cd web/console && npm run -s tsc -- --noEmit` passes
- [ ] `go test ./internal/topology/... -v` passes
- [ ] `go vet ./...` passes

---

## Priority Order

| Priority | Task | Impact | Effort |
|----------|------|--------|--------|
| P0 | Task 1: Filter children + fix badge | Fixes the core mess | ~15 min |
| P1 | Task 4: Collapse connections | Removes spaghetti lines | ~10 min |
| P1 | Task 2: Fix zone colors | Visual differentiation | ~10 min |
| P2 | Task 3: Auto-size zones | Better layout | ~15 min |
| P3 | Task 6: Backend host-based seeding | Better defaults for new users | ~45 min |
| P3 | Task 7: Reset topology action | Lets existing users re-seed | ~30 min |

Tasks 1-4 (frontend-only) can be done independently of Tasks 6-7 (backend). Ship frontend fixes first for immediate improvement.

**Known limitations (documented, not blockers):**
- Task 4 hides child-to-child connections entirely (no collapse-to-parent aggregation). Future enhancement.
- `ContainmentCard` has hardcoded `width: 260px`; expanded cards don't resize the ReactFlow node. Pre-existing issue.
- Tasks 1, 3, 4 all touch `TopologyCanvasInner` in `TopologyCanvas.tsx` — coordinate edits holistically to avoid churn in the same `useMemo` blocks.
