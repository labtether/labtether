"use client";

import { EthernetPort, Router, Wifi, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "../../../../i18n/navigation";
import { deviceIcon } from "../nodes/deviceListModel";
import { COL, type LayoutNode, type LayoutRow, type MapLayout } from "./mapLayout";
import { hitOf, type MapDevice, type MapGraph } from "./mapModel";
import { extraPaths, routeOf } from "./mapPaths";
import type { MapWords } from "./useMapWords";

const NET_ICON: Record<string, LucideIcon> = { router: Router, switch: EthernetPort, ap: Wifi };

/** What the device is: a router, switch or access point first, else the Devices list's drawing. */
export function mapIcon(d: MapDevice): LucideIcon {
  return NET_ICON[d.net] ?? deviceIcon(d.asset);
}

/** The device's kind in a quiet tile, a status dot on its corner only when something is wrong. */
export function MapTile({ d, size }: { d: MapDevice; size: number }) {
  const Icon = mapIcon(d);
  return (
    <span className="mp-t" aria-hidden="true">
      <Icon width={size} height={size} strokeWidth={1.75} />
      {d.tone !== "ok" && <i className={`mp-b ${d.tone}`} />}
    </span>
  );
}

type WorldProps = {
  L: MapLayout;
  graph: MapGraph;
  words: MapWords;
  /** The Dashboard picture: tiles open their device, nothing else reacts. */
  compact?: boolean;
  sel?: string;
  /** Find text (lower case): matches stand out, the rest dims. */
  q?: string;
  /** Hovered or focused device: its path to the top drawn strong. */
  hl?: string;
  onSelect?: (id: string) => void;
  onFold?: (host: string) => void;
  /** A node's ⋯ menu (the full Map only). */
  nodeMenu?: (d: MapDevice) => ReactNode;
};

/** The tree: lines, zone labels, device tiles and guest rows, in world pixels. */
export function MapWorld({ L, graph, words, compact = false, sel = "", q = "", hl = "", onSelect, onFold, nodeMenu }: WorldProps) {
  const hostOfId = (id: string) => graph.byId.get(id)?.host ?? "";
  const route = (id: string) => (id ? routeOf(L, id).map((k) => L.edgeD[k]) : []);
  const dashed = compact ? [] : extraPaths(L, graph.links, sel, hostOfId);
  const strong = route(hl || sel);
  return (
    <>
      <svg className="mp-links" width={L.w} height={L.h} aria-hidden="true">
        <g className="mp-base">{L.edges.map((e) => <path key={e.k} d={e.d} />)}</g>
        <g className="mp-hl">{strong.map((d, i) => <path key={i} d={d} />)}</g>
        <g className="mp-dep">{dashed.map((d, i) => <path key={i} d={d} />)}</g>
      </svg>
      {L.labels.map((l) => (
        <p key={`${l.key}:${l.x}:${l.y}`} className="mp-zl" style={{ left: l.x, top: l.y, width: l.w }}>
          <span>{words.label(l.key)}</span>
        </p>
      ))}
      {L.nodes.map((n) => (compact
        ? <CompactNode key={n.id} n={n} />
        : <Node key={n.id} n={n} L={L} words={words} sel={sel} q={q} graph={graph} onSelect={onSelect} menu={nodeMenu?.(n.d)} />))}
      {L.rows.map((r) => (compact
        ? <CompactRow key={r.id || `fold:${r.host}`} r={r} words={words} />
        : <Row key={r.id || `fold:${r.host}`} r={r} words={words} sel={sel} q={q} onSelect={onSelect} onFold={onFold} />))}
    </>
  );
}

function Node({ n, L, words, sel, q, graph, onSelect, menu }: {
  n: LayoutNode; L: MapLayout; words: MapWords; sel: string; q: string; graph: MapGraph;
  onSelect?: (id: string) => void; menu?: ReactNode;
}) {
  const d = n.d;
  const short = words.short(d);
  const long = words.long(d);
  const hit = q && (hitOf(d, q) || (graph.guests.get(d.id) ?? []).some((g) => hitOf(g, q)));
  const cls = `mp-n${sel === d.id ? " sel" : ""}${q ? (hit ? " hit" : " dim") : ""}${n.cx > L.w / 2 ? " mr" : ""}`;
  return (
    <div className={cls} data-node={d.id} style={{ left: n.cx - COL / 2, top: n.y }}>
      <button
        type="button"
        className="mp-hit"
        data-id={d.id}
        onClick={() => onSelect?.(d.id)}
        aria-label={`${d.name}, ${long}`}
        aria-current={sel === d.id ? "true" : undefined}
      >
        <MapTile d={d} size={18} />
        <span className="mp-nm" title={d.name}>{d.name}</span>
        {short && <span className={`mp-m ${d.tone}`} title={long}>{short}</span>}
      </button>
      {menu && <div className="mp-nm-more">{menu}</div>}
    </div>
  );
}

function Row({ r, words, sel, q, onSelect, onFold }: {
  r: LayoutRow; words: MapWords; sel: string; q: string; onSelect?: (id: string) => void; onFold?: (host: string) => void;
}) {
  const style = { left: r.x, top: r.y };
  if (!r.d) {
    return (
      <button type="button" className="mp-g mp-more" style={style} onClick={() => onFold?.(r.host)} aria-expanded={r.fold === "less"}>
        {words.fold(r.fold!)}
      </button>
    );
  }
  const g = r.d;
  const Icon = mapIcon(g);
  const cls = `mp-g${sel === g.id ? " sel" : ""}${q ? (hitOf(g, q) ? " hit" : " dim") : ""}`;
  return (
    <button
      type="button"
      className={cls}
      data-id={g.id}
      style={style}
      onClick={() => onSelect?.(g.id)}
      aria-label={`${g.name}, ${words.long(g)}`}
      aria-current={sel === g.id ? "true" : undefined}
      title={g.tone !== "ok" ? words.long(g) : undefined}
    >
      <Icon width={16} height={16} strokeWidth={1.75} aria-hidden="true" />
      <span className={`mp-gn ${g.tone}`}>{g.name}</span>
      {g.tone !== "ok" && <i className={`mp-gd ${g.tone}`} aria-hidden="true" />}
    </button>
  );
}

/** The Dashboard picture: each tile opens its device (mouse only; the picture is one image to assistive tech). */
function CompactNode({ n }: { n: LayoutNode }) {
  const d = n.d;
  return (
    <Link href={`/nodes/${encodeURIComponent(d.id)}`} className="mp-n" tabIndex={-1} aria-hidden="true" style={{ left: n.cx - COL / 2, top: n.y }}>
      <MapTile d={d} size={16} />
      <span className={`mp-nm ${d.tone}`} title={d.name}>{d.name}</span>
    </Link>
  );
}

function CompactRow({ r, words }: { r: LayoutRow; words: MapWords }) {
  const style = { left: r.x, top: r.y };
  if (!r.d) return <span className="mp-g mp-more" style={style}>{words.fold(r.fold!)}</span>;
  const g = r.d;
  const Icon = mapIcon(g);
  return (
    <Link href={`/nodes/${encodeURIComponent(g.id)}`} className="mp-g" tabIndex={-1} aria-hidden="true" style={style}>
      <Icon width={16} height={16} strokeWidth={1.75} />
      <span className={`mp-gn ${g.tone}`}>{g.name}</span>
      {g.tone !== "ok" && <i className={`mp-gd ${g.tone}`} />}
    </Link>
  );
}
