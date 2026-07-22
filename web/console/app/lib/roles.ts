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
