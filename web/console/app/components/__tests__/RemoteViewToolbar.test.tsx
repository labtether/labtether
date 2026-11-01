import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import messages from "../../../messages/en/desktop.json";
import { RemoteViewToolbar, type RemoteViewToolbarProps } from "../RemoteViewToolbar";
import { AUTOHIDE_DELAY_MS } from "../remote-toolbar/useToolbarAutoHide";
import { REMOTE_SHORTCUTS } from "../../types/viewer";

// Risk: every remote-screen control moved into menus in 2.0; each must still reach its handler, unavailable controls
// must stay out of reach, and the full-screen toolbar must hide and come back.
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;
let props: RemoteViewToolbarProps;

beforeEach(() => {
  vi.useFakeTimers();
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  props = {
    layout: "dock",
    connectionState: "connected",
    latencyMs: 20,
    transportLabel: "VNC",
    protocol: "vnc",
    quality: "high",
    onQualityChange: vi.fn(),
    scalingMode: "fit",
    onScalingModeChange: vi.fn(),
    pointerLocked: false,
    onPointerLockToggle: vi.fn(),
    viewOnly: false,
    onViewOnlyToggle: vi.fn(),
    isFullscreen: false,
    onFullscreenToggle: vi.fn(),
    onCtrlAltDel: vi.fn(),
    onDisconnect: vi.fn(),
  };
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.replaceChildren();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function render(overrides: Partial<RemoteViewToolbarProps> = {}) {
  props = { ...props, ...overrides };
  act(() =>
    root.render(
      <NextIntlClientProvider locale="en" messages={{ desktop: messages }}>
        <RemoteViewToolbar {...props} />
      </NextIntlClientProvider>,
    ),
  );
}
function button(label: string) {
  const found = document.body.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  if (!found) throw new Error(`Missing tool: ${label}`);
  return found;
}
function openMenu(label: string) {
  act(() => {
    button(label).dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
  });
}
function item(text: string) {
  return [...document.body.querySelectorAll<HTMLElement>('[role^="menuitem"]')].find((node) => node.textContent?.startsWith(text)) ?? null;
}
function choose(text: string) {
  const found = item(text);
  if (!found) throw new Error(`Missing menu item: ${text}`);
  act(() => found.click());
}

describe("remote toolbar", () => {
  it("routes the keys, input and More menus and the buttons to their handlers", () => {
    render({ onSendShortcut: vi.fn(), keyboardGrabState: "off", onKeyboardGrabToggle: vi.fn() });
    const altTab = REMOTE_SHORTCUTS.find((s) => s.id === "alt-tab")!;
    openMenu("Send keys");
    choose(altTab.title);
    expect(props.onSendShortcut).toHaveBeenCalledWith(altTab.keysyms);
    expect(item(altTab.title)).toBeNull();
    openMenu("Send keys");
    choose("Capture keyboard");
    openMenu("Send keys");
    choose("Capture mouse");
    expect(props.onKeyboardGrabToggle).toHaveBeenCalledOnce();
    expect(props.onPointerLockToggle).toHaveBeenCalledOnce();

    openMenu("More");
    choose("100%");
    expect(props.onScalingModeChange).toHaveBeenCalledWith("native");
    openMenu("More");
    choose("View only");
    expect(props.onViewOnlyToggle).toHaveBeenCalledOnce();

    act(() => button("Full screen").click());
    act(() => document.body.querySelector<HTMLButtonElement>('button[title="Disconnect"]')!.click());
    expect(props.onFullscreenToggle).toHaveBeenCalledOnce();
    expect(props.onDisconnect).toHaveBeenCalledOnce();
  });

  it("sends Ctrl+Alt+Del through the older callback when key sending is absent", () => {
    render();
    openMenu("Send keys");
    choose(REMOTE_SHORTCUTS.find((s) => s.id === "ctrl-alt-del")!.title);
    expect(props.onCtrlAltDel).toHaveBeenCalledOnce();
  });

  it("keeps a download path while its popover is closed and submits it trimmed", () => {
    render({ onDownloadFile: vi.fn() });
    act(() => button("Download file").click());
    const input = () => document.body.querySelector<HTMLInputElement>("#rv-download-path");
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input(), "  /tmp/report.txt  ");
      input()!.dispatchEvent(new Event("input", { bubbles: true }));
    });
    act(() => button("Download file").click());
    expect(input()).toBeNull();
    act(() => button("Download file").click());
    expect(input()!.value).toBe("  /tmp/report.txt  ");
    act(() => input()!.form!.requestSubmit());
    expect(props.onDownloadFile).toHaveBeenCalledWith("/tmp/report.txt");
    expect(input()).toBeNull();
  });

  it("says when a clipboard sync finished, then clears it and its timer", () => {
    render({ onClipboardPull: vi.fn() });
    render({ clipboardLastSync: "success" });
    expect(container.textContent).toContain("Clipboard synced");
    act(() => vi.advanceTimersByTime(1500));
    expect(container.textContent).not.toContain("Clipboard synced");
    render({ clipboardLastSync: "error" });
    expect(container.textContent).toContain("Clipboard sync failed");
    act(() => root.render(null));
    expect(vi.getTimerCount()).toBe(0);
  });

  it("hides the full-screen toolbar into a handle, brings it back, moves it and can stay up", () => {
    render({ layout: "overlay", isFullscreen: true });
    const bar = () => container.querySelector<HTMLElement>('[role="toolbar"]')!;
    expect(bar().hasAttribute("inert")).toBe(false);
    act(() => vi.advanceTimersByTime(AUTOHIDE_DELAY_MS));
    expect(bar().hasAttribute("inert")).toBe(true);
    act(() => button("Show Remote View tools").click());
    expect(bar().hasAttribute("inert")).toBe(false);

    openMenu("More");
    choose("Move to bottom");
    expect(container.querySelector(".rv-float.bottom")).not.toBeNull();
    openMenu("More");
    choose("Hide when idle");
    act(() => vi.advanceTimersByTime(AUTOHIDE_DELAY_MS * 3));
    expect(bar().hasAttribute("inert")).toBe(false);

    render({ layout: "dock", isFullscreen: false });
    act(() => vi.advanceTimersByTime(AUTOHIDE_DELAY_MS * 3));
    expect(bar().hasAttribute("inert")).toBe(false);
    expect(document.body.querySelector('[aria-label="Show Remote View tools"]')).toBeNull();
  });

  it("leaves out unavailable input, keeps Files disabled without the agent and shows sound as unavailable", () => {
    render({ pointerLockSupported: false, keyboardGrabState: "unsupported", onKeyboardGrabToggle: vi.fn(), audioUnavailable: true });
    openMenu("Send keys");
    expect(item("Capture mouse")).toBeNull();
    expect(item("Capture keyboard")).toBeNull();
    act(() => document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    expect(button("Files need the LabTether agent").disabled).toBe(true);
    openMenu("More");
    expect(item("Sound unavailable")?.hasAttribute("data-disabled")).toBe(true);
    expect(document.body.querySelector('[aria-label="Volume"]')).toBeNull();
  });
});
