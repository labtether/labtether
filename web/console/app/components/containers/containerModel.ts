// The 2.0 containers list model: what the hub sends for each Docker host (an agent) or Portainer endpoint, turned into
// rows with a state word, published ports and a link; then filtered, sorted and cut into host or stack sections.
import type { DockerContainer, DockerHostSummary } from "../../../lib/docker";

export type AgoUnit = "second" | "minute" | "hour" | "day" | "week" | "month" | "year";
export type Ago = { n: number; unit: AgoUnit };
export type StateKind = "running" | "exited" | "paused" | "restarting" | "created" | "dead" | "removing" | "other";
/** What Docker's status line says: the state, how long ago it changed (exited, restarting) or how long it is up, and health. */
export type CtState = { kind: StateKind; ago: Ago | null; up: Ago | null; health: "" | "healthy" | "unhealthy" | "starting" };
/** "" is plain running (no dot); off is a hollow dot; busy pulses. */
export type Tone = "" | "off" | "busy" | "warn" | "bad";
export type PortMap = { host: number; inside: number; proto: string };

export type CtHost = {
  /** Lookup key the hub resolves: the agent id or the Portainer endpoint asset id, normalized. */
  key: string;
  label: string;
  source: "docker" | "portainer";
  /** The host's own device page. */
  href: string;
  /** Docker agents take new containers and stacks; Portainer endpoints do not (yet). */
  canDeploy: boolean;
};

export type CtRow = {
  key: string;
  id: string;
  name: string;
  image: string;
  stack: string;
  state: CtState;
  tone: Tone;
  cpu: number | null;
  mem: number | null;
  ports: PortMap[];
  host: CtHost;
  /** The container's own page (a device page for its asset). */
  href: string;
  /** The asset id actions and Logs go to. */
  assetId: string;
};

const UNITS: AgoUnit[] = ["second", "minute", "hour", "day", "week", "month", "year"];
const AMOUNT = /(less than an?|about an?|an?|\d+)\s+(second|minute|hour|day|week|month|year)s?/i;

/** "2 days" → {2, day}; "About an hour" → {1, hour}; "Less than a second" → {0, second}. */
export function parseAmount(text: string): Ago | null {
  const m = AMOUNT.exec(text);
  if (!m) return null;
  const word = m[1].toLowerCase();
  const n = /^\d+$/.test(word) ? Number(word) : word.startsWith("less") ? 0 : 1;
  const unit = m[2].toLowerCase() as AgoUnit;
  return UNITS.includes(unit) ? { n, unit } : null;
}

/** Docker's state plus its status line ("Up 6 days (healthy)", "Exited (0) 2 days ago") as facts. */
export function parseDockerStatus(state: string, status: string): CtState {
  const s = (state || "").trim().toLowerCase();
  const line = (status || "").trim();
  const lower = line.toLowerCase();
  const health = /\(unhealthy\)/.test(lower) ? "unhealthy" : /health: starting/.test(lower) ? "starting" : /\(healthy\)/.test(lower) ? "healthy" : "";
  let kind: StateKind = (["running", "exited", "paused", "restarting", "created", "dead", "removing"] as const).find((k) => k === s) ?? "other";
  if (kind === "other") {
    if (/^up\b/.test(lower)) kind = /\(paused\)/.test(lower) ? "paused" : "running";
    else if (/^exited\b/.test(lower)) kind = "exited";
    else if (/^restarting\b/.test(lower)) kind = "restarting";
    else if (/^created\b/.test(lower)) kind = "created";
    else if (/^dead\b/.test(lower)) kind = "dead";
  }
  const up = /^up\b/.test(lower) ? parseAmount(line) : null;
  const ago = /\bago\b/.test(lower) ? parseAmount(line.replace(/\(\d+\)/, "")) : null;
  return { kind, ago, up, health };
}

export function stateTone(st: CtState): Tone {
  switch (st.kind) {
    case "running":
      return st.health === "unhealthy" ? "warn" : st.health === "starting" ? "busy" : "";
    case "restarting":
    case "removing":
      return "busy";
    case "dead":
      return "bad";
    default:
      return "off";
  }
}

/** Running, or on its way back (restarting): the Running chip. Everything else is Stopped. */
export const isRunning = (st: CtState) => st.kind === "running" || st.kind === "restarting";

/** "lscr.io/linuxserver/jellyfin:10.10.7" → "jellyfin:10.10.7"; a bare image id → its first 12 characters. */
export function shortImage(image: string): string {
  const ref = (image || "").replace(/@sha256:[0-9a-f]+$/i, "");
  if (/^sha256:[0-9a-f]+$/i.test(ref)) return ref.slice(7, 19);
  return ref.split("/").pop() ?? ref;
}

/** Published ports from "8096->8096/tcp, 0.0.0.0:8080:80/tcp"; unpublished (host 0) and repeated (IPv4 and IPv6) dropped. */
export function parsePorts(ports: string): PortMap[] {
  const out: PortMap[] = [];
  const seen = new Set<string>();
  // A bound address ("0.0.0.0:", "[::]:", ":::") is dropped first, so its last digits are not read as a port.
  const text = (ports || "").replace(/(\d{1,3}\.){3}\d{1,3}:|\[?::\]?:/g, "");
  for (const m of text.matchAll(/(\d+)(?:->|:)(\d+)(?:\/(tcp|udp|sctp))?/gi)) {
    const host = Number(m[1]);
    const inside = Number(m[2]);
    const proto = (m[3] || "tcp").toLowerCase();
    const id = `${host}/${inside}/${proto}`;
    if (!host || seen.has(id)) continue;
    seen.add(id);
    out.push({ host, inside, proto });
  }
  return out.sort((a, b) => a.host - b.host || a.inside - b.inside);
}

/** Memory as people read it: "96 MB", "1.2 GB". */
export function formatMem(bytes: number): string {
  const mb = bytes / 1048576;
  if (mb < 1000) return `${Math.max(0, Math.round(mb))} MB`;
  const gb = mb / 1024;
  return `${gb >= 100 ? Math.round(gb) : gb.toFixed(1)} GB`;
}

/** Lowercase, spaces and dots to dashes, as the hub names Docker assets. "docker-host-" on a route is dropped. */
export function hostKey(value: string): string {
  return (value || "").trim().toLowerCase().replaceAll(" ", "-").replaceAll(".", "-").replace(/^docker-host-/, "");
}
export const dockerHostAssetId = (key: string) => `docker-host-${hostKey(key)}`;
const short12 = (id: string) => (id.length > 12 ? id.slice(0, 12) : id);
export const dockerContainerAssetId = (host: string, id: string) => `docker-ct-${hostKey(host)}-${short12(id)}`;
export const dockerStackAssetId = (host: string, stack: string) => `docker-stack-${hostKey(host)}-${hostKey(stack)}`;
/** "portainer-endpoint-3" + container id → "portainer-container-3-<12>"; "" when the endpoint id is not that shape. */
export function portainerContainerAssetId(endpointAssetId: string, id: string): string {
  const m = /^portainer-endpoint-(\d+)$/.exec(endpointAssetId);
  return m ? `portainer-container-${m[1]}-${short12(id)}` : "";
}
export const nodeHref = (assetId: string) => `/nodes/${encodeURIComponent(assetId)}`;

/** A host summary from /api/docker/hosts or /api/portainer/endpoints. names: asset id → name, for Portainer's endpoint name. */
export function toHost(h: DockerHostSummary, names?: ReadonlyMap<string, string>): CtHost {
  if (h.source === "portainer") {
    return { key: h.agent_id, label: names?.get(h.agent_id) || h.agent_id, source: "portainer", href: nodeHref(h.agent_id), canDeploy: false };
  }
  const key = hostKey(h.normalized_id || h.agent_id);
  return { key, label: h.agent_id || key, source: "docker", href: nodeHref(dockerHostAssetId(key)), canDeploy: true };
}

export function toRow(host: CtHost, c: DockerContainer): CtRow {
  const state = parseDockerStatus(c.state, c.status);
  const assetId = host.source === "portainer" ? portainerContainerAssetId(host.key, c.id) : dockerContainerAssetId(host.key, c.id);
  const running = state.kind === "running";
  return {
    key: `${host.key}/${c.id}`,
    id: c.id,
    name: c.name || short12(c.id),
    image: c.image,
    stack: c.stack_name || c.labels?.["com.docker.compose.project"] || "",
    state,
    tone: stateTone(state),
    cpu: running && c.cpu_percent != null ? c.cpu_percent : null,
    mem: running && c.memory_bytes != null ? c.memory_bytes : null,
    ports: parsePorts(c.ports),
    host,
    href: assetId ? nodeHref(assetId) : host.href,
    assetId,
  };
}

export type Show = "all" | "running" | "stopped";
export type Grouping = "host" | "stack" | "none";
export const GROUPINGS: Grouping[] = ["host", "stack", "none"];
export type SortKey = "name" | "cpu" | "mem";
export type Sort = { key: SortKey; dir: "asc" | "desc" };

export function filterRows(rows: CtRow[], query: string, show: Show): CtRow[] {
  const q = query.trim().toLowerCase();
  return rows.filter((r) => {
    if (show === "running" && !isRunning(r.state)) return false;
    if (show === "stopped" && isRunning(r.state)) return false;
    return !q || [r.name, r.image, r.host.label, r.stack].some((v) => v.toLowerCase().includes(q));
  });
}

/** Name A–Z, or CPU / memory with stopped containers last whichever way; ties by name. */
export function sortRows(rows: CtRow[], sort: Sort): CtRow[] {
  const byName = (a: CtRow, b: CtRow) => a.name.localeCompare(b.name) || a.host.label.localeCompare(b.host.label);
  const value = (r: CtRow) => (sort.key === "cpu" ? r.cpu : r.mem);
  return [...rows].sort((a, b) => {
    if (sort.key === "name") return sort.dir === "asc" ? byName(a, b) : byName(b, a);
    const va = value(a);
    const vb = value(b);
    if (va == null || vb == null) return va == null && vb == null ? byName(a, b) : va == null ? 1 : -1;
    return (sort.dir === "asc" ? va - vb : vb - va) || byName(a, b);
  });
}

/** first | mid | last: the rail that joins a stack's containers under a host; "" for a lone row. */
export type Rail = "" | "first" | "mid" | "last";
export type SectionRow = { row: CtRow; rail: Rail; showStack: boolean; showHost: boolean };
export type Section = { key: string; title: string; kind: "host" | "stack" | "nostack" | "all"; href?: string; rows: SectionRow[] };

/** Keep a stack's containers together at its first member's place (the list's order otherwise). */
export function keepStacksTogether(rows: CtRow[]): CtRow[] {
  const out: CtRow[] = [];
  const seen = new Set<string>();
  for (const r of rows) {
    if (!r.stack) { out.push(r); continue; }
    if (seen.has(r.stack)) continue;
    seen.add(r.stack);
    out.push(...rows.filter((x) => x.stack === r.stack));
  }
  return out;
}

/** Rows of one host, stacks kept together on a rail; the stack name shows once, on its first row. */
export function railRows(rows: CtRow[], showHost = false): SectionRow[] {
  const list = keepStacksTogether(rows);
  return list.map((row, i) => {
    const up = !!row.stack && list[i - 1]?.stack === row.stack;
    const down = !!row.stack && list[i + 1]?.stack === row.stack;
    return { row, rail: up && down ? "mid" : up ? "last" : down ? "first" : "", showStack: !up, showHost };
  });
}

/**
 * Host sections (in the hosts' order), stack sections (A–Z, then containers outside a stack) or one plain list.
 * Rows keep the order they come in, which is the chosen sort.
 */
export function buildSections(rows: CtRow[], by: Grouping, hosts: CtHost[]): Section[] {
  if (by === "none") return rows.length ? [{ key: "all", title: "", kind: "all", rows: rows.map((row) => ({ row, rail: "", showStack: true, showHost: true })) }] : [];
  if (by === "stack") {
    const names = [...new Set(rows.filter((r) => r.stack).map((r) => r.stack))].sort((a, b) => a.localeCompare(b));
    const plain = (list: CtRow[]) => list.map((row) => ({ row, rail: "" as Rail, showStack: false, showHost: true }));
    const out: Section[] = names.map((s) => ({ key: `stack:${s}`, title: s, kind: "stack", rows: plain(rows.filter((r) => r.stack === s)) }));
    const loose = rows.filter((r) => !r.stack);
    if (loose.length) out.push({ key: "stack:-", title: "", kind: "nostack", rows: plain(loose) });
    return out;
  }
  return hosts
    .map((h): Section => ({ key: `host:${h.source}:${h.key}`, title: h.label, kind: "host", href: h.href, rows: railRows(rows.filter((r) => r.host.key === h.key && r.host.source === h.source)) }))
    .filter((s) => s.rows.length > 0);
}
