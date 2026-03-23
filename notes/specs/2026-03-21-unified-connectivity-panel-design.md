# Unified Connectivity Panel

## Problem

The device detail page has a disjointed "Services" section below the capability grid with two unrelated pieces bolted together:

1. A "SERVICES" label with an "Add Web Service" button that navigates away to `/services/config`
2. A standalone `ProtocolsPanel` card showing SSH/VNC/RDP/Telnet/ARD protocol configs

This looks bad, wastes vertical space (especially the empty state), and creates confusion about the relationship between "services" and "protocols." Both are ways to connect to a device — they should be unified.

## Design

### Concept

Replace the split Services + Protocols section with a single **"Connect" capability** that lives inside the existing `DeviceCapabilityGrid`. Both protocols (SSH, VNC, RDP, Telnet, ARD) and web services (HTTP/HTTPS URLs for device-hosted UIs) are treated as "connections" — different types under one umbrella.

### What Changes

**Remove:**
- The `<div className="mt-3">` section in `page.tsx` (lines ~328–340) containing the "SERVICES" label, "Add Web Service" link, and `<ProtocolsPanel>`
- The standalone `ProtocolsPanel` rendering on the dashboard view

**Add:**
- A "Connect" panel definition in `devicePanelCorePanels.ts` (or `devicePanelConnectorPanels.ts`)
- A "Connect" panel renderer in the panel rendering system
- An "+ Add Connection" button in `DeviceIdentityBar`
- A unified type picker + form flow for adding connections

**Keep:**
- `ProtocolsPanel.tsx` — refactored to render inside the Connect panel as the "Protocols" section
- `ProtocolForm.tsx` — reused as-is for adding/editing protocols
- `useProtocolConfigs.ts` — unchanged, still manages protocol CRUD
- Existing `WebService` model and `/api/v1/services/web/manual` API — reused for web service CRUD

### Connect Card in the Capability Grid

The "Connect" card appears in the `DeviceCapabilityGrid` alongside System, Remote View, Monitoring, etc.

**Summary content:**
- If connections exist: small badges showing configured connections (e.g., "SSH :22", "Portainer :9443") with status dots (green = ok, red = failed, gray = untested)
- If no connections: muted italic text "No connections" — minimal footprint, no big empty card

**Behavior:** Clicking the card opens the Connect panel (same `?panel=connect` URL pattern as other panels).

### Connect Panel (Full Panel View)

Opens as a full panel view, consistent with System, Logs, Actions, etc.

**Layout:**
- Header: "Connections" title + "+ Add Connection" button
- Two grouped sections with small uppercase labels:
  - **Protocols** — SSH, Telnet, VNC, RDP, ARD entries
  - **Web Services** — labeled HTTP/HTTPS URLs
- Sections only render if they have entries. If both empty, show a single centered message: "No connections configured"

**Connection row:**
- Icon (purple background for protocols, blue for web services)
- Name + status dot (green = test passed, red = test failed, gray = untested/unknown)
- Detail line: port or URL, username (protocols), last tested timestamp
- Action buttons:
  - **Connect** (arrow icon) — always present
  - **Test** (flask icon) — protocols only
  - **Push Hub Key** (key icon) — SSH only
  - **Edit** (pencil icon) — always present
  - **Delete** (trash icon) — always present

### Connect Action Behavior

Each connection type routes to the appropriate existing feature:

| Type | Connect Action |
|------|---------------|
| SSH | Navigate to Terminal tab (`?panel=terminal`) |
| VNC | Navigate to Remote View / Desktop tab (`?panel=desktop`) |
| RDP | Navigate to Remote View / Desktop tab (`?panel=desktop`) |
| ARD | Navigate to Remote View / Desktop tab (`?panel=desktop`) |
| Telnet | Navigate to Terminal tab (`?panel=terminal`) |
| Web Service | `window.open(url, '_blank')` — opens in new browser tab |

### Add Connection Flow

1. User clicks "+ Add Connection" (from header or panel button)
2. If triggered from header: navigates to Connect panel with add flow pre-opened
3. Type picker appears inline at the top of the panel:
   - **Protocol** — select type (SSH, Telnet, VNC, RDP, ARD)
   - **Web Service** — label + URL
4. Selecting "Protocol" → shows existing `ProtocolForm` component
5. Selecting "Web Service" → shows a simple form: label (text input) + URL (text input) + Save/Cancel

### "+ Add Connection" in the Header

- Rendered in `DeviceIdentityBar`, next to the edit and delete icon buttons
- Styled as a small bordered button with a plus icon and "Add Connection" text
- On click: navigates to `?panel=connect&adding=true`
- The Connect panel reads the `adding` query param to auto-open the type picker

### Web Service Data Model

Web services already exist in the codebase as `WebService` records with `host_asset_id`, `name`, `url`, `icon_key`, `category`, `status`, `response_ms`, health data, and more. The existing manual web service API lives at `/api/v1/services/web/manual`.

**No new data model or API endpoints needed.** The Connect panel:
- Fetches web services for the current device by filtering on `host_asset_id` matching the current asset
- Uses the existing `POST /api/v1/services/web/manual` endpoint for adding web services (pre-filling `host_asset_id`)
- Uses the existing update/delete endpoints for editing and removing
- Status dots for web services use the existing `status` field from the `WebService` model

### URL State

- The `adding` query param (`?panel=connect&adding=true`) is cleaned from the URL after the type picker is dismissed or a connection is saved, to avoid stale state on page refresh.

## Scope Boundaries

**In scope:**
- New "Connect" panel definition and renderer
- Refactor protocols rendering into the Connect panel
- Add Connection button in DeviceIdentityBar
- Type picker flow
- Web service add/edit/delete form (minimal)
- Connect card in capability grid with badges

**Out of scope:**
- Auto-discovery of web services (stays manual for now)
- Changing how Terminal or Desktop tabs work internally
- Changing the `/services` fleet-wide services page
- Protocol form redesign (reuse existing `ProtocolForm` as-is)
