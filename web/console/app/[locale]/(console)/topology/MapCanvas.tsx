"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ROW, type MapLayout } from "./mapLayout";
import type { MapDevice, MapGraph } from "./mapModel";
import { MapWorld } from "./MapWorld";
import type { MapWords } from "./useMapWords";

// Fit never goes under 80 %: names stay readable (about 10 px) and a wider lab pans sideways instead.
const ZMIN = 0.4, ZMAX = 2, FIT_MIN = 0.8, EDGE = 24;
const clampZ = (z: number) => Math.max(ZMIN, Math.min(ZMAX, z));
const NAV_KEYS: Record<string, "up" | "down" | "left" | "right"> = { ArrowUp: "up", ArrowDown: "down", ArrowLeft: "left", ArrowRight: "right" };
type Rect = { x: number; y: number; w: number; h: number };

type CanvasProps = {
  L: MapLayout;
  graph: MapGraph;
  words: MapWords;
  sel: string;
  q: string;
  /** A side panel is open: the canvas narrows and the selected device moves to the middle of what is left. */
  side: boolean;
  /** Bump to fit the tree to the width again. */
  fitKey: number;
  onSelect: (id: string) => void;
  onClose: () => void;
  onFold: (host: string) => void;
  nodeMenu?: (d: MapDevice) => ReactNode;
};

/**
 * The Map's canvas. Pan by dragging or scrolling, zoom with ctrl/⌘ + wheel or the controls, f fits, arrows walk the
 * tree. It opens fitted to the stage width (so opening a panel never shrinks the text) and keeps fitting while the data
 * settles, until the person moves, zooms or picks something.
 */
export function MapCanvas({ L, graph, words, sel, q, side, fitKey, onSelect, onClose, onFold, nodeMenu }: CanvasProps) {
  const { t } = words;
  const canvasRef = useRef<HTMLDivElement>(null);
  const worldRef = useRef<HTMLDivElement>(null);
  const view = useRef({ x: 0, y: EDGE, z: 1, touched: false });
  const [hl, setHl] = useState("");

  const apply = useCallback(() => {
    const w = worldRef.current; if (!w) return;
    const v = view.current;
    w.style.transform = `translate(${v.x}px, ${v.y}px) scale(${v.z})`;
  }, []);

  const fit = useCallback(() => {
    const c = canvasRef.current; if (!c || !c.clientWidth) return;
    const v = view.current;
    const stageW = c.closest(".mp-stage")?.clientWidth || c.clientWidth;
    v.z = Math.max(FIT_MIN, Math.min(1, (stageW - 32) / Math.max(1, L.w)));
    v.x = Math.round((c.clientWidth - L.w * v.z) / 2);
    v.y = Math.max(EDGE, Math.round((c.clientHeight - L.h * v.z) / 2));
    apply();
  }, [L.w, L.h, apply]);

  const zoomAt = useCallback((f: number, cx: number, cy: number) => {
    const v = view.current; const z = clampZ(v.z * f);
    v.x = cx - (cx - v.x) * (z / v.z); v.y = cy - (cy - v.y) * (z / v.z); v.z = z; v.touched = true;
    apply();
  }, [apply]);

  const rectOf = useCallback((id: string): Rect | null => {
    const n = L.nodeOf.get(id); if (n) return { x: n.cx - 56, y: n.y, w: 112, h: L.block };
    const r = L.rowOf.get(id); return r ? { x: r.x, y: r.y, w: 140, h: ROW } : null;
  }, [L]);

  /** Pan just enough that r plus 24 px stays inside the canvas. */
  const reveal = useCallback((r: Rect | null) => {
    const c = canvasRef.current; if (!r || !c?.clientWidth) return;
    const v = view.current; const cw = c.clientWidth, ch = c.clientHeight;
    const right = (r.x + r.w) * v.z + v.x + EDGE; if (right > cw) v.x -= right - cw;
    const left = r.x * v.z + v.x; if (left < EDGE) v.x += EDGE - left;
    const bottom = (r.y + r.h) * v.z + v.y + EDGE; if (bottom > ch) v.y -= bottom - ch;
    const top = r.y * v.z + v.y; if (top < EDGE) v.y += EDGE - top;
    apply();
  }, [apply]);

  /** Bring r to the middle across, keeping the tree's ends within 24 px of the canvas edges. */
  const centre = useCallback((r: Rect | null) => {
    const c = canvasRef.current; if (!r || !c?.clientWidth) return;
    const v = view.current; const cw = c.clientWidth; const want = cw / 2 - (r.x + r.w / 2) * v.z;
    const lo = Math.min(EDGE, cw - EDGE - L.w * v.z), hi = Math.max(EDGE, cw - EDGE - L.w * v.z);
    v.x = Math.round(Math.max(lo, Math.min(hi, want))); reveal(r);
  }, [L.w, reveal]);

  // Fit while untouched (data arriving changes the size), and whenever asked.
  useLayoutEffect(() => { if (!view.current.touched) fit(); }, [fit]);
  useEffect(() => { if (fitKey) { view.current.touched = false; fit(); } }, [fitKey]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const c = canvasRef.current; if (!c || typeof ResizeObserver !== "function") return;
    const ro = new ResizeObserver(() => { if (!view.current.touched) fit(); });
    ro.observe(c);
    return () => ro.disconnect();
  }, [fit]);

  // A panel opening brings the selected device to the middle of what is left; picking another reveals it.
  const sideWas = useRef(side);
  const selWas = useRef(sel);
  useEffect(() => {
    if (sel) view.current.touched = true;
    if (side && !sideWas.current && sel) centre(rectOf(sel));
    else if (sel && sel !== selWas.current) reveal(rectOf(sel));
    else if (!side && sideWas.current && !sel) { view.current.touched = false; fit(); }
    sideWas.current = side; selWas.current = sel;
  }, [sel, side, centre, reveal, rectOf, fit]);

  // Wheel: ctrl/⌘ zooms around the pointer, otherwise it pans. Needs a non-passive listener.
  useEffect(() => {
    const c = canvasRef.current; if (!c) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const v = view.current; v.touched = true;
      if (e.ctrlKey || e.metaKey) { const r = c.getBoundingClientRect(); zoomAt(Math.exp(-e.deltaY * 0.01), e.clientX - r.left, e.clientY - r.top); return; }
      v.x -= e.deltaX; v.y -= e.deltaY; apply();
    };
    c.addEventListener("wheel", onWheel, { passive: false });
    return () => c.removeEventListener("wheel", onWheel);
  }, [apply, zoomAt]);

  // Drag anywhere to pan; more than 4 px is a pan, not a click. A click on empty canvas closes the panel.
  const drag = useRef<{ sx: number; sy: number; x0: number; y0: number; moved: boolean; pid: number; empty: boolean } | null>(null);
  const swallowUntil = useRef(0);
  const onPointerDown = (e: React.PointerEvent) => {
    const el = e.target as HTMLElement;
    if (e.button !== 0 || el.closest(".mp-zoom, .mp-nm-more, .mp-find")) return;
    const v = view.current;
    drag.current = { sx: e.clientX, sy: e.clientY, x0: v.x, y0: v.y, moved: false, pid: e.pointerId, empty: !el.closest(".mp-hit, .mp-g") };
  };
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current; if (!d || e.pointerId !== d.pid) return;
    const dx = e.clientX - d.sx, dy = e.clientY - d.sy;
    if (!d.moved) {
      if (Math.hypot(dx, dy) < 4) return;
      d.moved = true; view.current.touched = true;
      try { canvasRef.current?.setPointerCapture(e.pointerId); } catch { /* pointer already gone */ }
      canvasRef.current?.classList.add("panning");
    }
    view.current.x = d.x0 + dx; view.current.y = d.y0 + dy; apply();
  };
  const onPointerEnd = (e: React.PointerEvent) => {
    const d = drag.current; if (!d || e.pointerId !== d.pid) return;
    drag.current = null; canvasRef.current?.classList.remove("panning");
    if (d.moved) { swallowUntil.current = Date.now() + 350; return; }
    if (d.empty && e.type === "pointerup" && sel) { onClose(); swallowUntil.current = Date.now() + 350; }
  };

  const idAt = (el: EventTarget | null) => ((el as HTMLElement | null)?.closest?.("[data-id]") as HTMLElement | null)?.dataset.id ?? "";
  const focusId = (id: string) => (canvasRef.current?.querySelector(`[data-id="${CSS.escape(id)}"]`) as HTMLElement | null)?.focus({ preventScroll: true });

  // Keyboard: arrows walk the tree (moving the selection with them while a device is open), f fits, + and − zoom.
  const onKeyDown = (e: React.KeyboardEvent) => {
    const el = e.target as HTMLElement;
    if (el.matches("input, select, textarea") || e.metaKey || e.ctrlKey || e.altKey || el.closest(".mp-nm-more")) return;
    const id = idAt(el); const dir = NAV_KEYS[e.key];
    if (id && dir) {
      e.preventDefault();
      const next = L.nav[id]?.[dir]; if (!next) return;
      if (sel) onSelect(next);
      focusId(next); reveal(rectOf(next));
      return;
    }
    const c = canvasRef.current; if (!c) return;
    if (e.key === "f") { e.preventDefault(); view.current.touched = false; fit(); }
    else if (e.key === "+" || e.key === "=") { e.preventDefault(); zoomAt(1.25, c.clientWidth / 2, c.clientHeight / 2); }
    else if (e.key === "-") { e.preventDefault(); zoomAt(0.8, c.clientWidth / 2, c.clientHeight / 2); }
  };

  const zoomBy = (f: number) => { const c = canvasRef.current; if (c) zoomAt(f, c.clientWidth / 2, c.clientHeight / 2); };

  return (
    <div
      ref={canvasRef}
      className="mp-canvas"
      role="region"
      aria-label={t("canvas")}
      aria-describedby="mp-keys"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerEnd}
      onPointerCancel={onPointerEnd}
      onClickCapture={(e) => { if (Date.now() < swallowUntil.current) { e.stopPropagation(); e.preventDefault(); } }}
      onKeyDown={onKeyDown}
    >
      <div
        ref={worldRef}
        className="mp-world"
        style={{ width: L.w, height: L.h }}
        onPointerOver={(e) => { if (!drag.current?.moved) setHl(idAt(e.target)); }}
        onPointerLeave={() => setHl("")}
        onFocus={(e) => { const id = idAt(e.target); setHl(id); reveal(rectOf(id)); }}
        onBlur={() => setHl("")}
      >
        <MapWorld
          L={L} graph={graph} words={words} sel={sel} q={q} hl={hl}
          onSelect={(id) => { view.current.touched = true; onSelect(id); }}
          onFold={(h) => { view.current.touched = true; onFold(h); }}
          nodeMenu={nodeMenu}
        />
      </div>
      <div className="mp-zoom" role="group" aria-label={t("zoom.label")}>
        <button type="button" className="mp-zb" onClick={() => zoomBy(0.8)} aria-label={t("zoom.out")}>−</button>
        <button type="button" className="mp-zb" onClick={() => zoomBy(1.25)} aria-label={t("zoom.in")}>+</button>
        <i className="mp-zs" aria-hidden="true" />
        <button type="button" className="mp-zb mp-zfit" onClick={() => { view.current.touched = false; fit(); }} title={t("zoom.fitTip")}>{t("zoom.fit")}</button>
      </div>
      <p className="sr-only" id="mp-keys">{t("keys")}</p>
    </div>
  );
}
