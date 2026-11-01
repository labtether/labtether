// Where everything on the Map sits. The router at the top, what plugs into it under a bus, grouped by zone, and each
// host's guests on a spine beneath it. Tiles sit one column (84 px) apart inside a zone and 16 px further apart between
// zones. A spine may run under a neighbour's tile; only what hangs below tiles keeps 16 px apart. Devices without a
// network link stand in their own row ("Not connected"); when nothing is linked they wrap to the given width.
// full: the Map. compact: the Dashboard picture (36 px tiles, no problem line, only guests with a problem).
import { byZone, hitOf, type MapDevice, type MapGraph } from "./mapModel";
import { rounded } from "./mapPaths";

export const COL = 84, TILE = 44, ROW = 28;
const ZGAP = 16, BGAP = 16, GGAP = 40, TICK = 12, NAME_MAX = 100, FOLD_AT = 6, WRAP_GAP = 36;
const MODE = {
  full: { tile: 44, block: 82, one: 40, bus: 64, drop: 24, start: 8, zl: 20, zly: 10 },
  compact: { tile: 36, block: 58, one: 24, bus: 44, drop: 18, start: 6, zl: 22, zly: 8 },
};

export type LayoutNode = { id: string; d: MapDevice; cx: number; y: number; depth: number; below: boolean; spineH: number };
/** A guest row, or the fold row ("+3 more" / "Show fewer") when id is "". */
export type LayoutRow = { id: string; d?: MapDevice; fold?: number | "less"; host: string; x: number; y: number };
/** A zone label or the "Not connected" title; key is a zone id, or "" for Other, or "lone". */
export type LayoutLabel = { key: string; x: number; y: number; w: number };
export type MapLayout = {
  nodes: LayoutNode[]; rows: LayoutRow[]; labels: LayoutLabel[];
  edges: Array<{ k: string; d: string }>; edgeD: Record<string, string>;
  parent: Record<string, string>; hostOf: Record<string, string>;
  w: number; h: number; block: number; tile: number;
  nodeOf: Map<string, LayoutNode>; rowOf: Map<string, LayoutRow>;
  nav: Record<string, { up: string; down: string; left: string; right: string }>;
};
export type LayoutOptions = {
  compact?: boolean;
  /** Levels under a root to draw (the Dashboard stops at the first tier under the switch). */
  depth?: number;
  /** Find text, lower case: unfolds a spine whose hidden guest matches. */
  q?: string;
  /** Hosts whose spine shows every guest. */
  open?: ReadonlySet<string>;
  /** Width the "Not connected" rows wrap at when nothing is linked. */
  wrap?: number;
  /** Measures a zone label (the label text depends on the language). */
  labelText?: (key: string) => string;
};

/** Rough width of Inter text, for spine and label room. */
export const textW = (s: string, px = 13) => [...s].reduce((w, c) => w + (/[ijlt.,:;'!|]/.test(c) ? 3.7 : /[fr\-\s()]/.test(c) ? 4.7 : /[mwMW]/.test(c) ? 10.4 : /[A-Z]/.test(c) ? 8.6 : /\d/.test(c) ? 7.4 : 7.1), 0) * (px / 13);

type Spine = { shown: MapDevice[]; fold: number | "less" | null };
type Measured = {
  id: string; d: MapDevice; s: Spine | null; spineH: number;
  kids: Array<{ sub: Measured; x: number }>; groups: Array<{ key: string; a: number; b: number }>;
  lo: number | null; hi: number | null; L: number; R: number;
};

/** Guests on a host's spine: six, then "+N more" (all once opened, or when a hidden one matches). Compact: problems only. */
export function spineRows(g: MapDevice[], host: string, o: LayoutOptions): Spine {
  if (o.compact) return { shown: g.filter((x) => x.tone !== "ok"), fold: null };
  if (g.length <= FOLD_AT) return { shown: g, fold: null };
  if (o.open?.has(host) || (o.q && g.slice(FOLD_AT).some((x) => hitOf(x, o.q!)))) return { shown: g, fold: "less" };
  return { shown: g.slice(0, FOLD_AT), fold: g.length - FOLD_AT };
}
/** Devices with guests (or children) hanging below alternate with those without, so tiles stay evenly spaced. */
function spread(subs: Measured[]): Measured[] {
  const below = subs.filter((t) => t.lo != null);
  if (below.length < 2) return subs;
  const plain = subs.filter((t) => t.lo == null);
  const out: Measured[] = [];
  while (below.length || plain.length) {
    if (below.length) out.push(below.shift()!);
    if (plain.length) out.push(plain.shift()!);
  }
  return out;
}
const foldW = (fold: number | "less") => (fold === "less" ? 72 : 52);
const rowW = (g: MapDevice) => Math.min(NAME_MAX, textW(g.name)) + (g.tone === "ok" ? 0 : 14);

export function layoutMap(graph: MapGraph, o: LayoutOptions = {}): MapLayout {
  const M = o.compact ? MODE.compact : MODE.full;
  const depthMax = o.depth ?? Infinity;
  const F = graph.forest;
  const label = o.labelText ?? ((k: string) => k);
  const nodes: LayoutNode[] = []; const rows: LayoutRow[] = []; const labels: LayoutLabel[] = [];
  const edges: MapLayout["edges"] = []; const edgeD: Record<string, string> = {}; const hostOf: Record<string, string> = {};

  /** Lays out siblings left to right by zone (shared by children and the "Not connected" row). */
  function flow(ids: string[], lvl: number, t: { kids: Measured["kids"]; groups: Measured["groups"] }) {
    let prev: number | null = null, sky = -Infinity;
    for (const [z, list] of byZone(graph, ids)) {
      spread(list.map((k) => measure(k, lvl))).forEach((sub, j) => {
        let x = prev == null ? 0 : prev + COL + (j ? 0 : ZGAP);
        if (sub.lo != null) { x = Math.max(x, sky + BGAP - sub.lo); sky = Math.max(sky, x + sub.hi!); }
        t.kids.push({ sub, x }); prev = x;
        if (!j) t.groups.push({ key: z, a: x, b: x }); else t.groups[t.groups.length - 1].b = x;
      });
    }
  }

  /** A node and all under it. Kid and group x run from this node's centre; lo/hi: reach below the tile; L/R: all of it. */
  function measure(id: string, lvl = 0): Measured {
    const d = graph.byId.get(id)!;
    const g = graph.guests.get(id) ?? [];
    const s0 = g.length ? spineRows(g, id, o) : null;
    const s = s0 && (s0.shown.length || s0.fold) ? s0 : null;
    let lo = Infinity, hi = -Infinity;
    if (s) { lo = -2; hi = TICK + 30 + Math.max(...s.shown.map(rowW), s.fold ? foldW(s.fold) : 0); }
    const t: Measured = { id, d, s, spineH: s ? M.start + 4 + (s.shown.length + (s.fold ? 1 : 0)) * ROW : 0, kids: [], groups: [], lo: null, hi: null, L: 0, R: 0 };
    flow(lvl < depthMax ? F.kids[id] ?? [] : [], lvl + 1, t);
    if (t.kids.length) {
      const mid = (t.kids[0].x + t.kids[t.kids.length - 1].x) / 2;
      for (const c of t.kids) { c.x -= mid; lo = Math.min(lo, c.x + c.sub.L); hi = Math.max(hi, c.x + c.sub.R); }
      for (const gr of t.groups) {
        gr.a -= mid; gr.b -= mid;
        if (t.groups.length > 1) hi = Math.max(hi, gr.a - M.tile / 2 + textW(label(gr.key), 12) + 6);
      }
    }
    const below = lo !== Infinity;
    return { ...t, lo: below ? lo : null, hi: below ? hi : null, L: Math.min(-COL / 2, below ? lo : 0), R: Math.max(COL / 2, below ? hi : 0) };
  }

  function place(t: Measured, cx: number, y: number, depth: number) {
    const bottom = y + M.block;
    nodes.push({ id: t.id, d: t.d, cx, y, depth, below: !!(t.s || t.kids.length), spineH: t.spineH });
    if (t.s) {
      const top = bottom + M.start;
      const items: Array<MapDevice | null> = [...t.s.shown, ...(t.s.fold ? [null] : [])];
      items.forEach((g, i) => {
        const ry = top + 4 + i * ROW; const mid = ry + ROW / 2; const k = g ? g.id : `fold:${t.id}`;
        rows.push(g ? { id: g.id, d: g, host: t.id, x: cx + TICK, y: ry } : { id: "", fold: t.s!.fold!, host: t.id, x: cx + TICK, y: ry });
        if (g) hostOf[g.id] = t.id;
        edgeD[k] = rounded([[cx, top], [cx, mid], [cx + TICK - 2, mid]]); edges.push({ k, d: edgeD[k] });
      });
    }
    if (!t.kids.length) return;
    // The bus runs from the first child to the last; zone labels sit under it, each over its group's tiles.
    const multi = t.kids.length > 1; const zl = t.groups.length > 1 ? M.zl : 0; const ys = bottom + M.start;
    const childTop = bottom + t.spineH + (multi ? M.bus : M.one) + zl; const bus = childTop - M.drop - zl;
    for (const c of t.kids) {
      const ccx = cx + c.x;
      edgeD[c.sub.id] = rounded(multi ? [[cx, ys], [cx, bus], [ccx, bus], [ccx, childTop]] : [[cx, ys], [ccx, childTop]]);
      edges.push({ k: c.sub.id, d: edgeD[c.sub.id] });
      place(c.sub, ccx, childTop, depth + 1);
    }
    if (zl) for (const gr of t.groups) labels.push({ key: gr.key, x: cx + gr.a - M.tile / 2, w: gr.b - gr.a + M.tile, y: bus + M.zly });
  }

  const loneTop = F.lone.length && F.roots.length ? 32 : 0;
  let x = 0;
  for (const t of F.roots.map((r) => measure(r))) { place(t, x - t.L, loneTop, 0); x += t.R - t.L + GGAP; }
  if (F.lone.length && F.roots.length) {
    // Beside the trees, in one row under a "Not connected" title.
    const lone = F.lone.map((r) => measure(r));
    const first = x - lone[0].L;
    for (const t of lone) { place(t, x - t.L, loneTop, 0); x += t.R - t.L; }
    labels.push({ key: "lone", x: first - M.tile / 2, y: 0, w: nodes[nodes.length - 1].cx - first + M.tile });
  } else if (F.lone.length) {
    placeLoneRows(F.lone);
  }

  /** Nothing is linked: the devices by zone, wrapped into rows no wider than o.wrap. */
  function placeLoneRows(ids: string[]) {
    const all = { kids: [] as Measured["kids"], groups: [] as Measured["groups"] };
    flow(ids, 0, all);
    const labelled = all.groups.length > 1;
    const wrapAt = Math.max(COL * 3, o.wrap ?? Infinity);
    let start = 0, top = 0;
    while (start < all.kids.length) {
      const x0 = all.kids[start].x + all.kids[start].sub.L;
      let end = start + 1;
      while (end < all.kids.length && all.kids[end].x + all.kids[end].sub.R - x0 <= wrapAt) end++;
      const tileTop = top + (labelled ? M.zl + 4 : 0);
      let bottom = tileTop;
      for (let i = start; i < end; i++) {
        const c = all.kids[i];
        place(c.sub, c.x - x0, tileTop, 0);
        bottom = Math.max(bottom, tileTop + M.block + c.sub.spineH);
      }
      if (labelled) {
        for (const gr of all.groups) {
          const inRow = all.kids.slice(start, end).filter((c) => c.x >= gr.a && c.x <= gr.b);
          if (!inRow.length) continue;
          const a = inRow[0].x - x0, b = inRow[inRow.length - 1].x - x0;
          labels.push({ key: gr.key, x: a - M.tile / 2, y: top, w: b - a + M.tile });
        }
      }
      top = bottom + WRAP_GAP; start = end;
    }
  }

  // Everything starts at x 0: shift right when a spine or label reaches left of it.
  const rowEnd = (r: LayoutRow) => r.x + 30 + (r.fold ? foldW(r.fold) : rowW(r.d!));
  const right = Math.max(0, ...nodes.map((n) => n.cx + COL / 2), ...rows.map(rowEnd), ...labels.map((l) => l.x + Math.max(l.w, textW(label(l.key), 12) + 6)));
  const h = Math.max(0, ...nodes.map((n) => n.y + M.block + n.spineH), ...rows.map((r) => r.y + ROW));
  const L: MapLayout = {
    nodes, rows, labels, edges, edgeD, parent: F.parent, hostOf, w: Math.ceil(right), h: Math.ceil(h + 4), block: M.block, tile: M.tile,
    nodeOf: new Map(nodes.map((n) => [n.id, n])), rowOf: new Map(rows.filter((r) => r.id).map((r) => [r.id, r])), nav: {},
  };
  L.nav = navOf(L);
  return L;
}

/** Arrow-key neighbours: up to the parent (or the guest above), down to the first guest or the nearest child, along a row. */
function navOf(L: MapLayout): MapLayout["nav"] {
  const nav: MapLayout["nav"] = {};
  const rowsAt = new Map<string, LayoutNode[]>();
  for (const n of L.nodes) { const k = `${n.depth}:${n.y}`; rowsAt.set(k, [...(rowsAt.get(k) ?? []), n]); }
  for (const list of rowsAt.values()) list.sort((a, b) => a.cx - b.cx);
  for (const n of L.nodes) {
    const row = rowsAt.get(`${n.depth}:${n.y}`)!; const i = row.indexOf(n);
    const guests = L.rows.filter((r) => r.host === n.id && r.id);
    const near = L.nodes.filter((c) => L.parent[c.id] === n.id).sort((a, b) => Math.abs(a.cx - n.cx) - Math.abs(b.cx - n.cx))[0];
    nav[n.id] = { up: L.parent[n.id] ?? "", down: guests[0]?.id ?? near?.id ?? "", left: row[i - 1]?.id ?? "", right: row[i + 1]?.id ?? "" };
  }
  for (const r of L.rows) {
    if (!r.id) continue;
    const sib = L.rows.filter((x) => x.host === r.host && x.id); const i = sib.indexOf(r);
    nav[r.id] = { up: sib[i - 1]?.id ?? r.host, down: sib[i + 1]?.id ?? "", left: r.host, right: "" };
  }
  return nav;
}
