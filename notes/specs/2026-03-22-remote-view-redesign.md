# Remote View NewTabPage Redesign

**Date:** 2026-03-22
**Status:** Approved
**Mockups:** `.superpowers/brainstorm/23640-1774129103/`

## Problem

The Remote View NewTabPage has a flat, lifeless layout with no visual hierarchy. It doesn't match the design quality of other console pages (Files, Terminal, Dashboard). Specific issues:

- Wall of identical gray device cards with no differentiation
- Quick Connect combines search, protocol dropdown, and Connect button in one confusing input
- Two-column layout (devices | bookmarks + protocol legend) is arbitrary and unbalanced
- Credential prompt is a separate dialog disconnected from the connect flow
- No protocol color coding, no status dots, no glow effects

## Design

### Layout

Vertical flow, `max-w-2xl mx-auto`, matching Files and Terminal landing pages:

1. **PageHeader** — standard `// REMOTE VIEW` + gradient title + subtitle + Quick Connect action button
2. **Search bar** — standalone `Input` component, filter-only (devices + bookmarks)
3. **Available Devices** — stacked list
4. **Bookmarks** — stacked list with inline add form

### PageHeader

Uses the existing `PageHeader` component with the `action` prop for a "Quick Connect" button:
- Accent-tinted background (`bg-[var(--accent-subtle)]`)
- Accent border (`border-[var(--accent)]/20`)
- Zap icon + "Quick Connect" text
- Clicking opens the Quick Connect dialog

### Search Bar

Simple `Input` component with search icon, no protocol controls:
- Placeholder: "Search devices and bookmarks..."
- Filters both the device list and bookmarks list as you type
- No connect functionality — purely a filter

### Available Devices Section

Section header: uppercase `text-xs font-medium` label "Available Devices"

Each device row:
- **Icon container**: 30x30px rounded-lg, green-tinted background (`bg-green-500/10`)
- **Status dot**: 7px circle, absolute positioned top-right of icon container
  - Online: `bg-[var(--ok)]` with `box-shadow: 0 0 6px 1px var(--ok-glow)`
  - Offline: no dot
- **Icon**: asset type icon from `assetTypeIcon()`, colored `text-green-500/60` (online) or `text-[var(--muted)]` (offline)
- **Text**: name (13px, font-medium) + IP · platform (10px, muted)
- **Protocol badge**: colored pill showing detected protocol (VNC/RDP/SPICE/ARD)
  - Uses per-protocol color scheme: green (VNC), blue (RDP), amber (SPICE), purple (ARD)
  - Background: `{color}-500/8`, border: `{color}-500/15`, text: `{color}-500/70`
  - Only shown for online devices
- **Offline treatment**: entire row at `opacity-50`, `cursor-not-allowed`, no protocol badge
- **Hover**: `hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)]` (matching Files page)
- **Transition**: `transition-[border-color,box-shadow] duration-[var(--dur-fast)]`

Row container: `border border-[var(--panel-border)] bg-[var(--panel-glass)] rounded-lg px-3 py-2.5`

### Bookmarks Section

Section header: uppercase label "Bookmarks"

Each bookmark row (same container style as devices):
- **Protocol dot**: 7px circle, colored per protocol
- **Text**: label (13px, font-medium) + `protocol://host:port` (10px, mono, muted)
- **Protocol badge**: same colored pill as devices
- **Delete button**: Trash2 icon, `opacity-0 group-hover:opacity-60 hover:!opacity-100`
- **Hover**: same accent glow as device rows

"Add bookmark" button:
- Dashed border (`border-dashed border-[var(--line)]`)
- Plus icon + "Add bookmark" text
- Hover: `hover:border-[var(--accent)]/50 hover:text-[var(--text-secondary)]`
- Clicking reveals an inline form (same pattern as current implementation, just restyled)

### Quick Connect Dialog

Modal overlay (`fixed inset-0 bg-black/50 z-50`) with centered dialog.

**Dialog container:**
- Width: 320px
- Background: `linear-gradient(180deg, #161619, #131316)` (or use `var(--panel-glass)` and `var(--panel)`)
- Border: `1px solid rgba(255,255,255,0.08)` / `var(--panel-border)`
- Border-radius: 16px
- Shadow: multi-layer (`0 4px 24px rgba(0,0,0,0.5), 0 12px 48px rgba(0,0,0,0.4)`)
- Specular highlight: top edge gradient bar (same as `Card` component)

**Two-step flow:**

#### Step 1 — Connection

1. **Header**: "Quick Connect" title + X close button
2. **Step indicator**: `1 Connection ── 2 Auth` with active/pending dot states
3. **Protocol selector**: 4-column grid of protocol buttons
   - Each button: protocol name + default port
   - Selected: filled background with protocol color, glow shadow
   - Unselected: subtle border with muted text
4. **Host input**: text field, placeholder "192.168.1.10 or hostname"
5. **Port input**: text field, auto-filled from selected protocol's default port
6. **Save as bookmark**: checkbox toggle
   - When checked: reveals **Nickname** text field below
7. **Next button**: accent gradient (`#ff0080` → `#d4006a`), full-width, "Next →"
8. **Hint**: "Or paste a URI: vnc://host:port" — pasting auto-fills protocol, host, port

#### Step 2 — Authentication

1. **Header**: same
2. **Step indicator**: step 1 shows green checkmark, step 2 active, gradient line between
3. **Connection summary card**: protocol dot + nickname + URI + edit pencil (goes back to step 1)
4. **Username input**: text field
5. **Password input**: password field
6. **Connect button**: accent gradient, full-width
7. **Connect without credentials**: secondary button (outline)
8. **Back link**: ghost button with chevron-left

**URI pasting behavior**: If user pastes a full URI (e.g., `vnc://192.168.1.10:5900`) into the Host field at step 1, auto-parse and fill protocol + host + port.

### Credential Flow Changes

The existing standalone `CredentialDialog` in `page.tsx` is replaced:
- **Quick Connect (adhoc)**: credentials are collected in step 2 of the dialog
- **Bookmark connections**: if bookmark has saved credentials, use them directly (no prompt). If not, show the same step-2 auth form
- **Device connections**: use agent-based auth as today (no credential prompt needed)

### Empty States

**No devices**: centered icon (Monitor) + "No devices found" text + "Add a device" link (same pattern as current)

**No bookmarks**: just the "Add bookmark" dashed button (no separate empty state needed)

**Search no results**: "No devices or bookmarks match your search" centered text

## Files Changed

- `web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx` — full rewrite
- `web/console/app/[locale]/(console)/remote-view/page.tsx` — replace `CredentialDialog` with Quick Connect dialog integration
- `web/console/app/[locale]/(console)/remote-view/types.ts` — add protocol color/style mappings (move from component to types)

## Not In Scope

- Protocol auto-detection from agent data (devices currently default to VNC)
- Display quality / scaling options in the Quick Connect dialog
- Drag-and-drop bookmark reordering
