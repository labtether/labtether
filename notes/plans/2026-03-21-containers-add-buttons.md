# Containers Add Container / Add Stack Buttons — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add "Add Container" and "Add Stack" buttons to the containers fleet page, with a Docker host selection modal that navigates to the existing create forms.

**Architecture:** A single new component (`DockerHostPickerModal`) renders host cards with capacity metrics. The containers page adds two header buttons that open the modal in container/stack mode. Single-host setups skip the modal and navigate directly with a toast. All data is already in page state — no new API calls.

**Tech Stack:** Next.js App Router, React, Radix Dialog (via Modal), Tailwind v4 CSS, existing `lib/docker.ts` client.

**Spec:** `notes/specs/2026-03-21-containers-add-buttons-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `web/console/app/components/containers/DockerHostPickerModal.tsx` | Create | Host selection modal with capacity cards |
| `web/console/app/[locale]/(console)/containers/page.tsx` | Modify | Add header buttons, modal state, single-host shortcut |

---

### Task 1: Create DockerHostPickerModal Component

**Files:**
- Create: `web/console/app/components/containers/DockerHostPickerModal.tsx`

- [ ] **Step 1: Create the component file**

```tsx
// web/console/app/components/containers/DockerHostPickerModal.tsx
"use client";

import { useRouter } from "../../../../i18n/navigation";
import { Modal } from "../ui/Modal";
import { Card } from "../ui/Card";
import { MiniBar } from "../ui/MiniBar";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/Button";
import { Server } from "lucide-react";
import type { DockerHostSummary, DockerContainer } from "../../../lib/docker";

type HostData = {
  host: DockerHostSummary;
  containers: DockerContainer[];
};

type Props = {
  open: boolean;
  onClose: () => void;
  mode: "container" | "stack";
  hostData: HostData[];
};

function HostCard({
  host,
  containers,
  onSelect,
}: {
  host: DockerHostSummary;
  containers: DockerContainer[];
  onSelect: () => void;
}) {
  const running = containers.filter((c) => c.state.toLowerCase() === "running").length;
  const total = containers.length;

  const avgCpu =
    running > 0
      ? containers.reduce((sum, c) => sum + (c.cpu_percent ?? 0), 0) / running
      : 0;
  const avgMem =
    running > 0
      ? containers.reduce((sum, c) => sum + (c.memory_percent ?? 0), 0) / running
      : 0;

  const stacks = [...new Set(containers.map((c) => c.stack_name).filter(Boolean))];

  const lastSeen = host.last_seen ? new Date(host.last_seen) : null;
  const secsAgo = lastSeen ? (Date.now() - lastSeen.getTime()) / 1000 : Infinity;
  const isOffline = secsAgo > 60;

  const relativeTime = lastSeen
    ? secsAgo < 60
      ? "just now"
      : secsAgo < 3600
        ? `${Math.floor(secsAgo / 60)}m ago`
        : `${Math.floor(secsAgo / 3600)}h ago`
    : "unknown";

  return (
    <Card
      interactive
      className={`cursor-pointer ${isOffline ? "opacity-50" : ""}`}
    >
      <button
        type="button"
        className="w-full text-left space-y-2"
        onClick={onSelect}
      >
        {/* Row 1: hostname + OS badge */}
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm font-semibold text-[var(--text)] truncate">
            {host.agent_id}
          </span>
          <span className="shrink-0 rounded-md border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
            {host.engine_os}/{host.engine_arch}
          </span>
        </div>

        {/* Row 2: engine version */}
        <p className="text-[10px] text-[var(--muted)]">
          Docker {host.engine_version}
        </p>

        {/* Row 3: metrics */}
        <div className="grid grid-cols-3 gap-3 text-[10px]">
          <div>
            <span className="text-[var(--muted)]">Containers</span>
            <p className="font-mono tabular-nums text-[var(--text)]">
              {running}/{total}
            </p>
          </div>
          <div>
            <span className="text-[var(--muted)]">CPU</span>
            <MiniBar value={avgCpu} />
            <p className="font-mono tabular-nums text-[var(--text)]">
              {avgCpu.toFixed(1)}%
            </p>
          </div>
          <div>
            <span className="text-[var(--muted)]">Memory</span>
            <MiniBar value={avgMem} />
            <p className="font-mono tabular-nums text-[var(--text)]">
              {avgMem.toFixed(1)}%
            </p>
          </div>
        </div>

        {/* Row 4: stacks */}
        {stacks.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {stacks.map((s) => (
              <span
                key={s}
                className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]"
              >
                {s}
              </span>
            ))}
          </div>
        )}

        {/* Row 5: last seen */}
        <p className={`text-[10px] ${isOffline ? "text-[var(--warn)]" : "text-[var(--muted)]"}`}>
          {isOffline ? `Offline \u2014 last seen ${relativeTime}` : `Last seen ${relativeTime}`}
        </p>
      </button>
    </Card>
  );
}

export function DockerHostPickerModal({ open, onClose, mode, hostData }: Props) {
  const router = useRouter();

  const handleSelect = (host: DockerHostSummary) => {
    const nodeId = `docker-host-${host.normalized_id}`;
    const path = mode === "container"
      ? `/nodes/${encodeURIComponent(nodeId)}/new-container`
      : `/nodes/${encodeURIComponent(nodeId)}/new-compose`;
    router.push(path);
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Select Docker Host"
      className="md:!max-w-2xl"
    >
      <div className="p-4 overflow-y-auto max-h-[calc(100vh-10rem)]">
        {hostData.length === 0 ? (
          <EmptyState
            icon={Server}
            title="No Docker hosts connected"
            description="Add a device with Docker to get started."
            action={
              <Button variant="primary" onClick={() => { router.push("/nodes"); onClose(); }}>
                Go to Devices
              </Button>
            }
          />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {hostData.map(({ host, containers }) => (
              <HostCard
                key={host.agent_id}
                host={host}
                containers={containers}
                onSelect={() => handleSelect(host)}
              />
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors (or pre-existing ones only)

- [ ] **Step 3: Commit**

```bash
git add web/console/app/components/containers/DockerHostPickerModal.tsx
git commit -m "feat(containers): add DockerHostPickerModal component

Host selection modal with capacity cards showing container counts,
CPU/memory utilization, existing stacks, and online/offline status.
Used by both Add Container and Add Stack flows."
```

---

### Task 2: Wire Buttons and Modal into Containers Page

**Files:**
- Modify: `web/console/app/[locale]/(console)/containers/page.tsx`

- [ ] **Step 1: Add imports**

At the top of the file, add these imports after the existing ones:

```tsx
import { Plus } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Tip } from "../../../components/ui/Tip";
import { DockerHostPickerModal } from "../../../components/containers/DockerHostPickerModal";
import { useToast } from "../../../contexts/ToastContext";
import { useRouter } from "../../../../i18n/navigation";
```

- [ ] **Step 2: Add state and handler logic**

Inside the `ContainersPage` component, after the existing state declarations (after line 34 — `stackFilter`), add:

```tsx
  const [pickerMode, setPickerMode] = useState<"container" | "stack" | null>(null);
  const { addToast } = useToast();
  const router = useRouter();

  const handleAddAction = useCallback(
    (mode: "container" | "stack") => {
      if (loading || hostData.length === 0) return;
      // Single-host shortcut
      if (hostData.length === 1) {
        const nodeId = `docker-host-${hostData[0].host.normalized_id}`;
        const path = mode === "container"
          ? `/nodes/${encodeURIComponent(nodeId)}/new-container`
          : `/nodes/${encodeURIComponent(nodeId)}/new-compose`;
        addToast("info", `Deploying to ${hostData[0].host.agent_id}`);
        router.push(path);
        return;
      }
      // Multi-host: open picker
      setPickerMode(mode);
    },
    [loading, hostData, addToast, router],
  );
```

- [ ] **Step 3: Add buttons to PageHeader**

Replace the existing `<PageHeader>` call (around line 98-101):

```tsx
      <PageHeader
        title="Containers"
        subtitle="Fleet-level container observability across all Docker hosts"
        action={
          <div className="flex gap-2">
            {!loading && hostData.length === 0 ? (
              <Tip content="No Docker hosts available">
                <Button variant="ghost" size="sm" disabled>
                  <Plus size={14} />
                  Add Stack
                </Button>
              </Tip>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                loading={loading && hostData.length === 0}
                disabled={loading && hostData.length === 0}
                onClick={() => handleAddAction("stack")}
              >
                <Plus size={14} />
                Add Stack
              </Button>
            )}
            {!loading && hostData.length === 0 ? (
              <Tip content="No Docker hosts available">
                <Button variant="primary" size="sm" disabled>
                  <Plus size={14} />
                  Add Container
                </Button>
              </Tip>
            ) : (
              <Button
                variant="primary"
                size="sm"
                loading={loading && hostData.length === 0}
                disabled={loading && hostData.length === 0}
                onClick={() => handleAddAction("container")}
              >
                <Plus size={14} />
                Add Container
              </Button>
            )}
          </div>
        }
      />
```

- [ ] **Step 4: Add the modal at the end of the JSX**

Before the closing `</>` of the component return (before line ~209), add:

```tsx
      <DockerHostPickerModal
        open={pickerMode !== null}
        onClose={() => setPickerMode(null)}
        mode={pickerMode ?? "container"}
        hostData={hostData}
      />
```

- [ ] **Step 5: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 6: Manual verification**

Open `http://localhost:3000/containers` in browser and verify:
1. Two buttons appear in the page header (Add Stack ghost, Add Container primary)
2. Buttons show built-in loading spinner while initial fetch runs, disabled when no hosts
3. When no hosts exist after loading: buttons disabled with "No Docker hosts available" tooltip
4. Clicking a button opens the host selection modal (if >1 host) or navigates directly (if 1 host) with toast
5. Host cards show name, OS/arch pill, engine version, container counts, CPU/mem bars, stack pills, last seen
6. Clicking a host card navigates to the correct create form
7. Offline hosts (>60s since last seen) appear dimmed with warning text

- [ ] **Step 7: Commit**

```bash
git add web/console/app/[locale]/(console)/containers/page.tsx
git commit -m "feat(containers): wire Add Container / Add Stack buttons to fleet page

Adds PageHeader action buttons that open DockerHostPickerModal for
host selection. Single-host setups skip the modal with a toast.
Buttons show loading state during initial fetch and disabled state
with tooltip when no Docker hosts are available."
```

---

## Verification Checklist

After both tasks:
- [ ] TypeScript compiles (`npx tsc --noEmit`)
- [ ] Loading state: buttons show built-in spinner, disabled
- [ ] Zero hosts: buttons disabled with tooltip
- [ ] One host: buttons navigate directly + toast
- [ ] Multiple hosts: modal opens with host cards
- [ ] Container mode navigates to `/nodes/{id}/new-container`
- [ ] Stack mode navigates to `/nodes/{id}/new-compose`
- [ ] Modal empty state links to nodes page
- [ ] Offline hosts appear dimmed with warning timestamp
