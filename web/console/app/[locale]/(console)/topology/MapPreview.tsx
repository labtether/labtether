"use client";

import { useTranslations } from "next-intl";
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { DashSection } from "../../../components/dashboard/DashSections";
import { useRouter } from "../../../../i18n/navigation";
import { layoutMap } from "./mapLayout";
import { useMapGraph } from "./useMapGraph";
import { useMapWords } from "./useMapWords";
import { MapWorld } from "./MapWorld";

// The picture shows the router, the switch and the first tier at a readable 78 % or more; a wider lab scrolls sideways.
const MAX = 1, MIN = 0.78, DEPTH = 2;

/** The Dashboard's small map: the Map's tree, compact. A tile opens its device; anywhere else opens the Map. */
export function MapPreview() {
  const t = useTranslations("dashboard");
  const router = useRouter();
  const { graph, isLoading } = useMapGraph({ edges: false });
  const words = useMapWords(graph);
  // Nothing linked: the rows wrap so the picture fits its box at the smallest readable scale.
  const [boxW, setBoxW] = useState(1100);
  const wrap = Math.max(400, Math.floor((boxW - 40) / MIN / 80) * 80);
  const L = useMemo(() => layoutMap(graph, { compact: true, depth: DEPTH, wrap, labelText: words.label }), [graph, wrap, words.label]);
  const boxRef = useRef<HTMLDivElement>(null);
  const worldRef = useRef<HTMLDivElement>(null);

  // Scale to the box and again when it changes size. Too wide at the smallest readable scale: scroll sideways,
  // opened on the middle (the router).
  useLayoutEffect(() => {
    const box = boxRef.current, w = worldRef.current; if (!box || !w) return;
    let centred = false;
    const fit = () => {
      if (box.clientWidth) setBoxW(box.clientWidth);
      const pad = box.clientWidth < 600 ? 12 : 20;
      const s = Math.max(box.clientWidth < 600 ? 0.5 : MIN, Math.min(MAX, (box.clientWidth - 2 * pad) / Math.max(1, L.w)));
      const wide = L.w * s + 2 * pad > box.clientWidth + 1;
      Object.assign(w.style, wide
        ? { left: `${pad}px`, top: `${pad}px`, transformOrigin: "0 0", transform: `scale(${s})` }
        : { left: "50%", top: `${pad}px`, transformOrigin: "50% 0", transform: `translateX(-50%) scale(${s})` });
      box.style.height = `${Math.round(L.h * s + 2 * pad)}px`;
      box.style.overflowX = wide ? "auto" : "hidden";
      if (wide && !centred) { box.scrollLeft = Math.round((L.w * s + 2 * pad - box.clientWidth) / 2); centred = true; }
    };
    fit();
    if (typeof ResizeObserver !== "function") return;
    const ro = new ResizeObserver(fit);
    ro.observe(box);
    return () => ro.disconnect();
  }, [L.w, L.h]);

  if (isLoading && !graph.devices.length) return null;
  if (!graph.devices.length) return null;
  const problems = graph.devices.filter((d) => d.tone !== "ok").map((d) => `${d.name}: ${words.short(d)}`);
  const label = [words.t("mini", { count: graph.devices.length }), ...problems].join(". ");

  return (
    <DashSection id="db-map" title={t("sections.map")} more={{ href: "/topology", label: t("links.openMap") }}>
      <div
        ref={boxRef}
        className="mp-mini"
        onClick={(e) => { if (!(e.target as HTMLElement).closest("a")) router.push("/topology"); }}
      >
        <div ref={worldRef} className="mp-mini-w" role="img" aria-label={label} style={{ width: L.w, height: L.h }}>
          <MapWorld L={L} graph={graph} words={words} compact />
        </div>
      </div>
    </DashSection>
  );
}
