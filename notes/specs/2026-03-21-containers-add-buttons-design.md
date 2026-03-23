# Design: Add Container / Add Stack from Containers Fleet Page

**Date:** 2026-03-21
**Status:** Approved
**Branch:** main

## Overview

Two action buttons in the containers page header open a shared Docker host selection modal. After picking a host, the user is navigated to the existing create container or deploy stack form for that host. Single-host setups skip the modal entirely with a toast notification.

## Buttons

Two buttons wrapped in `<div className="flex gap-2">` passed to the `PageHeader` `action` slot:
- **"Add Stack"** -- secondary/ghost variant, `Plus` icon (stacks are the less common action)
- **"Add Container"** -- primary variant, `Plus` icon (the more frequent action)

**Three visual states:**
1. **Loading** (initial data fetch in progress): Buttons show a `Spinner` and are disabled. No tooltip.
2. **No hosts** (fetch complete, zero Docker hosts): Buttons disabled with `Tip` tooltip: "No Docker hosts available".
3. **Ready** (one or more hosts): Buttons enabled, normal interaction.

## Host Selection Modal

**Trigger:** Either button opens the same modal. The modal receives a `mode: "container" | "stack"` prop that determines where to navigate after selection.

**Title:** "Select Docker Host"

**Modal width:** Pass `className="md:!max-w-2xl"` to the `Modal` component to accommodate the two-column card grid. The `!` (important) modifier ensures the override wins over the component's built-in `md:max-w-lg`, since Tailwind v4 cascade layers do not guarantee last-class-wins for same-breakpoint utilities.

**Body:** A grid of host cards (`grid grid-cols-1 md:grid-cols-2 gap-3`). Each card is a `Card` with `interactive` prop for hover lift.

### Host Card Contents

| Element | Source | Position |
|---------|--------|----------|
| Host name (clickable) | `host.agent_id` or engine hostname | Top-left, bold |
| OS/arch badge | `host.engine_os` / `host.engine_arch` | Top-right, subtle Badge |
| Engine version | `host.engine_version` | Below hostname, muted text |
| Container count | `running / total` | Inline metric with tabular-nums |
| CPU utilization | aggregate from `containers[].cpu_percent` | MiniBar + percentage |
| Memory utilization | aggregate from `containers[].memory_percent` | MiniBar + percentage |
| Existing stacks | derived from `containers[].stack_name` (unique, non-empty) | Bottom row, small Badge pills, wrapped |
| Last seen | `host.last_seen` as relative time | Bottom-right, muted |

**Data source:** The modal receives the full `HostData[]` array (the `{ host: DockerHostSummary; containers: DockerContainer[] }` tuples already in page state), not just `DockerHostSummary[]`. This provides:
- Running count: `containers.filter(c => c.state === "running").length`
- Stack names: `[...new Set(containers.map(c => c.stack_name).filter(Boolean))]`
- Aggregate CPU/mem: computed from `containers[].cpu_percent` / `containers[].memory_percent`

**Online/offline indicator:** Hosts not seen in >60s get `opacity-50` on the card. The last-seen timestamp turns into a warning: "Offline -- last seen X ago" in `text-[var(--warn)]`. Still selectable.

### Navigation

**Route ID transformation:** The create form pages live at `/nodes/{nodeId}/new-container` and `/nodes/{nodeId}/new-compose`. The `nodeId` route segment must be `docker-host-{normalized_id}`. The `DockerHostSummary` type already carries a `normalized_id` field computed by the backend -- no client-side normalization is needed.

Clicking a host card:
- Calls `router.push(\`/nodes/docker-host-${host.normalized_id}/new-container\`)` for container mode
- Calls `router.push(\`/nodes/docker-host-${host.normalized_id}/new-compose\`)` for stack mode

### Empty State

If zero Docker hosts exist, the modal body shows an `EmptyState` component:
- Icon: `Server`
- Title: "No Docker hosts connected"
- Description: "Add a device with Docker to get started."
- Action: `<Button onClick={() => router.push("/nodes")}>Go to Devices</Button>`

Links to the nodes page rather than trying to trigger the Add Device modal (which is managed by layout-level state and not accessible from a nested modal context).

## Single-Host Shortcut

When exactly one Docker host exists and data has finished loading:
- Skip the modal entirely
- Navigate directly to the create form using the same route transformation
- Show a toast via `addToast()`: `"Deploying to {hostname}"` (type: `"info"`, auto-dismiss)

## Data Flow

No new API calls needed. The containers page already fetches `fetchDockerHosts()` and per-host container data on mount with 15s refresh. The `hostData: HostData[]` array is already in component state. The modal receives it as a prop.

## Props Interface

```typescript
type DockerHostPickerModalProps = {
  open: boolean;
  onClose: () => void;
  mode: "container" | "stack";
  hostData: HostData[];
};
```

Where `HostData` is the existing type: `{ host: DockerHostSummary; containers: DockerContainer[] }`.

## New Files

| File | Purpose |
|------|---------|
| `app/components/containers/DockerHostPickerModal.tsx` | The shared host selection modal |

## Modified Files

| File | Change |
|------|--------|
| `app/[locale]/(console)/containers/page.tsx` | Add `action` prop to `PageHeader` with button wrapper, modal open/close state (`pickerMode: "container" \| "stack" \| null`), single-host shortcut logic in button click handlers |

## Edge Cases

- **Loading state:** Buttons show spinner, disabled until first fetch completes
- **Zero hosts:** Buttons disabled with tooltip "No Docker hosts available"
- **One host:** Skip modal, navigate directly, toast notification
- **Host goes offline between modal open and selection:** The create form handles this -- it already shows errors if the host action fails
- **Many hosts (10+):** The modal body scrolls with themed scrollbar. No search/filter needed at homelab scale (typically 2-5 Docker hosts)
- **Modal width:** Uses `max-w-2xl` (672px) to fit two-column card grid comfortably

## Design Decisions

1. **Shared modal for both actions** -- The host selection decision is the same regardless of container vs stack. Two modals would be redundant.
2. **Reuse existing create forms** -- The `/nodes/[id]/new-container` and `/nodes/[id]/new-compose` pages are already solid. No need to rebuild them as wizard steps.
3. **PageHeader action slot** -- Consistent with every other LabTether page (nodes, groups, schedules) that puts primary create actions in the header.
4. **Full context host cards** -- Operators need capacity (CPU/mem), existing workloads (stacks), and health (last seen) to make placement decisions.
5. **Single-host skip with toast** -- Most homelabs have one Docker host. Forcing a modal click to confirm the obvious adds friction without value.
6. **Navigate to nodes page (not Add Device modal) on empty state** -- The Add Device modal is controlled by layout-level state. Triggering it from a nested modal creates coupling. Linking to the nodes page is simpler and still gets the user where they need to go.
7. **HostData[] props (not DockerHostSummary[])** -- Running counts, stack names, and aggregate metrics all require the per-host container arrays, not just the summary.
