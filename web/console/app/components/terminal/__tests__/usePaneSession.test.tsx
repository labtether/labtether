import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: when the terminal stream never opens (a proxy without websocket upgrade, or a console address the Hub does
// not allow), the pane went back to idle and connected again at once, several times a second, for as long as the
// tab was open (seen against the 2.0 lab Hub, 2026-11-02). It must try once and then show the error.
const fake = vi.hoisted(() => ({ state: "idle" as string, error: null as string | null, connect: vi.fn() }));
vi.mock("../../../hooks/useSession", () => ({
  useSession: () => ({
    connectionState: fake.state,
    error: fake.error,
    connect: fake.connect,
    setTarget: () => {},
    disconnect: () => {},
    handleConnected: () => {},
    handleStreamReady: () => {},
    handleDisconnected: () => {},
    connectionProgress: { message: "", totalElapsedMs: 0 },
    connectedAgentIds: new Set<string>(),
  }),
}));

// The real German words, so the test also sees the console's own English error translated.
vi.mock("next-intl", async () => {
  const de = (await import("../../../../messages/de/terminal.json")).default as unknown as Record<string, Record<string, string>>;
  return { useTranslations: (ns: string) => (key: string) => de[ns.split(".")[1]]?.[key] ?? key };
});

import { terminalCloseFrom } from "../terminalClose";
import { usePaneSession, type PaneStatus } from "../usePaneSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let status: PaneStatus | null;
let pane: ReturnType<typeof usePaneSession> | null;

function Pane() {
  pane = usePaneSession({
    targetNodeId: "nas01", targetSource: "agent", isTabActive: true, isFocused: true, autoReconnect: false,
    onClipboardBlocked: () => {}, onToggleSearch: () => {},
  } as Parameters<typeof usePaneSession>[0]);
  status = pane.status;
  return null;
}

beforeEach(() => {
  root = createRoot(document.createElement("div"));
  fake.state = "idle";
  fake.error = null;
  fake.connect.mockReset();
});

afterEach(() => act(() => root.unmount()));

it("connects once and shows the error when the stream never opens", async () => {
  await act(async () => root.render(<Pane />));
  expect(fake.connect).toHaveBeenCalledTimes(1);

  // The attempt fails before the socket opens: useSession ends in idle with an error.
  for (const next of ["connecting", "idle", "connecting", "idle"]) {
    fake.state = next;
    fake.error = next === "idle" ? "Terminal connection error" : null;
    await act(async () => root.render(<Pane />));
  }
  expect(fake.connect).toHaveBeenCalledTimes(1);
  expect(status?.state).toBe("error");
  expect(status?.error).toBe("Die Terminalverbindung ist fehlgeschlagen.");
});

// Risk: xterm takes typing through a hidden textarea and the pane's key handler skipped every textarea, so
// Ctrl+Shift+C never copied while someone typed in the shell (found in a real browser on the 2.0 lab Hub).
it("copies with Ctrl+Shift+C while the shell has focus and leaves the shell its own keys", async () => {
  await act(async () => root.render(<Pane />));
  const term = { copySelection: vi.fn(async () => ({ ok: true as const })), pasteFromClipboard: vi.fn(async () => ({ ok: true as const })), selectAll: vi.fn() };
  pane!.termRef.current = term as unknown as NonNullable<typeof pane>["termRef"]["current"];
  const shell = document.createElement("textarea");
  shell.className = "xterm-helper-textarea";
  document.body.append(shell);
  const press = (key: string, mods: KeyboardEventInit) => {
    const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...mods });
    shell.dispatchEvent(event);
    return event.defaultPrevented;
  };
  try {
    expect(press("C", { ctrlKey: true, shiftKey: true })).toBe(true);
    expect(term.copySelection).toHaveBeenCalledTimes(1);
    // Start of line and the browser's own paste are not the pane's: no action, no preventDefault.
    expect(press("a", { ctrlKey: true })).toBe(false);
    expect(press("V", { ctrlKey: true, shiftKey: true })).toBe(false);
    expect(term.selectAll).not.toHaveBeenCalled();
    expect(term.pasteFromClipboard).not.toHaveBeenCalled();
  } finally {
    shell.remove();
  }
});

// Risk: a lost device and a typed `exit` both read "Session disconnected", and Reconnect to a device that was still
// away showed an empty screen with the same words (seen on a Windows device, 2026-11-11). The Hub sends the reason.
it("tells a shell that ended from a device that went away", async () => {
  await act(async () => root.render(<Pane />));
  const runThenClose = async (event: { code: number; reason: string }) => {
    fake.state = "connected";
    await act(async () => pane!.handleStreamReady());
    fake.state = "idle";
    await act(async () => pane!.handleDisconnected(terminalCloseFrom(event)));
  };

  await runThenClose({ code: 1000, reason: "" });
  expect(status).toMatchObject({ state: "disconnected", ended: "shell" });

  // A session another window took over closes cleanly too; it has not ended.
  await runThenClose({ code: 1000, reason: "Session attached elsewhere" });
  expect(status).toMatchObject({ state: "disconnected", ended: "" });

  await runThenClose({ code: 1001, reason: "Agent disconnected" });
  expect(status).toMatchObject({ state: "disconnected", ended: "device" });

  // Reconnect while the device is away: the new stream closes before a shell starts.
  await act(async () => pane!.handleDisconnected(terminalCloseFrom({ code: 1006, reason: "" })));
  expect(status).toMatchObject({ state: "disconnected", ended: "deviceStillAway" });
});
