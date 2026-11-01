"use client";

import { forwardRef, type ReactNode } from "react";
import type { Tone } from "./healthModel";

/** A Health sub-section: its heading with one fact or control on the right, then its list. */
export const Section = forwardRef<HTMLElement, { id: string; title: string; right?: ReactNode; children: ReactNode }>(
  function Section({ id, title, right, children }, ref) {
    return (
      <section ref={ref} className="pp-sec" aria-labelledby={id}>
        <div className="pp-sh">
          <h2 id={id}>{title}</h2>
          {right ? <div className="pp-sh-r">{right}</div> : null}
        </div>
        {children}
      </section>
    );
  },
);

/** A status dot and its word. No dot for plain facts. */
export function Status({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span className={`pp-st ${tone}`}>
      {tone !== "none" ? <i className={`pp-dot ${tone}`} aria-hidden="true" /> : null}
      {children}
    </span>
  );
}
