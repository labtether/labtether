import { expect, test } from "@playwright/test";

import { installConsoleApiMocks, type MockRouteContext } from "./helpers/consoleApiMocks";

type UserRole = "owner" | "admin" | "operator" | "viewer";
type MockUser = { id: string; username: string; role: UserRole };

test("owner adds a user and changes their role from the Users list", async ({ page }) => {
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
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add user", exact: true }).click();

  const addDialog = page.getByRole("dialog");
  await addDialog.getByLabel("Username").fill("ops-viewer");
  await addDialog.getByLabel("Password", { exact: true }).fill("TempPass123!");
  await addDialog.getByLabel("Repeat password").fill("TempPass123!");
  await addDialog.getByLabel("Role").selectOption("viewer");
  await addDialog.getByRole("button", { name: "Add user", exact: true }).click();

  const userRow = page.getByRole("row").filter({ hasText: "ops-viewer" });
  await expect(userRow).toBeVisible();
  expect(createdUsername).toBe("ops-viewer");

  await userRow.getByRole("button", { name: "Actions for ops-viewer", exact: true }).click();
  await page.getByRole("menuitem", { name: "Change role…" }).click();
  const roleDialog = page.getByRole("dialog");
  await roleDialog.getByLabel("Role").selectOption("operator");
  await roleDialog.getByRole("button", { name: "Save", exact: true }).click();

  await expect(userRow.getByText("Operator", { exact: true })).toBeVisible();
  expect(users.find((entry) => entry.username === "ops-viewer")?.role).toBe("operator");
});

test("viewer sees only their own account controls", async ({ page }) => {
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
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();
  // A viewer gets no user list, but keeps their own password and two-factor controls.
  await expect(page.getByRole("button", { name: "Add user", exact: true })).toHaveCount(0);
  await expect(page.getByRole("table", { name: "Users" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /password/i }).first()).toBeVisible();
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
