"use client";

import { useEffect, useState } from "react";

// A container's app icon from the service icon set (public/service-icons), picked by its image and name.

/** Images whose repository name is not the app's icon name. */
const ALIASES: Record<string, string> = {
  "pms-docker": "plex",
  "portainer-ce": "portainer",
  "portainer-ee": "portainer",
  "pgvecto-rs": "postgres",
  "postgresql": "postgres",
  "eclipse-mosquitto": "mosquitto",
  "server": "",
};

const strip = (v: string) => v.toLowerCase().replace(/[^a-z0-9._-]/g, "");

/**
 * The icon key for a container, or null for the plain box. Tries the image's repository ("jellyfin"), the container
 * name, the repository's namespace ("vaultwarden/server" → "vaultwarden"), then the first word of each
 * ("immich-server" → "immich").
 */
export function iconKeyFor(image: string, name: string, index: ReadonlySet<string>): string | null {
  const ref = (image || "").replace(/@sha256:.*$/, "").replace(/:[^:/]+$/, "");
  const parts = ref.split("/");
  const repo = strip(parts.pop() ?? "");
  const space = strip(parts.pop() ?? "");
  const nm = strip(name || "");
  const first = (v: string) => v.split(/[-_.]/)[0] ?? "";
  const tries = [repo, nm, space, first(repo), first(nm)].flatMap((v) => (v in ALIASES ? [ALIASES[v]] : [v]));
  for (const key of tries) {
    if (key && key.length > 1 && index.has(key)) return key;
  }
  return null;
}

let indexPromise: Promise<ReadonlySet<string>> | null = null;

/** The list of icon names, fetched once per page load. Empty while loading or when the list is missing. */
export function useIconIndex(): ReadonlySet<string> {
  const [index, setIndex] = useState<ReadonlySet<string>>(() => new Set());
  useEffect(() => {
    let live = true;
    indexPromise ??= fetch("/service-icons/index.json")
      .then((r) => (r.ok ? r.json() : []))
      .then((list: unknown) => new Set(Array.isArray(list) ? list.filter((v): v is string => typeof v === "string") : []))
      .catch(() => new Set<string>());
    void indexPromise.then((set) => { if (live) setIndex(set); });
    return () => { live = false; };
  }, []);
  return index;
}
