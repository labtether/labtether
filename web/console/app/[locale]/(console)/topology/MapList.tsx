"use client";

import { Link } from "../../../../i18n/navigation";
import { byZone, hitOf, type MapDevice, type MapGraph } from "./mapModel";
import { MapTile } from "./MapWorld";
import type { MapWords } from "./useMapWords";

/**
 * The same tree as an indented list (the phone's default): router, switch, the devices under it by zone, each host's
 * guests. Each row opens its device; a second word only for a problem.
 */
export function MapList({ graph, words, q }: { graph: MapGraph; words: MapWords; q: string }) {
  const F = graph.forest;
  const item = (d: MapDevice, sub: React.ReactNode = null) => {
    const short = words.short(d);
    const cls = !q ? "" : hitOf(d, q) ? " hit" : " dim";
    return (
      <li key={d.id}>
        <Link className={`mp-ti${cls}`} href={`/nodes/${encodeURIComponent(d.id)}`}>
          <MapTile d={d} size={14} />
          <span className="mp-ti-nm">{d.name}</span>
          {short && <span className={`mp-ti-prob ${d.tone}`} title={words.long(d)}>{short}</span>}
        </Link>
        {sub}
      </li>
    );
  };
  const groups = (ids: string[]) => {
    const g = byZone(graph, ids);
    return g.map(([z, list]) => [
      g.length > 1 ? <li key={`z:${z}`} className="mp-tz">{words.label(z)}</li> : null,
      ...list.map(node),
    ]);
  };
  function node(id: string): React.ReactNode {
    const d = graph.byId.get(id)!;
    const guests = graph.guests.get(id) ?? [];
    const kids = F.kids[id] ?? [];
    const inner = guests.length || kids.length ? <ul>{guests.map((g) => item(g))}{groups(kids)}</ul> : null;
    return item(d, inner);
  }
  return (
    <nav className="mp-tree" aria-label={words.t("view.listLabel")}>
      <ul>
        {F.roots.map(node)}
        {F.lone.length > 0 && F.roots.length > 0 && <li className="mp-tz">{words.label("lone")}</li>}
        {F.roots.length > 0 ? F.lone.map(node) : groups(F.lone)}
      </ul>
    </nav>
  );
}
