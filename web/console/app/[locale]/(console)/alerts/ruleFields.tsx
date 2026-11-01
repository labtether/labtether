"use client";

import type { ReactNode } from "react";

/** A labelled field; its error (when shown) sits under it and is tied to the control by id. */
export function Field({ id, label, error, wide, children }: { id: string; label: string; error?: string; wide?: boolean; children: ReactNode }) {
  return (
    <div className={`al-field${wide ? " al-wide" : ""}`}>
      <label htmlFor={id}>{label}</label>
      {children}
      {error ? <p className="al-hint bad" id={`${id}-err`}>{error}</p> : null}
    </div>
  );
}

/** Props for an input that may be invalid: aria-invalid and the error's id. */
export const invalid = (id: string, error?: string) =>
  error ? { "aria-invalid": true as const, "aria-describedby": `${id}-err` } : {};

/** A number from an input; NaN while it is empty, so the check can say so. */
export const numberValue = (raw: string) => (raw.trim() === "" ? NaN : Number(raw));
export const shownNumber = (n: number) => (Number.isNaN(n) ? "" : String(n));
