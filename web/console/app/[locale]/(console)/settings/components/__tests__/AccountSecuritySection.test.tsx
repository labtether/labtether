import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Risk: a single sign-on account was offered a password change the Hub refuses and a two-factor setup its
// sign-ins never ask for, so the account looked protected by a code it never needs.
const auth = vi.hoisted(() => ({ user: { id: "u1", username: "sso-user", role: "viewer", auth_provider: "oidc" } as Record<string, string> }));
vi.mock("next-intl", () => ({ useTranslations: () => (key: string) => key }));
vi.mock("../../../../../contexts/AuthContext", () => ({ useAuth: () => ({ user: auth.user, refreshUser: vi.fn() }) }));
vi.mock("../AccountSecurityCard", () => ({ AccountSecurityCard: () => <p>password and two-factor forms</p> }));

import { AccountSecuritySection } from "../AccountSecuritySection";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let container: HTMLDivElement;
let root: Root;
beforeEach(() => { container = document.createElement("div"); document.body.append(container); root = createRoot(container); });
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });

describe("AccountSecuritySection", () => {
  it("gives a single sign-on account the identity provider line instead of the forms", async () => {
    await act(async () => root.render(<AccountSecuritySection />));
    expect(container.textContent).toContain("security.singleSignOn");
    expect(container.textContent).not.toContain("password and two-factor forms");
  });

  it("keeps the forms for a local account", async () => {
    auth.user = { ...auth.user, auth_provider: "local" };
    await act(async () => root.render(<AccountSecuritySection />));
    expect(container.textContent).toContain("password and two-factor forms");
  });
});
