"use client";

import { useAuth } from "../contexts/AuthContext";
import { hasAdminRole, hasWriteRole } from "../lib/roles";

/** Whether the signed-in person may change things. The Hub refuses every change from a viewer. */
export function useCanWrite(): boolean {
  return hasWriteRole(useAuth().user?.role);
}

/** Whether the signed-in person is an owner or admin (users, settings, agent approval). */
export function useIsAdmin(): boolean {
  return hasAdminRole(useAuth().user?.role);
}
