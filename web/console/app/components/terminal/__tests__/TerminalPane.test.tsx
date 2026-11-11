import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

// Risk: a terminal stream that fails before it opens (the Hub refuses the websocket, a proxy without websocket
// upgrade) leaves the ended terminal mounted, because useSession keeps wsUrl. The pane said "Could not connect" in
// its status but showed no Try again, so the only way back was a page reload (seen on the 2.0 lab Hub, 2026-11-03).
const pane = vi.hoisted(() => ({
  state: "error",
  error: "The connection closed unexpectedly",
  wsUrl: "wss://hub.example/terminal/sessions/s1/stream",
  ended: "",
  reconnect: vi.fn(),
}));
vi.mock("next-intl", () => ({ useTranslations: () => (key: string) => key }));
vi.mock("../../../contexts/StatusContext", () => ({ useFastStatus: () => ({ assets: [{ id: "nas01", name: "nas01" }] }) }));
vi.mock("../../XTerminal", () => ({ default: () => <div data-testid="xterm" /> }));
vi.mock("../SearchBar", () => ({ default: () => null }));
vi.mock("../TerminalPicker", () => ({ default: () => null }));
vi.mock("../TerminalTargetPicker", () => ({ TerminalTargetPicker: () => null }));
vi.mock("../usePaneSession", () => ({
  usePaneSession: () => ({
    session: { wsUrl: pane.wsUrl, connectedAgentIds: new Set<string>(), disconnect: () => {} },
    termRef: { current: null },
    status: { state: pane.state, progress: "", error: pane.error, via: "agent", ended: pane.ended },
    handleConnected: () => {},
    handleStreamReady: () => {},
    reconnect: pane.reconnect,
    switchTarget: () => {},
    report: () => {},
  }),
}));

import TerminalPane from "../TerminalPane";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let host: HTMLDivElement;

async function render() {
  await act(async () => root.render(
    <TerminalPane
      paneIndex={0} targetNodeId="nas01" onTargetChange={() => {}} isTabActive isFocused onFocus={() => {}}
      broadcastActive={false} onBroadcastData={() => {}} onTerminalRef={() => {}}
      prefs={{ auto_reconnect: true } as Parameters<typeof TerminalPane>[0]["prefs"]}
      themeDef={{ theme: { background: "#000", foreground: "#fff" } } as Parameters<typeof TerminalPane>[0]["themeDef"]}
      fontDef={{ family: "monospace" } as Parameters<typeof TerminalPane>[0]["fontDef"]}
    />,
  ));
}

const retryButton = () => [...host.querySelectorAll("button")].find((button) => button.textContent === "retry");
const reconnectButton = () => [...host.querySelectorAll("button")].find((button) => button.textContent === "reconnect");

beforeEach(() => {
  host = document.createElement("div");
  root = createRoot(host);
  pane.state = "error";
  pane.ended = "";
  pane.reconnect.mockReset();
});

afterEach(() => act(() => root.unmount()));

it("offers Try again over the ended terminal when the stream failed", async () => {
  await render();
  expect(host.textContent).toContain("The connection closed unexpectedly");
  await act(async () => retryButton()?.click());
  expect(pane.reconnect).toHaveBeenCalledTimes(1);
});

// Risk: the pane read "Session disconnected" whether someone typed `exit` or the device went away, so nobody could
// tell a finished shell from a lost device (seen on a Windows device, 2026-11-11).
it("says the shell ended, or names the device that went away, and keeps Reconnect", async () => {
  pane.state = "disconnected";
  pane.ended = "shell";
  await render();
  expect(host.querySelector(".tm-state-t")?.textContent).toBe("sessionEnded");

  pane.ended = "device";
  await render();
  expect(host.querySelector(".tm-state-t")?.textContent).toBe("deviceDisconnected");

  pane.ended = "deviceStillAway";
  await render();
  expect(host.querySelector(".tm-state-t")?.textContent).toBe("deviceStillDisconnected");
  await act(async () => reconnectButton()?.click());
  expect(pane.reconnect).toHaveBeenCalledTimes(1);
});
