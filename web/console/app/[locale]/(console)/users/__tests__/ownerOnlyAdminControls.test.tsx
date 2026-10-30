import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import UsersPage from "../page";
import { ApiKeysCard } from "../../settings/components/ApiKeysCard";
import { OIDCSettingsCard } from "../../settings/components/OIDCSettingsCard";

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock("../../../../contexts/AuthContext", () => ({
  useAuth: () => ({ user: { id: "admin-1", username: "alice", role: "admin" } }),
}));

vi.mock("../../../../contexts/ToastContext", () => ({
  useToast: () => ({ addToast: vi.fn() }),
}));

vi.mock("../../../../contexts/StatusContext", () => ({
  useFastStatus: () => ({ assets: [] }),
}));

vi.mock("../../settings/components/AccountSecurityCard", () => ({
  AccountSecurityCard: () => null,
}));

vi.mock("../../../../hooks/useHubUsers", () => ({
  useHubUsers: () => ({
    users: [
      { id: "admin-1", username: "alice", role: "admin", auth_provider: "local" },
      { id: "admin-2", username: "bob", role: "admin", auth_provider: "local" },
      { id: "owner-1", username: "olga", role: "owner", auth_provider: "local" },
      { id: "ops-1", username: "oscar", role: "operator", auth_provider: "local" },
    ],
    loading: false,
    error: "",
    createUser: vi.fn(),
    updateRole: vi.fn(),
    resetPassword: vi.fn(),
    deleteUser: vi.fn(),
    revokeSessions: vi.fn(),
  }),
}));

vi.mock("../../../../hooks/useApiKeys", () => ({
  useApiKeys: () => ({
    keys: [
      { id: "key-bob", name: "bob automation", prefix: "aaaa", role: "admin", scopes: ["*"], created_by: "admin-2", created_at: "2026-10-01T00:00:00Z" },
      { id: "key-alice", name: "alice automation", prefix: "bbbb", role: "admin", scopes: ["*"], created_by: "admin-1", created_at: "2026-10-01T00:00:00Z" },
      { id: "key-ops", name: "backup job", prefix: "cccc", role: "operator", scopes: ["*"], created_by: "admin-2", created_at: "2026-10-01T00:00:00Z" },
    ],
    loading: false,
    error: "",
    createKey: vi.fn(),
    updateKey: vi.fn(),
    revokeKey: vi.fn(),
  }),
}));

vi.mock("../../../../hooks/useOIDCSettings", () => ({
  useOIDCSettings: () => ({
    enabled: true, setEnabled: vi.fn(),
    issuerURL: "https://login.example.com", setIssuerURL: vi.fn(),
    clientID: "labtether", setClientID: vi.fn(),
    clientSecret: "", setClientSecret: vi.fn(),
    clientSecretConfigured: true, clearClientSecret: false, removeClientSecret: vi.fn(),
    scopes: "openid,profile,email", setScopes: vi.fn(),
    roleClaim: "labtether_role", setRoleClaim: vi.fn(),
    defaultRole: "viewer", setDefaultRole: vi.fn(),
    displayName: "SSO", setDisplayName: vi.fn(),
    adminRoleValues: "lab-admins", setAdminRoleValues: vi.fn(),
    operatorRoleValues: "operator", setOperatorRoleValues: vi.fn(),
    autoProvision: true, setAutoProvision: vi.fn(),
    sources: {}, active: true, activeIssuer: "https://login.example.com",
    loading: false, saving: false, applying: false, error: "", message: "",
    save: vi.fn(), apply: vi.fn(),
  }),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
});

// The Hub refuses an admin who is not the owner when they create or promote
// an admin, change another admin, edit another user's admin API key or save
// SSO settings. The console must not offer those controls.
describe("controls only the owner may use", () => {
  it("are not offered to an admin", async () => {
    await act(async () => {
      root.render(
        <>
          <UsersPage />
          <ApiKeysCard />
          <OIDCSettingsCard canManageUsers canEdit={false} />
        </>,
      );
    });

    // Row actions: only the admin's own row and the operator's row.
    const rowNames = [...container.querySelectorAll<HTMLButtonElement>('button[aria-label="User actions"]')]
      .map((button) => button.closest("li")?.textContent ?? "");
    expect(rowNames).toHaveLength(2);
    expect(rowNames.some((text) => text.includes("alice"))).toBe(true);
    expect(rowNames.some((text) => text.includes("oscar"))).toBe(true);

    const addUser = [...container.querySelectorAll<HTMLButtonElement>("button")].find((button) =>
      button.textContent?.includes("addUser"),
    );
    if (!addUser) throw new Error("missing add user button");
    await act(async () => {
      addUser.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    const dialog = container.querySelector<HTMLElement>('[aria-labelledby="create-user-dialog-title"]');
    const roles = [...(dialog?.querySelectorAll<HTMLOptionElement>("option") ?? [])].map((option) => option.value);
    expect(roles).toEqual(["operator", "viewer"]);

    // API keys: edit and revoke on the admin's own admin key and on the
    // operator key, not on another admin's admin key.
    expect(container.querySelectorAll('button[aria-label="apiKeys.edit"]')).toHaveLength(2);
    expect(container.querySelectorAll('button[aria-label="apiKeys.revoke"]')).toHaveLength(2);

    // SSO: shown read-only with the owner note and no Save.
    expect(container.textContent).toContain("oidc.ownerOnly");
    const saveButton = [...container.querySelectorAll<HTMLButtonElement>("button")].find((button) =>
      button.textContent?.trim() === "oidc.save",
    );
    expect(saveButton).toBeUndefined();
    const issuerInput = [...container.querySelectorAll<HTMLInputElement>("input")].find((input) =>
      input.value === "https://login.example.com",
    );
    expect(issuerInput?.matches(":disabled")).toBe(true);
  });
});
