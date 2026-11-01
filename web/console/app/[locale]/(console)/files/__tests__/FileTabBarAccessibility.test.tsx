import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import messages from "../../../../../messages/en/files.json";

import { FileTabBar } from "../FileTabBar";

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

// A close button nested inside the tab button is unreachable for screen readers and closes the tab on a tab click.
describe("FileTabBar accessibility", () => {
  it("renders separate, named tab and close buttons", async () => {
    const onRemoveTab = vi.fn();
    const onSetActiveTab = vi.fn();
    await act(async () => {
      root.render(
        <NextIntlClientProvider locale="en" messages={{ files: messages }}>
          <FileTabBar
            tabs={[{ id: "t1", type: "agent", label: "nas01", sourceId: "nas01" }]}
            activeTabId="t1"
            onAddTab={vi.fn()}
            onRemoveTab={onRemoveTab}
            onSetActiveTab={onSetActiveTab}
          />
        </NextIntlClientProvider>,
      );
    });

    const tab = container.querySelector<HTMLElement>('[role="tab"]');
    const close = container.querySelector<HTMLButtonElement>('button[aria-label="Close nas01"]');
    expect(container.querySelector('[role="tablist"]')?.getAttribute("aria-label")).toBe("File tabs");
    expect(tab?.getAttribute("aria-selected")).toBe("true");
    expect(close).not.toBeNull();
    expect(tab?.contains(close ?? null)).toBe(false);

    await act(async () => tab?.click());
    expect(onSetActiveTab).toHaveBeenCalledWith("t1");
    expect(onRemoveTab).not.toHaveBeenCalled();

    await act(async () => close?.click());
    expect(onRemoveTab).toHaveBeenCalledWith("t1");
  });
});
