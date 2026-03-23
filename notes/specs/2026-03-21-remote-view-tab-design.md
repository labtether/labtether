# Remote View Tab — Design Spec

## Overview

Add a top-level Remote View tab to the LabTether console, peer to Terminal and Files. Provides a dedicated, full-screen workspace for managing multiple remote desktop sessions via tabs, with support for VNC, RDP, SPICE, and ARD (Apple Remote Desktop) protocols.

## Goals

- **Multi-device remote view** — Multiple remote desktop sessions open simultaneously, switch between them via tab bar.
- **Dedicated workspace** — Full-screen viewer area with expanded toolbar controls, not embedded in node detail pages.
- **Unified remote access** — Terminal (SSH), Files (SFTP), and Remote View (VNC/RDP) as three peer top-level workspaces for interacting with the homelab.

## Non-Goals

- Does not replace the existing remote view in node detail pages (both coexist independently).
- No tiling/grid layout — one session at a time, full-screen per tab.
- No shared connection registry with Files — Remote View uses device picker + bookmarks, not saved connections.
- WebRTC viewer exists in the codebase but is not exposed as a user-selectable protocol in the new tab. It remains available only via node detail pages where device capabilities auto-select it.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Session layout | One at a time, tab switching | Remote desktops need maximum screen real estate |
| New tab flow | Device picker + quick connect | Like Terminal — devices managed elsewhere, not in this tab |
| External hosts | Bookmarks (lightweight) | Lighter than full saved connections; just host/port/protocol/label |
| Session persistence | Reconnect on return | Tab remembers target; viewer reconnects on mount. Simple, no background resource usage |
| File drawer | Keep as slide-out panel | Quick file transfers without leaving the session |
| Node detail page | Keep both, independent | Non-breaking; node detail viewer stays as-is |
| Protocols | VNC, RDP, SPICE, ARD | VNC + RDP cover 95% of use cases; SPICE for Proxmox/QEMU VMs; ARD for macOS. ARD routes to VNCViewer (ARD uses VNC as transport with Apple extensions) |
| Architecture | Mirror Files pattern (Approach A) | Proven tabbed pattern, independent evolution, minimal coupling |

## Layout Structure

```
┌─────────────────────────────────────────────────────────────┐
│ Sidebar │ Tab Bar: [proxmox-host] [win-server] [+]  Status │
│         ├───────────────────────────────────────────────────┤
│   D     │ Toolbar: Quality | Scale | Kbd | Clipboard |     │
│   N     │          Send Keys | Screenshot | Record |       │
│   S     │          Monitor | Files | Audio | Fullscreen    │
│   T     ├───────────────────────────────────────────────────┤
│   F     │                                                   │
│  [R]    │              Viewer Area                          │
│   To    │         (full remaining space)                    │
│   A     │                                                   │
│         │                                                   │
└─────────┴───────────────────────────────────────────────────┘
```

## Tab Bar

- Horizontal tabs with protocol color dots, device name labels, close buttons, `+` new tab button.
- Protocol colors: green = VNC, blue = RDP, amber = SPICE, purple = ARD.
- Connection status pill in right corner: connected/disconnected indicator + latency in ms.
- Follows the same visual pattern as `FileTabBar.tsx`.

## New Tab Landing Page

Two-column layout:

### Left: Available Devices
- Grid of managed assets that have remote view capabilities.
- Each card shows: device icon, name, IP address, protocol badges.
- Offline devices shown dimmed. Devices with no remote view capabilities greyed out.
- Search bar to filter devices.
- Click a card to connect. Multi-protocol devices show a small picker popover ("Connect via VNC" / "Connect via SPICE").

### Right: Bookmarks
- Saved external host connections with protocol dot, name, and URL.
- Click to connect immediately. Right-click for edit/delete.
- "Add bookmark" button opens a small form for manual entry.
- Protocol legend below bookmarks.

### Top: Quick Connect
- URL-style input that parses `vnc://host:port`, `rdp://host`, `spice://host:port`, `ard://host`.
- Protocol dropdown as fallback when no URL scheme is provided.
- Typing just a hostname defaults to VNC.
- After connecting via quick-connect, prompt to "Save as bookmark".

## Tab State Model

```typescript
type RemoteViewTabType = "new" | "device" | "bookmark" | "adhoc";
type RemoteViewProtocol = "vnc" | "rdp" | "spice" | "ard";
// Aligns with existing RemoteViewToolbar connectionState, plus "disconnected" for reconnect-on-return
type ConnectionState = "idle" | "connecting" | "authenticating" | "connected" | "disconnected" | "error";

interface RemoteViewTab {
  id: string;                          // UUID
  type: RemoteViewTabType;
  label: string;                       // device name, bookmark name, or hostname
  protocol?: RemoteViewProtocol;
  target?: {
    host: string;
    port: number;
    assetId?: string;                  // for managed devices
    bookmarkId?: string;               // for bookmarked external hosts
  };
  connectionState: ConnectionState;
  lastConnectedAt?: number;            // timestamp for reconnect-on-return
}
```

### Lifecycle

1. User clicks `+` → new tab with landing page (`type: "new"`).
2. User picks a device/bookmark/quick-connect → tab type updates, viewer starts connecting.
3. User navigates away from Remote View → WebSocket closes (viewer unmounts), tab retains target info.
4. User returns → tab auto-reconnects using saved target (viewer remounts, connects).
5. User closes tab → connection torn down, tab removed from state.

### Credentials

- Managed devices: credentials from the asset's stored config (already in LabTether).
- Bookmarks/ad-hoc: credentials dialog on first connect. Option to save credentials with bookmark.

## Expanded Toolbar

Organized into 6 groups separated by dividers:

| Group | Controls | Behavior |
|-------|----------|----------|
| **Display** | Quality (Auto/High/Med/Low), Scale (Fit/Fill/Native) | Auto quality adapts to connection bandwidth |
| **Input** | Keyboard Grab, Pointer Lock | Green highlight when active. Grab captures all keys including browser shortcuts |
| **Clipboard & Keys** | Clipboard Sync indicator, Send Keys dropdown | Clipboard shows green dot when synced. Send Keys: Ctrl+Alt+Del, Alt+Tab, Alt+F4, Print Screen, Super, Escape, custom combo |
| **Capture** | Screenshot, Record | Screenshot saves to Downloads. Record captures session as webm video. Both use viewer canvas |
| **Monitor** | Monitor selector dropdown | Only visible for RDP/SPICE sessions reporting multiple monitors. Defaults to primary |
| **Right side** | Files drawer toggle, Audio passthrough, Fullscreen | Fullscreen is highlighted as primary action. Audio is protocol-dependent |

## File Drawer

Reuses `RemoteViewFileDrawer` component as-is:
- Toggled via "Files" toolbar button.
- 300px width, slides in from right edge, overlays the viewer.
- Browses files on the connected device via agent file API.
- Supports: navigate, upload (drag-drop), download, show/hide hidden files.
- For ad-hoc connections without an agent or SFTP: Files button disabled with tooltip "File access requires an agent or SFTP connection".

## Component Structure

### New files

```
web/console/app/[locale]/(console)/remote-view/
├── page.tsx                    # Orchestrator — tab bar + active viewer or landing
├── RemoteViewTabBar.tsx        # Tab bar with protocol dots, close, +, status
├── useRemoteViewTabsState.ts   # Tab CRUD hook (add/remove/switch/update)
├── NewTabPage.tsx              # Device picker grid + bookmarks + quick connect
└── RemoteViewSession.tsx       # Active viewer + toolbar + file drawer wrapper
```

### Reused existing components

- `components/VNCViewer.tsx` — as-is (also handles ARD connections, since ARD uses VNC transport)
- `components/GuacamoleViewer.tsx` — as-is (handles RDP)
- `components/SPICEViewer.tsx` — as-is
- `components/WebRTCViewer.tsx` — as-is
- `components/RemoteViewerShell.tsx` — reference implementation for `RemoteViewSession.tsx`; already orchestrates viewer selection by protocol, toolbar wiring, file drawer, session panel, reconnect overlay, and performance overlay. `RemoteViewSession.tsx` adapts this pattern for the tabbed context rather than duplicating it.
- `components/RemoteViewFileDrawer.tsx` — as-is
- `components/SessionPanel.tsx` — credentials dialog reuse

### Modified existing components

- `components/RemoteViewToolbar.tsx` — add Screenshot button (clipboard sync, send keys, recording, and monitor selector already exist in the toolbar)
- `app/[locale]/(console)/layout.tsx` — add Remote View entry to sidebar nav

### New API route

- `/api/remote-bookmarks/[[...path]]/route.ts` — CRUD for external host bookmarks

### Bookmark data model

```typescript
interface RemoteBookmark {
  id: string;
  label: string;
  protocol: RemoteViewProtocol;
  host: string;
  port: number;
  credentials?: {
    username?: string;
    password?: string;  // encrypted at rest
  };
  createdAt: string;
  updatedAt: string;
}
```

Storage: Postgres (consistent with other LabTether operational state).

## Data Flow

1. `useRemoteViewTabsState` manages tab array + `activeTabId` in React state.
2. `page.tsx` renders `RemoteViewTabBar` + either `NewTabPage` or `RemoteViewSession` based on active tab type.
3. `RemoteViewSession` selects viewer component by protocol, passes connection config.
4. Viewer components handle their own WebSocket lifecycle (connect on mount, disconnect on unmount — gives reconnect-on-return for free).
5. Device capabilities come from `useFastStatus` context (already provided by layout).
6. Bookmarks fetched via `/api/remote-bookmarks/` on landing page mount.

## Estimated Scope

| Component | Approx. Lines | Complexity |
|-----------|--------------|------------|
| `page.tsx` | ~150 | Low — mirrors Files page pattern |
| `RemoteViewTabBar.tsx` | ~120 | Low — mirrors FileTabBar |
| `useRemoteViewTabsState.ts` | ~100 | Low — simpler than Files (no split mode) |
| `NewTabPage.tsx` | ~200 | Medium — device grid + bookmarks + quick connect |
| `RemoteViewSession.tsx` | ~100 | Low — adapts RemoteViewerShell pattern for tabbed context |
| `RemoteViewToolbar.tsx` changes | ~30 | Low — add Screenshot button (other controls already exist) |
| `/api/remote-bookmarks/` route | ~80 | Low — standard CRUD |
| DB migration (bookmarks table) | ~20 | Low |
| Layout sidebar update | ~5 | Trivial |
