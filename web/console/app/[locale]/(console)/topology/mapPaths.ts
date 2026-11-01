// Lines on the Map: rounded orthogonal paths, a device's path to the top of its tree, and the dashed connections
// (depends on, a second link) drawn while one of their ends is selected.
import type { MapLayout, LayoutRow } from "./mapLayout";
import type { MapLink } from "./mapModel";

type Pt = [number, number];
const ROW = 28;
const NAME_MAX = 100;

/** SVG path through orthogonal points with rounded corners (8 px). */
export function rounded(raw: Pt[], r = 8): string {
  const p = raw
    .filter((q, i) => !i || q[0] !== raw[i - 1][0] || q[1] !== raw[i - 1][1])
    .filter((q, i, a) => i === 0 || i === a.length - 1 || !((a[i - 1][0] === q[0] && q[0] === a[i + 1][0]) || (a[i - 1][1] === q[1] && q[1] === a[i + 1][1])));
  const f = (n: number) => (Math.round(n * 10) / 10).toString();
  let d = `M${f(p[0][0])} ${f(p[0][1])}`;
  for (let i = 1; i < p.length - 1; i++) {
    const [a, b, c] = [p[i - 1], p[i], p[i + 1]];
    const rr = Math.min(r, Math.hypot(b[0] - a[0], b[1] - a[1]) / 2, Math.hypot(c[0] - b[0], c[1] - b[1]) / 2);
    const to = (from: Pt, t: Pt): Pt => {
      const n = Math.hypot(t[0] - from[0], t[1] - from[1]) || 1;
      return [from[0] + ((t[0] - from[0]) / n) * rr, from[1] + ((t[1] - from[1]) / n) * rr];
    };
    const s = to(b, a), e = to(b, c);
    d += ` L${f(s[0])} ${f(s[1])} Q${f(b[0])} ${f(b[1])} ${f(e[0])} ${f(e[1])}`;
  }
  const z = p[p.length - 1];
  return `${d} L${f(z[0])} ${f(z[1])}`;
}

/** Edge keys from a node (or guest) up to its root: its path to the router. */
export function routeOf(L: MapLayout, id: string): string[] {
  const out: string[] = [];
  let k = id;
  if (L.hostOf[k]) { out.push(k); k = L.hostOf[k]; }
  for (let i = 0; k && L.edgeD[k] && i < 64; i++) { out.push(k); k = L.parent[k]; }
  return out;
}

type Anchor =
  | { node: true; cx: number; top: number; bottom: number; below: boolean }
  | { node: false; host: string; left: number; right: number; y: number };

/** Right end of a guest row's text (and its problem dot). */
const rowRight = (r: LayoutRow) => r.x + 4 + 16 + 6 + (r.fold ? 60 : Math.min(NAME_MAX, (r.d?.name.length ?? 0) * 7.1) + (r.d?.tone === "ok" ? 0 : 14)) + 6;

/** Where a dashed connection meets a node (under its labels) or a guest row. Hidden guests use their host's fold row. */
function anchor(L: MapLayout, id: string, hostOfId: (id: string) => string): Anchor | null {
  const n = L.nodeOf.get(id);
  if (n) return { node: true, cx: n.cx, top: n.y - 4, bottom: n.y + L.block + 4, below: n.below };
  const r = L.rowOf.get(id) ?? L.rows.find((x) => !x.id && x.host === hostOfId(id));
  if (!r) return null;
  return { node: false, host: r.host, left: r.x - 2, right: rowRight(r), y: r.y + ROW / 2 };
}

type NodeA = Extract<Anchor, { node: true }>;
type RowA = Extract<Anchor, { node: false }>;
const side = (N: NodeA, x: number) => (N.below ? Math.sign(x - N.cx || 1) * 16 : 0);

function toNode(R: RowA, N: NodeA, L: MapLayout): Pt[] {
  const above = R.y > N.bottom; const out = above ? N.cx > R.left + 24 : N.cx >= R.right; const sx = out ? R.right : R.left;
  const nx = N.cx + (above ? side(N, sx) : 0); const host = L.nodeOf.get(R.host);
  // A link to a device in its host's row leaves the spine beside it and runs along the gap under that row's labels.
  if (above && host && N.bottom === host.y + L.block + 4) {
    const lane = out ? Math.max(sx, ...L.rows.filter((r) => r.host === R.host && r.y + ROW / 2 < R.y).map(rowRight)) + 10 : host.cx - 10;
    const gy = N.bottom + 3;
    return [[sx, R.y], [lane, R.y], [lane, gy], [nx, gy], [nx, N.bottom]];
  }
  return [[sx, R.y], [nx, R.y], [nx, above ? N.bottom : N.top]];
}

function connect(A: Anchor, B: Anchor, L: MapLayout): Pt[] {
  if (!A.node && B.node) return toNode(A, B, L);
  if (A.node && !B.node) return toNode(B, A, L).reverse();
  if (!A.node && !B.node) {
    if (A.host === B.host) { const x = Math.max(A.right, B.right) + 12; return [[A.right, A.y], [x, A.y], [x, B.y], [B.right, B.y]]; }
    const [P, Q] = A.right < B.left ? [A, B] : [B, A]; const mx = (P.right + Q.left) / 2;
    return [[P.right, P.y], [mx, P.y], [mx, Q.y], [Q.left, Q.y]];
  }
  const a = A as NodeA, b = B as NodeA;
  const ax = a.cx + side(a, b.cx), bx = b.cx + side(b, a.cx); const y = Math.max(a.bottom, b.bottom) + 10;
  return [[ax, a.bottom], [ax, y], [bx, y], [bx, b.bottom]];
}

/** The selected device's connections that are not tree lines (depends on, a second link), as dashed paths. */
export function extraPaths(L: MapLayout, links: MapLink[], id: string, hostOfId: (id: string) => string): string[] {
  if (!id) return [];
  const out: string[] = [];
  for (const l of links) {
    if (l.a !== id && l.b !== id) continue;
    if (L.parent[l.b] === l.a || L.parent[l.a] === l.b || L.hostOf[l.a] === l.b || L.hostOf[l.b] === l.a) continue;
    const A = anchor(L, l.a, hostOfId), B = anchor(L, l.b, hostOfId);
    if (!A || !B) continue;
    out.push(rounded(connect(A, B, L)));
  }
  return out;
}
