import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  RemoteViewToolbar,
  type RemoteViewToolbarProps,
} from "../RemoteViewToolbar";
import { REMOTE_SHORTCUTS } from "../../types/viewer";

(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;
let props: RemoteViewToolbarProps;

beforeEach(() => {
  vi.useFakeTimers();
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  props = {
    connectionState: "connected",
    latencyMs: 20,
    transportLabel: "direct",
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
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function render(overrides: Partial<RemoteViewToolbarProps> = {}) {
  props = { ...props, ...overrides };
  act(() => root.render(<RemoteViewToolbar {...props} />));
}
function button(title: string) {
  const result = container.querySelector<HTMLButtonElement>(
    `button[title="${title}"]`,
  );
  if (!result) throw new Error(`Missing toolbar button: ${title}`);
  return result;
}
function click(title: string) {
  act(() => button(title).click());
}
function inputPath(value: string) {
  const input = container.querySelector<HTMLInputElement>(
    'input[placeholder="/path/to/file"]',
  )!;
  act(() => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  return input;
}

describe("remote toolbar interactions", () => {
  it.each(["overlay", "dock"] as const)(
    "routes primary and menu controls in %s layout",
    (layout) => {
      render({
        layout,
        onSendShortcut: vi.fn(),
        keyboardGrabState: "off",
        onKeyboardGrabToggle: vi.fn(),
      });
      click("Send mouse to remote session");
      click("View only");
      click("Fullscreen");
      expect(props.onPointerLockToggle).toHaveBeenCalledOnce();
      expect(props.onViewOnlyToggle).toHaveBeenCalledOnce();
      expect(props.onFullscreenToggle).toHaveBeenCalledOnce();

      const scaling = container.querySelector('[aria-label="Scaling mode"]')!;
      act(() =>
        [...scaling.querySelectorAll("button")]
          .find((node) => node.textContent === "1:1")!
          .click(),
      );
      expect(props.onScalingModeChange).toHaveBeenCalledWith("native");
      click("More viewer tools");
      click("Send keyboard to remote session");
      expect(props.onKeyboardGrabToggle).toHaveBeenCalledOnce();
      const shortcut = REMOTE_SHORTCUTS.find((item) => item.id === "alt-tab")!;
      click(shortcut.title);
      expect(props.onSendShortcut).toHaveBeenCalledWith(shortcut.keysyms);
      expect(
        container.querySelector(`button[title="${shortcut.title}"]`),
      ).toBeNull();
      click("Disconnect");
      expect(props.onDisconnect).toHaveBeenCalledOnce();
    },
  );

  it("keeps the legacy Ctrl-Alt-Delete callback when shortcut sending is absent", () => {
    render();
    click("More viewer tools");
    click(REMOTE_SHORTCUTS.find((item) => item.id === "ctrl-alt-del")!.title);
    expect(props.onCtrlAltDel).toHaveBeenCalledOnce();
  });

  it("keeps a pending download across overflow closure and submits the trimmed path", () => {
    render({ onDownloadFile: vi.fn() });
    click("More viewer tools");
    click("Download file from remote");
    act(() => vi.advanceTimersByTime(0));
    inputPath("  /tmp/report.txt  ");
    click("More viewer tools");
    click("More viewer tools");
    const input = container.querySelector<HTMLInputElement>(
      'input[placeholder="/path/to/file"]',
    )!;
    expect(input.value).toBe("  /tmp/report.txt  ");
    act(() =>
      input.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
      ),
    );
    expect(props.onDownloadFile).toHaveBeenCalledWith("/tmp/report.txt");
    expect(
      container.querySelector('input[placeholder="/path/to/file"]'),
    ).toBeNull();
  });

  it("retains clipboard feedback while closed, expires it, and cleans its timer on unmount", () => {
    render({ onClipboardPull: vi.fn() });
    render({ clipboardLastSync: "success" });
    click("More viewer tools");
    expect(
      button("Pull clipboard from remote").parentElement!.style.boxShadow,
    ).toContain("var(--ok)");
    act(() => vi.advanceTimersByTime(1500));
    expect(
      button("Pull clipboard from remote").parentElement!.style.boxShadow,
    ).toBe("none");
    render({ clipboardLastSync: "error" });
    act(() => root.render(null));
    expect(vi.getTimerCount()).toBe(0);
  });

  it("moves, hides and reveals the overlay, and keeps dock controls visible", () => {
    render();
    expect(
      button("Disconnect")
        .closest('[style*="position: absolute"]')
        ?.getAttribute("style"),
    ).toContain("bottom: 0px");
    click("Move toolbar to top");
    click("Enable auto-hide");
    act(() => vi.advanceTimersByTime(5000));
    expect(button("Disconnect").parentElement!.style.visibility).toBe("hidden");
    click("Show remote view tools");
    expect(button("Disconnect").parentElement!.style.visibility).toBe(
      "visible",
    );
    render({ layout: "dock" });
    act(() => vi.advanceTimersByTime(6000));
    expect(button("Disconnect").parentElement!.style.visibility).not.toBe(
      "hidden",
    );
    expect(
      container.querySelector('[title="Show remote view tools"]'),
    ).toBeNull();
  });

  it("hides unavailable input and transfer controls and disables unavailable audio", () => {
    render({
      pointerLockSupported: false,
      keyboardGrabState: "unsupported",
      onKeyboardGrabToggle: vi.fn(),
      audioUnavailable: true,
    });
    expect(
      container.querySelector('[title="Send mouse to remote session"]'),
    ).toBeNull();
    expect(button("Audio unavailable").disabled).toBe(true);
    click("More viewer tools");
    expect(
      container.querySelector('[title="Send keyboard to remote session"]'),
    ).toBeNull();
    expect(
      container.querySelector('[title="Download file from remote"]'),
    ).toBeNull();
  });
});
