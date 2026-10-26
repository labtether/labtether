import { act, createRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  vncs: [] as Array<EventTarget & { viewOnly: boolean; disconnect: ReturnType<typeof vi.fn> }>,
  spices: [] as Array<{ options: { onsuccess?: () => void; onerror?: (error: Error) => void }; stop: ReturnType<typeof vi.fn> }>,
}));
vi.mock("@novnc/novnc", () => ({ default: class extends EventTarget {
  viewOnly = false;
  disconnect = vi.fn();
  constructor(container: HTMLElement) {
    super();
    const canvas = document.createElement("canvas");
    canvas.tabIndex = 0;
    container.append(canvas);
    mocks.vncs.push(this);
  }
} }));
vi.mock("@spice-project/spice-html5/src/main.js", () => ({ SpiceMainConn: class {
  stop = vi.fn();
  constructor(public options: { screen_id: string; onsuccess?: () => void; onerror?: (error: Error) => void }) {
    const canvas = document.createElement("canvas");
    canvas.tabIndex = 0;
    document.getElementById(options.screen_id)!.append(canvas);
    mocks.spices.push(this);
  }
} }));

import VNCViewer from "../VNCViewer";
import SPICEViewer, { type SPICEViewerHandle } from "../SPICEViewer";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;
const onConnect = vi.fn();
const onDisconnect = vi.fn();
const writeText = vi.fn(async () => {});

async function render(element: React.ReactNode) {
  await act(async () => { root.render(element); });
  await act(async () => { await vi.dynamicImportSettled(); });
}

beforeEach(() => {
  mocks.vncs = [];
  mocks.spices = [];
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => { root.unmount(); });
  container.remove();
});

describe("VNC session boundaries", () => {
  it("applies view-only mode after the async viewer import", async () => {
    await render(<VNCViewer wsUrl="wss://example.test/first" viewOnly />);
    expect(mocks.vncs[0].viewOnly).toBe(true);
  });

  it("ignores stale connection and clipboard events after reconnect", async () => {
    await render(<VNCViewer wsUrl="wss://example.test/first" onConnect={onConnect} onDisconnect={onDisconnect} />);
    const old = mocks.vncs[0];
    await render(<VNCViewer wsUrl="wss://example.test/second" onConnect={onConnect} onDisconnect={onDisconnect} />);
    await act(async () => {
      old.dispatchEvent(new Event("connect"));
      old.dispatchEvent(new CustomEvent("disconnect", { detail: { clean: false } }));
      old.dispatchEvent(new CustomEvent("clipboard", { detail: { text: "old session" } }));
    });
    expect(onConnect).not.toHaveBeenCalled();
    expect(onDisconnect).not.toHaveBeenCalled();
    expect(writeText).not.toHaveBeenCalled();
    expect(old.disconnect).toHaveBeenCalledOnce();
  });

  it("syncs an empty remote clipboard only while its viewer is focused", async () => {
    await render(<VNCViewer wsUrl="wss://example.test/first" />);
    mocks.vncs[0].dispatchEvent(new CustomEvent("clipboard", { detail: { text: "background" } }));
    expect(writeText).not.toHaveBeenCalled();
    container.querySelector("canvas")!.focus();
    mocks.vncs[0].dispatchEvent(new CustomEvent("clipboard", { detail: { text: "" } }));
    expect(writeText).toHaveBeenCalledWith("");
  });
});

describe("SPICE session input", () => {
  it("focuses and sends Ctrl-Alt-Del through this viewer's keyboard canvas", async () => {
    const ref = createRef<SPICEViewerHandle>();
    await render(<SPICEViewer ref={ref} wsUrl="wss://example.test/spice" password="" />);
    const canvas = container.querySelector("canvas")!;
    const keys: Array<[string, number, boolean, boolean]> = [];
    for (const type of ["keydown", "keyup"]) {
      canvas.addEventListener(type, (event) => {
        const key = event as KeyboardEvent;
        keys.push([key.type, key.keyCode, key.ctrlKey, key.altKey]);
      });
    }
    ref.current!.focus();
    expect(document.activeElement).toBe(canvas);
    ref.current!.sendCtrlAltDel();
    expect(keys).toEqual([
      ["keydown", 17, true, false], ["keydown", 18, true, true], ["keydown", 46, true, true],
      ["keyup", 46, true, true], ["keyup", 18, true, false], ["keyup", 17, false, false],
    ]);
  });

  it("ignores the replaced SPICE connection's late callbacks", async () => {
    await render(<SPICEViewer wsUrl="wss://example.test/first" password="" onConnect={onConnect} onDisconnect={onDisconnect} />);
    const old = mocks.spices[0];
    await render(<SPICEViewer wsUrl="wss://example.test/second" password="" onConnect={onConnect} onDisconnect={onDisconnect} />);
    old.options.onsuccess?.();
    old.options.onerror?.(new Error("old connection"));
    expect(onConnect).not.toHaveBeenCalled();
    expect(onDisconnect).not.toHaveBeenCalled();
    expect(old.stop).toHaveBeenCalledOnce();
  });
});
