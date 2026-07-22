import { act, useState, type AnchorHTMLAttributes } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../i18n/navigation", () => ({
  Link: ({ href, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props} />
  ),
  usePathname: () => "/nodes",
}));

vi.mock("../../contexts/AuthContext", () => ({
  useAuth: () => ({ user: { role: "owner" } }),
}));

import { MobileNavOverlay, MobileNavToggle, useMobileNav } from "../MobileNav";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

function Harness() {
  const { open, toggle, close } = useMobileNav();
  return (
    <>
      <MobileNavToggle open={open} onToggle={toggle} />
      <MobileNavOverlay open={open} onClose={close} />
    </>
  );
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("MobileNav accessibility", () => {
  it("announces state, marks the current route, closes on Escape, and restores focus", async () => {
    await act(async () => root.render(<Harness />));

    const toggle = document.getElementById("mobile-navigation-toggle") as HTMLButtonElement;
    expect(toggle.getAttribute("aria-controls")).toBe("mobile-navigation-drawer");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");

    await act(async () => toggle.click());

    const drawer = document.getElementById("mobile-navigation-drawer");
    expect(drawer?.getAttribute("role")).toBe("dialog");
    expect(drawer?.getAttribute("aria-modal")).toBe("true");
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(document.querySelector('a[href="/nodes"]')?.getAttribute("aria-current")).toBe("page");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Close navigation");

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });

    expect(document.getElementById("mobile-navigation-drawer")).toBeNull();
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(toggle);
  });
});
