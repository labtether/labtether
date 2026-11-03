import { act, type AnchorHTMLAttributes } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "../../../../contexts/ToastContext";

// Risk: Preview (a dry run) looks on for an action that has no dry run, or screen readers cannot name the switch.

vi.mock("next-intl", () => ({
  useLocale: () => "en",
  useTranslations: () => (key: string) => key,
}));

vi.mock("../../../../../i18n/navigation", () => ({
  Link: ({ href, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => <a href={href} {...props} />,
}));

vi.mock("../../../../contexts/AuthContext", () => ({ useAuth: () => ({ user: { id: "u1", username: "u1", role: "owner" } }) }));

vi.mock("../../../../hooks/useActions", () => ({
  useActions: () => ({
    actionParameters: [],
    actionParamValues: {},
    assets: [],
    connectors: [{ id: "docker", display_name: "Docker" }],
    selectedConnector: "docker",
    setSelectedConnector: vi.fn(),
    connectorActions: [{ id: "create", name: "Create Container" }],
    selectedConnectorAction: "create",
    setSelectedConnectorAction: vi.fn(),
    selectedActionDescriptor: { id: "create", name: "Create Container", description: "", parameters: [] },
    actionRequiresTarget: false,
    actionSupportsDryRun: false,
    actionTarget: "",
    setActionTarget: vi.fn(),
    setActionParamValue: vi.fn(),
    actionDryRun: true,
    setActionDryRun: vi.fn(),
    actionSubmitting: false,
    connectorActionsError: null,
    actionRuns: [],
    submitConnectorAction: vi.fn(),
  }),
}));

import ActionsPage from "../page";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ data: [] }), { status: 200 })));
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const press = async (label: string) => {
  const target = [...container.querySelectorAll("button")].find((b) => b.textContent?.trim() === label);
  if (!target) throw new Error(`missing button ${label}`);
  await act(async () => { target.dispatchEvent(new MouseEvent("click", { bubbles: true })); });
};

describe("Actions page accessibility", () => {
  it("keeps Preview off and disabled for an action without a dry run, named by its visible label", async () => {
    await act(async () => root.render(<ToastProvider><ActionsPage /></ToastProvider>));
    await press("new");
    await press("integration");

    const preview = container.querySelector<HTMLButtonElement>('button[role="switch"]');
    expect(preview).not.toBeNull();
    expect(preview?.disabled).toBe(true);
    expect(preview?.getAttribute("aria-checked")).toBe("false");
    const label = document.getElementById(preview?.getAttribute("aria-labelledby") ?? "");
    expect(label?.textContent).toBe("preview");
    expect(container.textContent).toContain("noPreview");
  });
});
