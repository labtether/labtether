import type { HubUser } from "../../../hooks/useHubUsers";
import { normalizeHubRole, type HubRole } from "../../../lib/roles";

export type UserSortKey = "name" | "role";
export type UserSort = { key: UserSortKey; desc: boolean };

const RANK: Record<HubRole, number> = { owner: 0, admin: 1, operator: 2, viewer: 3 };

const byName = (a: HubUser, b: HubUser) => a.username.localeCompare(b.username);

/** Users in list order: by name, or by role from the owner down with names breaking ties. desc flips the chosen key only. */
export function sortUsers(users: HubUser[], sort: UserSort): HubUser[] {
  const sign = sort.desc ? -1 : 1;
  const rank = (u: HubUser) => RANK[normalizeHubRole(u.role)];
  return [...users].sort((a, b) =>
    sort.key === "role" ? sign * (rank(a) - rank(b)) || byName(a, b) : sign * byName(a, b),
  );
}

/** Clicking a column: the same column flips its order, another column starts ascending. */
export const nextSort = (sort: UserSort, key: UserSortKey): UserSort =>
  sort.key === key ? { key, desc: !sort.desc } : { key, desc: false };

/** A hub password account, as opposed to one that signs in through single sign-on. */
export const isLocalUser = (user: HubUser) => !user.auth_provider || user.auth_provider === "local";

/** The users list carries no two-factor state today; the 2FA column shows only when it does. */
export const hasTwoFactorState = (users: HubUser[]) => users.some((u) => typeof u.totp_enabled === "boolean");
