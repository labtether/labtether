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
    connectionProgress: { message: "", totalElapsedMs: 0 },
    connectedAgentIds: new Set<string>(),
  }),
}));

// The real German words, so the test also sees the console's own English error translated.
vi.mock("next-intl", async () => {
  const de = (await import("../../../../messages/de/terminal.json")).default as unknown as Record<string, Record<string, string>>;
  return { useTranslations: (ns: string) => (key: string) => de[ns.split(".")[1]]?.[key] ?? key };
});

import { usePaneSession, type PaneStatus } from "../usePaneSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let status: PaneStatus | null;

function Pane() {
  status = usePaneSession({
    targetNodeId: "nas01", targetSource: "agent", isTabActive: true, isFocused: true, autoReconnect: false,
    onClipboardBlocked: () => {}, onToggleSearch: () => {},
  } as Parameters<typeof usePaneSession>[0]).status;
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
