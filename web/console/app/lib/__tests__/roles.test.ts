import { describe, expect, it } from "vitest";

import { hasAdminRole, hasWriteRole, meetsMinimumRole, normalizeHubRole } from "../roles";

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
});
