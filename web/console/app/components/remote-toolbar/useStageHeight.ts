"use client";

import { useEffect, useState, type RefObject } from "react";

const BOTTOM_GAP_PX = 16;
const MIN_PX = 320;

/**
 * The remote screen takes the rest of the window below where it starts, so it is as large as the page allows without
 * scrolling. Measured on mount, on window resize and when the page's layout changes; null before the first measurement
 * (the stylesheet's fallback height applies).
 */
export function useStageHeight(stageRef: RefObject<HTMLElement | null>, active: boolean): number | null {
  const [height, setHeight] = useState<number | null>(null);

  useEffect(() => {
    if (!active) return undefined;
    const stage = stageRef.current;
    if (!stage) return undefined;
    let frame = 0;
    const measure = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const top = stage.getBoundingClientRect().top + window.scrollY;
        const next = Math.max(MIN_PX, Math.round(window.innerHeight - top - BOTTOM_GAP_PX));
        setHeight((prev) => (prev !== null && Math.abs(prev - next) < 2 ? prev : next));
      });
    };
    measure();
    window.addEventListener("resize", measure);
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(measure) : null;
    // The shell (its row can wrap) and the page (a header above can load late) both move the screen's top.
    if (observer && stage.parentElement) observer.observe(stage.parentElement);
    if (observer) observer.observe(document.body);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", measure);
      observer?.disconnect();
    };
  }, [active, stageRef]);

  return height;
}
