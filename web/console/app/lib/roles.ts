export type HubRole = "owner" | "admin" | "operator" | "viewer";

export type MinimumRole = "read" | "write" | "admin";

export function normalizeHubRole(role: string | null | undefined): HubRole {
  switch (role?.trim().toLowerCase()) {
    case "owner":
      return "owner";
    case "admin":
      return "admin";
    case "operator":
      return "operator";
    default:
      return "viewer";
  }
}

export function hasAdminRole(role: string | null | undefined): boolean {
  const normalized = normalizeHubRole(role);
  return normalized === "owner" || normalized === "admin";
}

export function hasWriteRole(role: string | null | undefined): boolean {
  return normalizeHubRole(role) !== "viewer";
}

export function meetsMinimumRole(
  role: string | null | undefined,
  minimumRole: MinimumRole = "read",
): boolean {
  if (minimumRole === "admin") {
    return hasAdminRole(role);
  }
  if (minimumRole === "write") {
    return hasWriteRole(role);
  }
  return true;
}

// Mirrors the Hub's host-data policy (cmd/labtether/host_data_access_policy.go).
// Viewers may read inventory, status, alerts and stored logs, but opening
// terminal, desktop or exec sessions and reading data from inside a host
// (files, processes, cron, host users, journals, container logs, clipboard,
// session recordings) needs operator or higher. The Hub answers 403 otherwise,
// so the console hides those entry points instead of showing broken controls.
export function canAccessHostData(role: string | null | undefined): boolean {
  return hasWriteRole(role);
}

const HOST_DATA_NODE_PANELS = new Set(["terminal", "desktop", "files", "processes", "cron", "users"]);

export function isNodePanelAllowed(
  panel: string,
  role: string | null | undefined,
  options: { isDockerContainer?: boolean } = {},
): boolean {
  if (canAccessHostData(role)) {
    return true;
  }
  if (HOST_DATA_NODE_PANELS.has(panel)) {
    return false;
  }
  // A Docker container's logs panel streams live container output; a host's
  // logs panel shows stored log history, which stays viewer-readable.
  return !(panel === "logs" && options.isDockerContainer);
}
