import { expect, test } from "@playwright/test";

import { installConsoleApiMocks, type MockRouteContext } from "./helpers/consoleApiMocks";

type UserRole = "owner" | "admin" | "operator" | "viewer";
type MockUser = { id: string; username: string; role: UserRole };

test("owner can create and update users from Settings user access card", async ({ page }) => {
  const users: MockUser[] = [{ id: "owner", username: "admin", role: "owner" }];
  let nextID = 1;
  let createdUsername = "";

  await installConsoleApiMocks(page, {
    authUser: { id: "owner", username: "admin", role: "owner" },
    customRoute: async (context: MockRouteContext) => {
      const { pathname, method, requestBody, fulfillJSON } = context;

      if (pathname === "/api/auth/users" && method === "GET") {
        await fulfillJSON({ users });
        return true;
      }

      if (pathname === "/api/auth/users" && method === "POST") {
        const username = typeof requestBody.username === "string" ? requestBody.username.trim().toLowerCase() : "";
        const role = normalizeRole(requestBody.role);
        const created: MockUser = {
          id: `usr-${String(nextID++).padStart(3, "0")}`,
          username: username || "user",
          role,
        };
        createdUsername = created.username;
        users.push(created);
        await fulfillJSON({ user: created }, 201);
        return true;
      }

      if (pathname.startsWith("/api/auth/users/") && method === "PATCH") {
        const id = pathname.split("/").pop() ?? "";
        const existing = users.find((entry) => entry.id === id);
        if (!existing) {
          await fulfillJSON({ error: "not found" }, 404);
          return true;
        }

        if (requestBody.role != null) {
          existing.role = normalizeRole(requestBody.role);
        }
        await fulfillJSON({ user: existing });
        return true;
      }

      return false;
    },
  });

  await page.goto("/users", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Users", level: 1, exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add User", exact: true }).click();

  await page.getByLabel("Username").fill("ops-viewer");
  await page.getByLabel("Password", { exact: true }).fill("TempPass123!");
  await page.getByLabel("Confirm Password").fill("TempPass123!");
  await page.getByLabel("Role").selectOption("viewer");
  await page.getByRole("button", { name: "Create User", exact: true }).click();

  await expect(page.getByText("ops-viewer", { exact: true })).toBeVisible();
  expect(createdUsername).toBe("ops-viewer");

  const userRow = page
    .locator("li")
    .filter({ has: page.getByText("ops-viewer", { exact: true }) })
    .first();
  await userRow.getByRole("button", { name: "User actions", exact: true }).click();
  await page.getByRole("button", { name: "Edit Role", exact: true }).click();
  await page.getByLabel("Role").selectOption("operator");
  await page.getByRole("button", { name: "Update Role", exact: true }).click();

  await expect(userRow.getByText("operator", { exact: true })).toBeVisible();
  expect(users.find((entry) => entry.username === "ops-viewer")?.role).toBe("operator");
});

test("viewer sees read-only user access messaging", async ({ page }) => {
  await installConsoleApiMocks(page, {
    authUser: { id: "viewer-1", username: "readonly", role: "viewer" },
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/auth/users" && method === "GET") {
        await fulfillJSON({ users: [{ id: "viewer-1", username: "readonly", role: "viewer" }] });
        return true;
      }
      return false;
    },
  });

  await page.goto("/users", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "Users", level: 1, exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Account Security", level: 2, exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add User", exact: true })).toHaveCount(0);
});

function normalizeRole(value: unknown): UserRole {
  if (typeof value !== "string") {
    return "viewer";
  }
  switch (value.trim().toLowerCase()) {
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
