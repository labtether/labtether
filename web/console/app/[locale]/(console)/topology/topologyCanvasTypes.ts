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
  id: string
  name: string
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

export interface UnsortedResponse {
  unsorted: string[]
  suggestions: PlacementSuggestion[]
}

// ── Shared color constants ─────────────────────────────────────────────────────

export const SOURCE_COLORS: Record<string, { bg: string; text: string }> = {
  proxmox:        { bg: "rgba(249,115,22,0.12)", text: "#fb923c" },
  agent:          { bg: "rgba(59,130,246,0.12)",  text: "#60a5fa" },
  docker:         { bg: "rgba(34,197,94,0.12)",   text: "#4ade80" },
  portainer:      { bg: "rgba(139,92,246,0.12)",  text: "#a78bfa" },
  truenas:        { bg: "rgba(6,182,212,0.12)",    text: "#22d3ee" },
  homeassistant:  { bg: "rgba(234,179,8,0.12)",   text: "#fbbf24" },
};

export const STATUS_COLORS: Record<string, string> = {
  online:   "#22c55e",
  degraded: "#eab308",
  offline:  "#ef4444",
  unknown:  "#71717a",
}

export const RELATIONSHIP_TYPES: { value: RelationshipType; label: string }[] = [
  { value: "runs_on",      label: "Runs on" },
  { value: "hosted_on",    label: "Hosted on" },
  { value: "depends_on",   label: "Depends on" },
  { value: "provides_to",  label: "Provides to" },
  { value: "connected_to", label: "Connected to" },
  { value: "peer_of",      label: "Peer of" },
];
