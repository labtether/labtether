export const recentsStorageKey = "labtether.terminal.recentTargets";

export interface RecentTarget {
  id: string;
  name: string;
  type: "device" | "container";
  lastConnected: string; // ISO 8601
}

export function loadRecentTargets(): RecentTarget[] {
  try {
    const raw = window.localStorage.getItem(recentsStorageKey);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // Migrate old format (string[]) to new format (RecentTarget[])
    return parsed
      .map((entry: unknown) => {
        if (typeof entry === "string") {
          return { id: entry, name: "", type: "device" as const, lastConnected: "" };
        }
        if (entry && typeof entry === "object" && "id" in entry) {
          const obj = entry as Record<string, unknown>;
          return {
            id: String(obj.id ?? ""),
            name: String(obj.name ?? ""),
            type: obj.type === "container" ? ("container" as const) : ("device" as const),
            lastConnected: String(obj.lastConnected ?? ""),
          };
        }
        return null;
      })
      .filter((entry: RecentTarget | null): entry is RecentTarget => entry !== null && entry.id !== "")
      .slice(0, 8);
  } catch {
    return [];
  }
}

export function saveRecentTarget(target: RecentTarget): RecentTarget[] {
  const current = loadRecentTargets();
  const updated = [target, ...current.filter((entry) => entry.id !== target.id)].slice(0, 8);
  try {
    window.localStorage.setItem(recentsStorageKey, JSON.stringify(updated));
  } catch {
    // Ignore storage write failures.
  }
  return updated;
}
