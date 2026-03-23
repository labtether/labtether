# Remote View NewTabPage Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the Remote View NewTabPage to match the design quality of Files/Terminal pages — vertical flow layout, status dots with glow, protocol badges, and a two-step Quick Connect dialog replacing the current search+dropdown+button combo.

**Architecture:** Extract protocol style mappings to `types.ts`. Rewrite `NewTabPage.tsx` as a vertical flow page with search-only filter bar, device rows with status dots/protocol badges, and bookmark rows. Extract the Quick Connect dialog into its own component (`QuickConnectDialog.tsx`) with two-step flow (connection → auth). Remove the standalone `CredentialDialog` from `page.tsx` and integrate auth into the Quick Connect flow.

**Tech Stack:** React, TypeScript, Tailwind CSS, Lucide icons, existing design system (CSS variables, `Card`, `Input`, `PageHeader` components)

**Spec:** `notes/specs/2026-03-22-remote-view-redesign.md`

---

### Task 1: Add protocol style mappings to types.ts

**Files:**
- Modify: `web/console/app/[locale]/(console)/remote-view/types.ts`

- [ ] **Step 1: Add protocol style constants**

Add per-protocol color mappings for badges and UI elements after the existing `PROTOCOL_DOT_COLOR`:

```typescript
/** Protocol badge styles for device/bookmark rows. */
export const PROTOCOL_BADGE_STYLE: Record<RemoteViewProtocol, string> = {
  vnc: "bg-green-500/8 text-green-500/70 border-green-500/15",
  rdp: "bg-blue-500/8 text-blue-500/70 border-blue-500/15",
  spice: "bg-amber-500/8 text-amber-500/70 border-amber-500/15",
  ard: "bg-purple-500/8 text-purple-500/70 border-purple-500/15",
};

/** Protocol icon tint backgrounds. */
export const PROTOCOL_ICON_BG: Record<RemoteViewProtocol, string> = {
  vnc: "bg-green-500/10",
  rdp: "bg-blue-500/10",
  spice: "bg-amber-500/10",
  ard: "bg-purple-500/10",
};

/** Protocol icon stroke colors. */
export const PROTOCOL_ICON_COLOR: Record<RemoteViewProtocol, string> = {
  vnc: "text-green-500/60",
  rdp: "text-blue-500/60",
  spice: "text-amber-500/60",
  ard: "text-purple-500/60",
};

/** Protocol selector button styles (selected vs unselected). */
export const PROTOCOL_SELECTOR_STYLE: Record<RemoteViewProtocol, { selected: string; unselected: string }> = {
  vnc: {
    selected: "bg-green-500/10 border-green-500/35 shadow-[0_0_16px_rgba(34,197,94,0.06)]",
    unselected: "bg-green-500/4 border-green-500/10",
  },
  rdp: {
    selected: "bg-blue-500/10 border-blue-500/35 shadow-[0_0_16px_rgba(59,130,246,0.06)]",
    unselected: "bg-blue-500/4 border-blue-500/10",
  },
  spice: {
    selected: "bg-amber-500/10 border-amber-500/35 shadow-[0_0_16px_rgba(245,158,11,0.06)]",
    unselected: "bg-amber-500/4 border-amber-500/10",
  },
  ard: {
    selected: "bg-purple-500/10 border-purple-500/35 shadow-[0_0_16px_rgba(168,85,247,0.06)]",
    unselected: "bg-purple-500/4 border-purple-500/10",
  },
};

/** Protocol name text colors for selector buttons. */
export const PROTOCOL_NAME_COLOR: Record<RemoteViewProtocol, { selected: string; unselected: string }> = {
  vnc: { selected: "text-green-500/95", unselected: "text-green-500/45" },
  rdp: { selected: "text-blue-500/95", unselected: "text-blue-500/45" },
  spice: { selected: "text-amber-500/95", unselected: "text-amber-500/45" },
  ard: { selected: "text-purple-500/95", unselected: "text-purple-500/45" },
};

/** All supported protocols with display metadata. */
export const PROTOCOLS: { value: RemoteViewProtocol; label: string }[] = [
  { value: "vnc", label: "VNC" },
  { value: "rdp", label: "RDP" },
  { value: "spice", label: "SPICE" },
  { value: "ard", label: "ARD" },
];
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/remote-view/types.ts
git commit -m "refactor(remote-view): extract protocol style mappings to types.ts"
```

---

### Task 2: Create the QuickConnectDialog component

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/QuickConnectDialog.tsx`

This is the two-step modal dialog. Step 1: protocol + host + port + save-as-bookmark + nickname. Step 2: username + password + connect.

- [ ] **Step 1: Create the QuickConnectDialog component**

Create `QuickConnectDialog.tsx` with the following structure:

```typescript
// Props interface
interface QuickConnectDialogProps {
  open: boolean;
  onClose: () => void;
  onConnect: (params: {
    protocol: RemoteViewProtocol;
    host: string;
    port: number;
    username?: string;
    password?: string;
    saveBookmark?: { label: string };
  }) => void;
}
```

Component internals:
- State: `step` (1 | 2), `protocol`, `host`, `port`, `saveAsBookmark`, `nickname`, `username`, `password`
- `parseQuickConnect()` reused from existing code for URI auto-detection in the host field
- Step 1 renders: dialog header + step indicator + protocol grid + host input + port input + bookmark checkbox (with nickname field when checked) + Next button + URI hint
- Step 2 renders: dialog header + step indicator (step 1 green check) + connection summary card + username input + password input + Connect button + "Connect without credentials" + Back link
- When protocol changes, auto-update port to `defaultPort(protocol)`
- When user pastes a URI in host field, auto-detect and fill protocol + host + port via `onChange` handler
- `onClose` resets all state
- Overlay: `fixed inset-0 bg-black/50 z-50 flex items-center justify-center`
- Dialog: 320px wide, `var(--panel-glass)` background, `border-[var(--panel-border)]`, `rounded-2xl`, multi-layer shadow, specular highlight (same as Card component)

Key UI details from the polished mockup:
- Protocol buttons: 4-col grid, each with name + port, selected state uses `PROTOCOL_SELECTOR_STYLE[proto].selected` and `PROTOCOL_NAME_COLOR[proto].selected`
- Step indicator: numbered circles with accent glow when active, green check when done, gray when pending. Line between dots.
- Inputs use the `Input` component from `../../components/ui/Input`
- Primary button: accent gradient background (`linear-gradient(135deg, var(--accent), #d4006a)`), white text, rounded-lg, shadow glow
- Secondary button: transparent, `border-[var(--panel-border)]`, muted text
- Ghost "Back" button: text-only with chevron-left icon

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/remote-view/QuickConnectDialog.tsx
git commit -m "feat(remote-view): add QuickConnectDialog two-step component"
```

---

### Task 3: Rewrite NewTabPage with new layout

**Files:**
- Modify: `web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx`

Full rewrite of the component. The interface (`NewTabPageProps`) stays the same — `onConnectDevice`, `onConnectBookmark`, `onConnectAdhoc`.

- [ ] **Step 1: Rewrite NewTabPage**

New structure:
1. Remove: `Card` import, `quickProtocol` state, `handleQuickConnect`, `parseQuickConnect` function (moved to dialog), protocol legend section, two-column grid layout
2. Add: `QuickConnectDialog` import, `showQuickConnect` state
3. Add: `filteredBookmarks` memo that filters bookmarks by search query (label, host)

Layout (vertical flow, `max-w-2xl mx-auto`):

**PageHeader** with `action` prop:
```tsx
<PageHeader
  title="Remote View"
  subtitle="Connect to remote desktops across your homelab"
  action={
    <button
      onClick={() => setShowQuickConnect(true)}
      className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-lg bg-[var(--accent-subtle)] border border-[var(--accent)]/20 text-[var(--accent-text)] text-xs font-medium hover:border-[var(--accent)]/40 transition-[border-color] duration-[var(--dur-fast)]"
    >
      <Zap size={14} />
      Quick Connect
    </button>
  }
/>
```

**Search bar** (standalone, no Card wrapper):
```tsx
<div className="max-w-2xl mx-auto w-full">
  <div className="relative">
    <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--muted)] pointer-events-none" />
    <Input
      className="pl-9"
      placeholder="Search devices and bookmarks..."
      value={searchQuery}
      onChange={(e) => setSearchQuery(e.target.value)}
    />
  </div>
</div>
```

**Available Devices section:**
- Section header: `<h3 className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider mb-3">Available Devices</h3>`
- Each device row is a `<button>` with:
  - Icon container (30x30, `rounded-lg`, `bg-green-500/10` when online, `bg-[var(--surface)]` when offline)
  - Status dot (absolute, top-right, `w-[7px] h-[7px]`, `bg-[var(--ok)]` with `box-shadow: 0 0 6px 1px var(--ok-glow)`) — only shown when online
  - Asset type icon via `assetTypeIcon(device.type)`, `w-3.5 h-3.5`, `text-green-500/60` (online) or `text-[var(--muted)]` (offline)
  - Name + IP · platform text
  - Protocol badge: `<span className="text-[9px] px-[7px] py-0.5 rounded-[5px] border font-medium ${PROTOCOL_BADGE_STYLE[protocol]}">{label}</span>` — only shown for online
  - Row container: `flex items-center gap-2.5 w-full text-left px-3 py-2.5 rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)]`
  - Online hover: `hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)]`
  - Offline: `opacity-50 cursor-not-allowed`
  - `transition-[border-color,box-shadow] duration-[var(--dur-fast)]`
- Empty state: same centered Monitor icon + message pattern
- Stacked vertically with `gap-1.5`

**Bookmarks section:**
- Section header: same style as devices
- Each bookmark row (same container style as devices):
  - Protocol dot: `w-[7px] h-[7px] rounded-full ${PROTOCOL_DOT_COLOR[bm.protocol]}`
  - Label + `protocol://host:port` (mono) text
  - Protocol badge (same as devices)
  - Delete button: `Trash2` icon, `opacity-0 group-hover:opacity-60 hover:!opacity-100`
  - Row: `group` class for hover-reveal delete
- Add bookmark button: dashed border, Plus icon, text
- Inline bookmark form: same fields as current but restyled to match new design (use `Input` component)

**QuickConnectDialog:**
```tsx
<QuickConnectDialog
  open={showQuickConnect}
  onClose={() => setShowQuickConnect(false)}
  onConnect={handleQuickConnectSubmit}
/>
```

Where `handleQuickConnectSubmit` calls `onConnectAdhoc` and optionally creates a bookmark via `createBookmark`.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx
git commit -m "feat(remote-view): rewrite NewTabPage with vertical flow layout"
```

---

### Task 4: Update page.tsx — remove CredentialDialog, integrate Quick Connect

**Files:**
- Modify: `web/console/app/[locale]/(console)/remote-view/page.tsx`

- [ ] **Step 1: Update page.tsx**

Changes:
1. Remove the `CredentialDialog` component (lines ~33-131) and its `PendingCredentials` interface
2. Remove `pendingCredentials` / `setPendingCredentials` state
3. Remove `sessionCredentials` / `setSessionCredentials` state
4. Update `NewTabPageProps` callback: `onConnectAdhoc` now receives credentials directly (protocol, host, port, username?, password?)
5. Simplify `handleConnectBookmark`:
   - If bookmark `has_credentials`, fetch creds via `getBookmarkCredentials()` and pass to session directly
   - If not, open the Quick Connect dialog pre-filled (or connect without creds) — but for now, just connect without since the Quick Connect dialog handles new ad-hoc connections only. Bookmark clicks still connect directly.
6. Remove the `{pendingCredentials && <CredentialDialog>}` render
7. Pass credentials into `RemoteViewSession` via `initialCredentials` keyed by tab ID (keep this pattern, just source creds from the connect callbacks instead of a separate dialog)

Key: the `CredentialDialog` overlay is fully replaced by QuickConnectDialog's step 2. Bookmark clicks with saved credentials bypass the dialog entirely. Bookmark clicks without saved credentials connect without creds (user can always bookmark from Quick Connect with creds for future use).

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 3: Verify the app renders**

Run: `cd web/console && npm run dev` (if not already running)
Navigate to Remote View page, verify:
- Page renders with new layout (vertical flow, search bar, device list, bookmarks)
- Quick Connect button opens the dialog
- Dialog two-step flow works (Next, Back, Connect)
- Clicking a device connects
- Clicking a bookmark connects
- Search filters devices and bookmarks
- Add bookmark inline form works

- [ ] **Step 4: Commit**

```bash
git add web/console/app/[locale]/(console)/remote-view/page.tsx
git commit -m "feat(remote-view): remove CredentialDialog, integrate Quick Connect flow"
```

---

### Task 5: Visual polish pass

**Files:**
- Modify: `web/console/app/[locale]/(console)/remote-view/QuickConnectDialog.tsx`
- Modify: `web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx`

- [ ] **Step 1: Polish the Quick Connect dialog**

Ensure these details from the polished mockup are implemented:
- Dialog container: gradient background, specular highlight (top edge), multi-layer shadow
- Protocol buttons: glow shadow on selected state
- Step indicator: accent glow on active number dot, gradient line between steps when on step 2
- Primary button: accent gradient with `box-shadow: 0 2px 12px rgba(255,0,128,0.15)`, lift on hover (`hover:-translate-y-px`)
- Connection summary card (step 2): protocol dot + nickname + URI + edit pencil icon
- Smooth transitions between steps (no jarring layout shifts)

- [ ] **Step 2: Polish the main page**

Ensure these details are implemented:
- Device rows: verify accent glow hover matches Files page exactly
- Status dots: verify glow shadow renders properly
- Protocol badges: verify color contrast is readable
- Bookmark delete icon: verify opacity transition is smooth
- Search input focus state: accent border + subtle glow ring
- Empty states: verify centered layout with proper spacing

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add web/console/app/[locale]/(console)/remote-view/QuickConnectDialog.tsx web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx
git commit -m "style(remote-view): polish Quick Connect dialog and page layout"
```
