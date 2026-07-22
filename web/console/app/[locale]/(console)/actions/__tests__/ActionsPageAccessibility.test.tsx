import { act, type AnchorHTMLAttributes } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key === "runAction.previewOnly.label" ? "Preview Only" : key,
}));

vi.mock("../../../../../i18n/navigation", () => ({
  Link: ({ href, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props} />
  ),
}));

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
    actionDryRun: false,
    setActionDryRun: vi.fn(),
    actionSubmitting: false,
    actionMessage: null,
    connectorActionsError: null,
    actionRuns: [],
    submitConnectorAction: vi.fn((event: Event) => event.preventDefault()),
  }),
}));

vi.mock("../SavedActionsCard", () => ({ SavedActionsCard: () => null }));

import ActionsPage from "../page";

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

describe("Actions page accessibility", () => {
  it("uses a native, visibly labelled checkbox for preview mode", async () => {
    await act(async () => root.render(<ActionsPage />));

    const checkbox = container.querySelector<HTMLInputElement>('input[type="checkbox"]');
    expect(checkbox).not.toBeNull();
    expect(checkbox?.disabled).toBe(true);
    expect(checkbox?.closest("label")?.textContent).toContain("Preview Only");
    expect(container.querySelector('[role="checkbox"]')).toBeNull();
  });
});
