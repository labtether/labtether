"use client";

import { useEffect, useState } from "react";

/** What removing a device also removes, as the Hub works it out (GET /assets/{id}/delete-preview). */
export type DeletePreview = {
  devices: number;
  connection: boolean;
  saved_login: boolean;
};

/** Loads the removal preview while the Remove dialog is open; null until it arrives or when it cannot be read. */
export function useDeletePreview(open: boolean, assetId?: string): DeletePreview | null {
  const [preview, setPreview] = useState<DeletePreview | null>(null);
  useEffect(() => {
    if (!open || !assetId) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    fetch(`/api/assets/${encodeURIComponent(assetId)}/delete-preview`, { cache: "no-store" })
      .then((res) => (res.ok ? (res.json() as Promise<DeletePreview>) : null))
      .then((data) => {
        if (!cancelled) setPreview(data && typeof data.devices === "number" ? data : null);
      })
      .catch(() => {
        if (!cancelled) setPreview(null);
      });
    return () => {
      cancelled = true;
    };
  }, [open, assetId]);
  return preview;
}
