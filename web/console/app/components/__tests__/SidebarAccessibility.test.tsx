import { act, type AnchorHTMLAttributes } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../i18n/navigation", () => ({
  Link: ({ href, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props} />
  ),
  usePathname: () => "/nodes",
}));

vi.mock("next-intl", () => ({ useTranslations: () => (key: string) => key }));
vi.mock("../../contexts/StatusContext", () => ({ useServiceStatusLabel: () => "0/0 services online" }));
vi.mock("../../contexts/DesktopSessionContext", () => ({ useDesktopSession: () => ({ activeSession: null }) }));
vi.mock("../../contexts/AuthContext", () => ({ useAuth: () => ({ user: { role: "owner" } }) }));
vi.mock("../LanguagePicker", () => ({ LanguagePicker: () => null }));

import { Sidebar } from "../Sidebar";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("Sidebar accessibility", () => {
  it("names the navigation landmark and exposes the current page", async () => {
    await act(async () => root.render(<Sidebar />));

    expect(container.querySelector("nav")?.getAttribute("aria-label")).toBe("Primary navigation");
    expect(container.querySelector('a[href="/nodes"]')?.getAttribute("aria-current")).toBe("page");
    expect(container.querySelector('a[href="/"]')?.hasAttribute("aria-current")).toBe(false);
  });
});
