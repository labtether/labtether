# Portainer Fleet Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show Portainer-managed endpoints alongside Docker hosts on the containers fleet page, with source badges and normalized container data.

**Architecture:** New Go backend endpoint lists all Portainer endpoints as summaries. Frontend fetches both Docker and Portainer hosts, normalizes Portainer containers to the existing `DockerContainer` shape, and merges them into the fleet view with source badges. Portainer hosts appear in the host picker modal but are marked as "deploy not yet supported."

**Tech Stack:** Go (backend API), Next.js App Router (frontend), TypeScript, Tailwind v4 CSS.

**Spec:** `notes/specs/2026-03-21-containers-add-buttons-design.md` (extended)

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/hubapi/portainer/portainer_api_endpoints.go` | Create | Go handler: list all Portainer endpoints as summaries |
| `internal/hubapi/portainer/portainer_api_handlers.go` | Modify | Register new endpoints route in dispatcher |
| `web/console/lib/docker.ts` | Modify | Add `source` field to types, add Portainer fetch + normalize functions |
| `web/console/app/[locale]/(console)/containers/page.tsx` | Modify | Dual-fetch Docker + Portainer, merge into unified view |
| `web/console/app/components/containers/ContainerHostCard.tsx` | Modify | Add source badge (Docker/Portainer) |
| `web/console/app/components/containers/ContainerFleetOverview.tsx` | Modify | Add source badge in top-N tables |
| `web/console/app/components/containers/DockerHostPickerModal.tsx` | Modify | Show Portainer hosts with "deploy not supported" indicator |

---

### Task 1: Backend — Portainer Endpoints Listing API

**Files:**
- Create: `internal/hubapi/portainer/portainer_api_endpoints.go`
- Modify: `internal/hubapi/portainer/portainer_api_handlers.go`

- [ ] **Step 1: Create the endpoints listing handler**

Create `internal/hubapi/portainer/portainer_api_endpoints.go`:

```go
package portainer

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// PortainerEndpointSummary mirrors DockerHostSummary shape for fleet page consumption.
type PortainerEndpointSummary struct {
	AssetID          string    `json:"asset_id"`
	EndpointID       string    `json:"endpoint_id"`
	Name             string    `json:"name"`
	NormalizedID     string    `json:"normalized_id"`
	URL              string    `json:"url"`
	PortainerVersion string    `json:"portainer_version"`
	EngineOS         string    `json:"engine_os"`
	EngineArch       string    `json:"engine_arch"`
	ContainerCount   int       `json:"container_count"`
	StackCount       int       `json:"stack_count"`
	ImageCount       int       `json:"image_count"`
	LastSeen         time.Time `json:"last_seen"`
	Source           string    `json:"source"` // always "portainer"
}

func (d *Deps) HandlePortainerEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list assets"})
		return
	}

	// Collect container-host assets from portainer source
	var endpoints []PortainerEndpointSummary
	// Also count containers/stacks per endpoint
	containerCounts := map[string]int{}
	stackCounts := map[string]int{}

	for _, a := range allAssets {
		if a.Source != "portainer" {
			continue
		}
		epID := a.Metadata["endpoint_id"]
		switch a.Type {
		case "container-host":
			normalizedID := strings.ToLower(strings.ReplaceAll(a.Metadata["name"], " ", "-"))
			endpoints = append(endpoints, PortainerEndpointSummary{
				AssetID:          a.ID,
				EndpointID:       epID,
				Name:             a.Metadata["name"],
				NormalizedID:     normalizedID,
				URL:              a.Metadata["url"],
				PortainerVersion: a.Metadata["portainer_version"],
				EngineOS:         "linux",
				EngineArch:       "",
				LastSeen:         a.LastSeen,
				Source:           "portainer",
			})
		case "container":
			containerCounts[epID]++
		case "stack":
			stackCounts[epID]++
		}
	}

	// Merge counts into endpoint summaries
	for i := range endpoints {
		endpoints[i].ContainerCount = containerCounts[endpoints[i].EndpointID]
		endpoints[i].StackCount = stackCounts[endpoints[i].EndpointID]
	}

	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Name < endpoints[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]any{"endpoints": endpoints})
}
```

Note: `writeJSON` should already exist in the package. Check `portainer_api_handlers.go` for the existing JSON response helper — it may be `WritePortainerJSON` or a local helper. Use whatever the file already uses.

- [ ] **Step 2: Register the route in the dispatcher**

In `internal/hubapi/portainer/portainer_api_handlers.go`, the `HandlePortainerAssets` function dispatches based on path segments. Add a check at the top of the dispatcher (before the per-asset routing) for the `/endpoints` path:

```go
// At the top of HandlePortainerAssets, before parsing assetID:
if pathTail == "endpoints" || pathTail == "endpoints/" {
    d.HandlePortainerEndpoints(w, r)
    return
}
```

Find the exact location by reading the dispatcher function. The path parsing typically strips a prefix and routes based on the next segment.

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/michael/Development/LabTether/hub && go vet ./internal/hubapi/portainer/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/hubapi/portainer/portainer_api_endpoints.go internal/hubapi/portainer/portainer_api_handlers.go
git commit -m "feat(portainer): add endpoints listing API for fleet page

New GET /portainer/endpoints route lists all Portainer-managed endpoints
as summaries with container/stack counts, mirroring the Docker hosts
listing shape for unified fleet consumption."
```

---

### Task 2: Frontend — Portainer Data Fetching and Normalization

**Files:**
- Modify: `web/console/lib/docker.ts`

- [ ] **Step 1: Add source field to DockerHostSummary**

Add an optional `source` field to `DockerHostSummary`:

```typescript
export interface DockerHostSummary {
  agent_id: string;
  normalized_id: string;
  engine_version: string;
  engine_os: string;
  engine_arch: string;
  container_count: number;
  image_count: number;
  stack_count: number;
  last_seen: string;
  source?: "docker" | "portainer"; // added for fleet unification
}
```

- [ ] **Step 2: Add Portainer endpoint fetching function**

At the bottom of `lib/docker.ts`, add:

```typescript
// ── Portainer fleet integration ──

interface PortainerEndpointSummary {
  asset_id: string;
  endpoint_id: string;
  name: string;
  normalized_id: string;
  url: string;
  portainer_version: string;
  engine_os: string;
  engine_arch: string;
  container_count: number;
  stack_count: number;
  image_count: number;
  last_seen: string;
  source: string;
}

/** Fetch all Portainer endpoints as DockerHostSummary-compatible objects. */
export async function fetchPortainerEndpoints(): Promise<DockerHostSummary[]> {
  try {
    const res = await fetch("/api/portainer/endpoints", { cache: "no-store" });
    if (!res.ok) return [];
    const json = await res.json();
    const endpoints: PortainerEndpointSummary[] = json?.endpoints ?? [];
    return endpoints.map((ep): DockerHostSummary => ({
      agent_id: ep.asset_id,
      normalized_id: ep.normalized_id,
      engine_version: ep.portainer_version || "Portainer",
      engine_os: ep.engine_os || "linux",
      engine_arch: ep.engine_arch || "",
      container_count: ep.container_count,
      image_count: ep.image_count,
      stack_count: ep.stack_count,
      last_seen: ep.last_seen,
      source: "portainer",
    }));
  } catch {
    return [];
  }
}

interface PortainerRawContainer {
  Id: string;
  Names: string[];
  Image: string;
  State: string;
  Status: string;
  Ports?: Array<{ IP?: string; PrivatePort: number; PublicPort?: number; Type: string }>;
  Labels?: Record<string, string>;
  Created?: number;
}

function formatPortainerPorts(ports?: PortainerRawContainer["Ports"]): string {
  if (!ports || ports.length === 0) return "";
  return ports
    .filter((p) => p.PublicPort)
    .map((p) => `${p.PublicPort}:${p.PrivatePort}/${p.Type}`)
    .join(", ");
}

/** Fetch containers from a Portainer endpoint and normalize to DockerContainer shape. */
export async function fetchPortainerContainers(assetId: string): Promise<DockerContainer[]> {
  try {
    const res = await fetch(`/api/portainer/assets/${encodeURIComponent(assetId)}/containers`, {
      cache: "no-store",
    });
    if (!res.ok) return [];
    const json = await res.json();
    const raw: PortainerRawContainer[] = json?.data ?? [];
    return raw.map((c): DockerContainer => ({
      id: c.Id,
      name: (c.Names?.[0] ?? "").replace(/^\//, ""),
      image: c.Image,
      state: c.State?.toLowerCase() ?? "unknown",
      status: c.Status ?? "",
      created: c.Created ? new Date(c.Created * 1000).toISOString() : "",
      ports: formatPortainerPorts(c.Ports),
      stack_name: c.Labels?.["com.docker.compose.project"] ?? "",
      labels: c.Labels ?? {},
      // Portainer doesn't provide live CPU/memory metrics
      cpu_percent: undefined,
      memory_percent: undefined,
      memory_bytes: undefined,
      memory_limit: undefined,
    }));
  } catch {
    return [];
  }
}
```

- [ ] **Step 3: Update normalizeDockerHostSummary to preserve source**

Find the `normalizeDockerHostSummary` function and ensure it preserves the `source` field:

```typescript
// In the existing normalizeDockerHostSummary function, add:
source: raw.source ?? "docker",
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/console/lib/docker.ts
git commit -m "feat(docker): add Portainer endpoint fetching and container normalization

Adds fetchPortainerEndpoints() and fetchPortainerContainers() that
normalize Portainer API responses to DockerHostSummary and DockerContainer
shapes. Adds optional source field to DockerHostSummary for fleet unification."
```

---

### Task 3: Frontend — Merge Portainer into Containers Fleet Page

**Files:**
- Modify: `web/console/app/[locale]/(console)/containers/page.tsx`

- [ ] **Step 1: Import the new Portainer fetch functions**

Add to the existing docker imports:

```typescript
import {
  fetchDockerHosts,
  fetchDockerContainers,
  fetchPortainerEndpoints,
  fetchPortainerContainers,
  type DockerHostSummary,
  type DockerContainer,
} from "../../../../lib/docker";
```

- [ ] **Step 2: Update the load function to dual-fetch**

Replace the existing `load` callback body. After fetching Docker hosts, also fetch Portainer endpoints and their containers, then merge:

```typescript
  const load = useCallback(async () => {
    try {
      // Fetch Docker and Portainer hosts in parallel
      const [dockerHosts, portainerEndpoints] = await Promise.all([
        fetchDockerHosts(),
        fetchPortainerEndpoints(),
      ]);

      // Tag Docker hosts with source
      const taggedDockerHosts = dockerHosts.map((h) => ({ ...h, source: "docker" as const }));

      // Fetch containers for all hosts in parallel
      const allHosts = [...taggedDockerHosts, ...portainerEndpoints];
      const results = await Promise.allSettled(
        allHosts.map(async (host) => {
          const containers = host.source === "portainer"
            ? await fetchPortainerContainers(host.agent_id)
            : await fetchDockerContainers(host.agent_id);
          return { host, containers };
        })
      );

      const loaded: HostData[] = [];
      for (const result of results) {
        if (result.status === "fulfilled") {
          loaded.push(result.value);
        }
      }
      setHostData(loaded);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load container fleet data");
    } finally {
      setLoading(false);
    }
  }, []);
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add 'web/console/app/[locale]/(console)/containers/page.tsx'
git commit -m "feat(containers): merge Portainer endpoints into fleet page data

Dual-fetches Docker hosts and Portainer endpoints in parallel, then
fetches containers for all hosts. Portainer containers are normalized
to the same DockerContainer shape for unified fleet observability."
```

---

### Task 4: Frontend — Source Badges and Modal Awareness

**Files:**
- Modify: `web/console/app/components/containers/ContainerHostCard.tsx`
- Modify: `web/console/app/components/containers/ContainerFleetOverview.tsx`
- Modify: `web/console/app/components/containers/DockerHostPickerModal.tsx`

- [ ] **Step 1: Add source badge to ContainerHostCard**

In `ContainerHostCard.tsx`, add a source pill next to the engine version in the header. Read the file first, then find the header section where `host.engine_version` is displayed. Add a badge:

```tsx
{/* Next to engine version text, add: */}
<span className={`rounded-md px-1.5 py-0.5 text-[10px] font-medium ${
  host.source === "portainer"
    ? "bg-[var(--accent-subtle)] text-[var(--accent-text)]"
    : "bg-[var(--surface)] text-[var(--muted)]"
}`}>
  {host.source === "portainer" ? "Portainer" : "Docker"}
</span>
```

- [ ] **Step 2: Add source badge to ContainerFleetOverview top-N tables**

In `ContainerFleetOverview.tsx`, in the top-10 tables where host names are shown, add the same source pill next to the host link. Read the file to find the exact location.

- [ ] **Step 3: Update DockerHostPickerModal for Portainer hosts**

In `DockerHostPickerModal.tsx`, Portainer hosts should appear but indicate "deploy not yet supported":

In the `HostCard` component, add a visual indicator:

```tsx
{/* After the last-seen row, if host is Portainer: */}
{host.source === "portainer" && (
  <p className="text-[10px] text-[var(--muted)] italic">
    Deploy via Portainer not yet supported
  </p>
)}
```

In the `handleSelect` function, skip navigation for Portainer hosts:

```tsx
const handleSelect = (host: DockerHostSummary) => {
  if (host.source === "portainer") {
    // Portainer create flows not implemented yet
    return;
  }
  const nodeId = `docker-host-${host.normalized_id}`;
  // ... existing navigation
};
```

Update the card to show a muted/disabled style for Portainer in the picker:

```tsx
<Card
  interactive={host.source !== "portainer"}
  className={`${host.source === "portainer" ? "opacity-60 cursor-default" : "cursor-pointer"} ${isOffline ? "opacity-50" : ""}`}
>
```

Note: The `HostCard` in the picker needs the `host` prop passed to it so it can check `source`. Currently it receives individual fields — check the current props and add `source` if needed.

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 5: Manual verification**

Open `http://localhost:3000/containers` and verify:
1. Portainer endpoints appear alongside Docker hosts (if any are configured)
2. Source badges show "Docker" or "Portainer" on each host card
3. Top-N tables show source badges
4. Host picker modal shows Portainer hosts with "deploy not yet supported" text, dimmed and non-clickable
5. Docker hosts in the picker remain fully functional

- [ ] **Step 6: Commit**

```bash
git add web/console/app/components/containers/ContainerHostCard.tsx \
  web/console/app/components/containers/ContainerFleetOverview.tsx \
  web/console/app/components/containers/DockerHostPickerModal.tsx
git commit -m "feat(containers): add source badges and Portainer modal awareness

Shows Docker/Portainer source badges on host cards and fleet overview.
Portainer hosts appear in the host picker modal but are dimmed with
'deploy not yet supported' indicator until create flows are implemented."
```

---

## Verification Checklist

After all tasks:
- [ ] Go compiles (`go vet ./...`)
- [ ] TypeScript compiles (`npx tsc --noEmit`)
- [ ] Docker hosts still appear on fleet page (no regression)
- [ ] Portainer endpoints appear on fleet page (if configured)
- [ ] Source badges visible on host cards and fleet overview
- [ ] Host picker shows both sources, Portainer dimmed
- [ ] Filters (host/state/stack) work across both sources
- [ ] Fleet overview metrics aggregate both sources
