import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { RemoteViewerShellProps } from "../../../../components/RemoteViewerShell";

const mocks = vi.hoisted(() => ({
  shellProps: null as RemoteViewerShellProps | null,
  session: {
    activeSessionId: "session-1",
    assets: [],
    audioWsUrl: null,
    connect: vi.fn(async () => undefined),
    connectedAgentIds: new Set(["asset-1"]),
    connectionState: "connected" as const,
    disconnect: vi.fn(),
    error: null,
    handleConnected: vi.fn(),
    handleDisconnected: vi.fn(),
    handleError: vi.fn(),
    isReconnecting: false,
    maxReconnectAttempts: 5,
    quality: "medium",
    reconnectAttempt: 0,
    reconnectExhausted: false,
    setQuality: vi.fn(),
    spiceTicket: null,
    target: "asset-1",
    vncPassword: "  exact ticket password\n",
    wsUrl: "wss://console.example.test/desktop",
  },
  runtime: {
    adaptiveQuality: {
      applyQuality: vi.fn(),
      autoEnabled: false,
      dismissSuggestion: vi.fn(),
      getNextQuality: vi.fn(() => "low" as const),
      setAutoEnabled: vi.fn(),
      suggestion: "downgrade" as const,
    },
    audioSideband: { status: "playing" },
    clipboard: {
      lastSync: "idle" as const,
      pullFromRemote: vi.fn(),
      pushToRemote: vi.fn(),
      syncing: false,
    },
    effectiveTransportLabel: "VNC",
    fileDownload: {
      downloadFile: vi.fn(),
      downloading: false,
    },
    handleSendShortcut: vi.fn(),
    keyboardGrab: {
      activate: vi.fn(async () => undefined),
      deactivate: vi.fn(),
      state: "off" as const,
    },
    networkQuality: "good" as const,
    reconnectState: {
      active: true,
      attempt: 1,
      maxAttempts: 5,
      nextRetryMs: 1000,
    },
    showPerfOverlay: true,
    togglePerfOverlay: vi.fn(),
    viewerMetrics: {
      bitrateKbps: 1000,
      codec: "raw",
      fps: 30,
      latencyMs: 12,
      resolution: "1920x1080",
      transport: "vnc",
    },
    virtualKeyboard: {
      isTouchDevice: true,
      toggle: vi.fn(),
    },
  },
  focusActiveViewer: vi.fn(),
  restoreViewerFocus: vi.fn(),
}));

vi.mock("../../../../components/RemoteViewerShell", () => ({
  RemoteViewerShell: (props: RemoteViewerShellProps) => {
    mocks.shellProps = props;
    return (
      <div ref={props.viewerWrapperRef} data-testid="remote-view-shell">
        {props.credentialOverlay}
      </div>
    );
  },
}));

vi.mock("../../../../hooks/useConnectedAgents", () => ({
  useConnectedAgents: () => ({ connectedAgentIds: new Set(["asset-1"]) }),
}));
vi.mock("../../../../hooks/useDisplayList", () => ({
  useDisplayList: () => ({
    displays: [
      { name: "Display 1", width: 1920, height: 1080, primary: true },
      { name: "Display 2", width: 1280, height: 720, primary: false },
    ],
    error: null,
    loading: false,
    refresh: vi.fn(),
  }),
}));
vi.mock("../../../../hooks/useFullscreen", () => ({
  useFullscreen: () => ({ isFullscreen: false, toggleFullscreen: vi.fn() }),
}));
vi.mock("../../../../hooks/useLatency", () => ({
  useLatency: () => 12,
}));
vi.mock("../../../../hooks/useSession", () => ({
  useSession: () => mocks.session,
}));
vi.mock("../../nodes/[id]/useDesktopViewerFocus", () => ({
  useDesktopViewerFocus: () => ({
    focusActiveViewer: mocks.focusActiveViewer,
    restoreViewerFocus: mocks.restoreViewerFocus,
  }),
}));
vi.mock("../../nodes/[id]/useDesktopViewerRuntime", () => ({
  useDesktopViewerRuntime: () => mocks.runtime,
}));

import RemoteViewSession from "../RemoteViewSession";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let fetchMock: ReturnType<typeof vi.fn>;

const tab = {
  id: "tab-1",
  type: "device" as const,
  label: "Test Device",
  protocol: "vnc" as const,
  target: {
    assetId: "asset-1",
    host: "",
    port: 5900,
  },
  connectionState: "connected" as const,
};

beforeEach(() => {
  mocks.shellProps = null;
  for (const fn of [
    mocks.session.connect,
    mocks.session.disconnect,
    mocks.session.handleConnected,
    mocks.session.handleDisconnected,
    mocks.session.handleError,
    mocks.session.setQuality,
    mocks.runtime.adaptiveQuality.applyQuality,
    mocks.runtime.adaptiveQuality.dismissSuggestion,
    mocks.runtime.adaptiveQuality.getNextQuality,
    mocks.runtime.adaptiveQuality.setAutoEnabled,
    mocks.runtime.clipboard.pullFromRemote,
    mocks.runtime.clipboard.pushToRemote,
    mocks.runtime.fileDownload.downloadFile,
    mocks.runtime.handleSendShortcut,
    mocks.runtime.keyboardGrab.activate,
    mocks.runtime.keyboardGrab.deactivate,
    mocks.runtime.togglePerfOverlay,
    mocks.runtime.virtualKeyboard.toggle,
    mocks.focusActiveViewer,
    mocks.restoreViewerFocus,
  ]) {
    fn.mockClear();
  }
  mocks.runtime.adaptiveQuality.getNextQuality.mockReturnValue("low");
  fetchMock = vi.fn(async () =>
    new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => {
    root.unmount();
  });
  container.remove();
  vi.unstubAllGlobals();
});

async function renderSession(): Promise<RemoteViewerShellProps> {
  await act(async () => {
    root.render(
      <RemoteViewSession
        tab={tab}
        onConnectionStateChange={vi.fn()}
      />,
    );
  });
  if (!mocks.shellProps) throw new Error("RemoteViewerShell was not rendered");
  return mocks.shellProps;
}

describe("RemoteViewSession feature wiring", () => {
  it("exposes the complete viewer runtime only for the managed target", async () => {
    const props = await renderSession();

    expect(props.metrics).toBe(mocks.runtime.viewerMetrics);
    expect(props.showPerformanceOverlay).toBe(true);
    expect(props.onPerformanceOverlayToggle).toBe(
      mocks.runtime.togglePerfOverlay,
    );
    expect(props.keyboardGrabState).toBe("off");
    expect(props.onSendShortcut).toBe(mocks.runtime.handleSendShortcut);
    expect(props.reconnectState).toEqual(mocks.runtime.reconnectState);
    expect(props.isTouchDevice).toBe(true);
    expect(props.onClipboardPull).toBe(
      mocks.runtime.clipboard.pullFromRemote,
    );
    expect(props.onClipboardPush).toBe(
      mocks.runtime.clipboard.pushToRemote,
    );
    expect(props.onDownloadFile).toBe(
      mocks.runtime.fileDownload.downloadFile,
    );
    expect(props.displays).toHaveLength(2);
    expect(props.onDisplayChange).toBeTypeOf("function");
    expect(props.adaptiveQualityEnabled).toBe(false);

    props.onQualitySuggestionApply?.();
    expect(mocks.session.setQuality).toHaveBeenCalledWith("low");
    expect(mocks.runtime.adaptiveQuality.applyQuality).toHaveBeenCalledWith(
      "low",
    );
  });

  it("preserves exact VNC credentials and performs real recording lifecycle calls", async () => {
    let props = await renderSession();
    const sendCredentials = vi.fn();
    Object.assign(props.vncRef, {
      current: {
        disconnect: vi.fn(),
        exitPointerLock: vi.fn(),
        requestPointerLock: vi.fn(),
        sendCredentials,
      },
    });

    props.onCredentialsRequired({ types: ["password"] });
    expect(sendCredentials).toHaveBeenCalledWith({
      password: "  exact ticket password\n",
    });

    await act(async () => {
      props.onToggleRecording();
      await Promise.resolve();
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/recordings",
      expect.objectContaining({ method: "POST" }),
    );
    const startInit = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(startInit.body))).toEqual({
      session_id: "session-1",
    });

    if (!mocks.shellProps) throw new Error("shell props missing after start");
    props = mocks.shellProps;
    await act(async () => {
      props.onToggleRecording();
      await Promise.resolve();
    });
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/recordings/session-1",
      { method: "POST" },
    );
  });

  it("requests pointer lock and reconnects against a selected monitor", async () => {
    const props = await renderSession();
    const requestPointerLock = vi.fn();
    const disconnectViewer = vi.fn();
    Object.assign(props.vncRef, {
      current: {
        disconnect: disconnectViewer,
        exitPointerLock: vi.fn(),
        requestPointerLock,
      },
    });

    props.onPointerLockToggle();
    expect(requestPointerLock).toHaveBeenCalledTimes(1);

    await act(async () => {
      props.onDisplayChange?.("Display 2");
      await Promise.resolve();
    });
    expect(disconnectViewer).toHaveBeenCalledTimes(1);
    expect(mocks.session.disconnect).toHaveBeenCalledTimes(1);
    expect(mocks.session.connect).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({
        protocol: "vnc",
        display: "Display 2",
        record: false,
      }),
    );
  });
});
