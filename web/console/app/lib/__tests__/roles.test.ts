import { describe, expect, it } from "vitest";

import {
  canAccessHostData,
  hasAdminRole,
  hasWriteRole,
  isNodePanelAllowed,
  meetsMinimumRole,
  normalizeHubRole,
} from "../roles";
import { navGroups } from "../navigation";

describe("hub role helpers", () => {
  it("mirrors the backend normalization and defaults unknown roles to viewer", () => {
    expect(normalizeHubRole(" OWNER ")).toBe("owner");
    expect(normalizeHubRole("Operator")).toBe("operator");
    expect(normalizeHubRole("unexpected-role")).toBe("viewer");
    expect(normalizeHubRole(undefined)).toBe("viewer");
  });

  it("limits administrative controls to owners and administrators", () => {
    expect(hasAdminRole("owner")).toBe(true);
    expect(hasAdminRole("admin")).toBe(true);
    expect(hasAdminRole("operator")).toBe(false);
    expect(hasAdminRole("viewer")).toBe(false);
  });

  it("allows operators to mutate fleet state while keeping viewers read-only", () => {
    expect(hasWriteRole("owner")).toBe(true);
    expect(hasWriteRole("admin")).toBe(true);
    expect(hasWriteRole("operator")).toBe(true);
    expect(hasWriteRole("viewer")).toBe(false);
    expect(hasWriteRole("corrupt")).toBe(false);
  });

  it("evaluates minimum role requirements", () => {
    expect(meetsMinimumRole("viewer", "read")).toBe(true);
    expect(meetsMinimumRole("viewer", "write")).toBe(false);
    expect(meetsMinimumRole("operator", "write")).toBe(true);
    expect(meetsMinimumRole("operator", "admin")).toBe(false);
    expect(meetsMinimumRole("admin", "admin")).toBe(true);
  });

  // The Hub answers 403 to viewers on these routes; showing the entry points
  // would leave viewers with broken terminals, desktops and file browsers.
  it("keeps host-data and interactive panels away from viewers", () => {
    expect(canAccessHostData("viewer")).toBe(false);
    expect(canAccessHostData("operator")).toBe(true);
    for (const panel of ["terminal", "desktop", "files", "processes", "cron", "users"]) {
      expect(isNodePanelAllowed(panel, "viewer")).toBe(false);
      expect(isNodePanelAllowed(panel, "operator")).toBe(true);
    }
    expect(isNodePanelAllowed("logs", "viewer", { isDockerContainer: true })).toBe(false);
    expect(isNodePanelAllowed("logs", "viewer")).toBe(true);
    expect(isNodePanelAllowed("system", "viewer")).toBe(true);
    expect(isNodePanelAllowed("logs", "operator", { isDockerContainer: true })).toBe(true);
  });

  it("hides operator-only navigation from viewers", () => {
    const viewerHrefs = navGroups
      .flatMap((group) => group.items)
      .filter((item) => meetsMinimumRole("viewer", item.minimumRole))
      .map((item) => item.href);
    expect(viewerHrefs).not.toContain("/terminal");
    expect(viewerHrefs).not.toContain("/remote-view");
    expect(viewerHrefs).not.toContain("/files");
    expect(viewerHrefs).toContain("/nodes");
  });
});
