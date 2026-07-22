import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

describe("FileTabBar accessibility", () => {
  it("renders separate, named tab and close buttons", async () => {
    const onRemoveTab = vi.fn();
    const onSetActiveTab = vi.fn();
    await act(async () => {
      root.render(
        <FileTabBar
          tabs={[{ id: "new", type: "new", label: "New Tab" }]}
          activeTabId="new"
          splitMode={false}
          onAddTab={vi.fn()}
          onRemoveTab={onRemoveTab}
          onSetActiveTab={onSetActiveTab}
          onToggleSplit={vi.fn()}
        />,
      );
    });

    const tab = container.querySelector<HTMLElement>('[role="tab"]');
    const close = container.querySelector<HTMLButtonElement>('button[aria-label="Close New Tab tab"]');
    expect(container.querySelector('[role="tablist"]')?.getAttribute("aria-label")).toBe("File tabs");
    expect(tab?.getAttribute("aria-selected")).toBe("true");
    expect(close).not.toBeNull();
    expect(tab?.contains(close ?? null)).toBe(false);

    await act(async () => tab?.click());
    expect(onSetActiveTab).toHaveBeenCalledWith("new");
    expect(onRemoveTab).not.toHaveBeenCalled();

    await act(async () => close?.click());
    expect(onRemoveTab).toHaveBeenCalledWith("new");
  });
});
