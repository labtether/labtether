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

// Mirrors the Hub's owner rule (internal/hubapi/auth/identity_audit.go): only
// the owner creates, promotes, changes, signs out or deletes an admin, saves
// SSO settings, or changes an admin-role API key another user made. Admins
// manage operators, viewers and their own account and keys. The Hub refuses
// the rest, so the console does not offer it.
export function isOwnerRole(role: string | null | undefined): boolean {
  return normalizeHubRole(role) === "owner";
}

type Principal = { id?: string; role?: string } | null | undefined;

// Roles the caller may give a user whose current role is targetRole. A
// non-owner admin keeps "admin" only for an account that already has it,
// which is their own.
export function assignableUserRoles(callerRole: string | null | undefined, targetRole?: string): HubRole[] {
  const roles: HubRole[] = ["operator", "viewer"];
  if (isOwnerRole(callerRole) || normalizeHubRole(targetRole) === "admin") {
    roles.unshift("admin");
  }
  return roles;
}

export function canManageUserAccount(caller: Principal, target: { id: string; role: string }): boolean {
  if (!caller) return false;
  if (caller.id && caller.id === target.id) return true;
  return !hasAdminRole(target.role) || isOwnerRole(caller.role);
}

export function canManageApiKey(caller: Principal, key: { role: string; created_by?: string }): boolean {
  if (!hasAdminRole(key.role) || isOwnerRole(caller?.role)) return true;
  return Boolean(caller?.id) && key.created_by === caller?.id;
}
