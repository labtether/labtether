import { describe, expect, it } from "vitest";
import type { HubUser } from "../../../../hooks/useHubUsers";
import { hasTwoFactorState, nextSort, sortUsers } from "../usersModel";

const user = (username: string, role: string, extra: Partial<HubUser> = {}): HubUser => ({ id: username, username, role, auth_provider: "local", ...extra });
const names = (list: HubUser[]) => list.map((u) => u.username);

const users = [user("sam", "admin"), user("guest", "viewer"), user("admin", "owner"), user("ops", "operator"), user("amy", "viewer")];

// The Role column must order by rank (owner first), not alphabetically, and keep a stable name order inside a rank.
describe("sortUsers", () => {
  it("orders roles from the owner down, names breaking ties", () => {
    expect(names(sortUsers(users, { key: "role", desc: false }))).toEqual(["admin", "sam", "ops", "amy", "guest"]);
  });

  it("flips only the role order when descending", () => {
    expect(names(sortUsers(users, { key: "role", desc: true }))).toEqual(["amy", "guest", "ops", "sam", "admin"]);
  });

  it("starts a new column ascending and flips the same one", () => {
    expect(nextSort({ key: "name", desc: false }, "role")).toEqual({ key: "role", desc: false });
    expect(nextSort({ key: "role", desc: false }, "role")).toEqual({ key: "role", desc: true });
  });
});

// A missing field must not read as "2FA off" for every user.
describe("hasTwoFactorState", () => {
  it("is false when the hub sends no two-factor state", () => {
    expect(hasTwoFactorState(users)).toBe(false);
    expect(hasTwoFactorState([...users, user("x", "viewer", { totp_enabled: false })])).toBe(true);
  });
});
