export type DesktopBrowserMockOptions = {
  webrtcStatsProfile?: "good" | "fair" | "poor";
  webrtcRouteType?: "direct" | "reflexive" | "relay";
  accelerateReconnectTimers?: boolean;
  vncDisconnects?: Array<{
    afterMs: number;
    clean: boolean;
    reason?: string;
    beforeConnect?: boolean;
  }>;
  vncCredentialPrompts?: Array<{
    types: string[];
    outcome: "success" | "securityfailure" | "repeatprompt";
    reason?: string;
  }>;
  audioBehaviors?: Array<{
    state: "started" | "unavailable";
    error?: string;
  }>;
};

// This function is serialized into the browser; keep runtime dependencies local.
export function initDesktopBrowserEnvironment(config: DesktopBrowserMockOptions) {
  type WsRecord = { url: string; payload: string };
  type InputRecord = { label: string; payload: string };
  type DisconnectPlan = {
    afterMs: number;
    clean: boolean;
    reason?: string;
    beforeConnect?: boolean;
  };
  type CredentialPromptPlan = {
    types: string[];
    outcome: "success" | "securityfailure" | "repeatprompt";
    reason?: string;
  };
  type AudioBehavior = {
    state: "started" | "unavailable";
    error?: string;
  };

  const settings = {
    webrtcStatsProfile: config.webrtcStatsProfile ?? "good",
    webrtcRouteType: config.webrtcRouteType ?? "direct",
    accelerateReconnectTimers: config.accelerateReconnectTimers ?? false,
    vncDisconnects: (config.vncDisconnects ?? []) as DisconnectPlan[],
    vncCredentialPrompts:
      (config.vncCredentialPrompts ?? []) as CredentialPromptPlan[],
    audioBehaviors: (config.audioBehaviors ?? []) as AudioBehavior[],
  };

  const auditState = {
    websocketMessages: [] as WsRecord[],
    inputMessages: [] as InputRecord[],
    clipboardLocal: "local clipboard text",
    clipboardRemoteReadText: "remote clipboard text",
    clipboardRemoteWriteText: "",
    fileTransfers: [] as Array<{
      requestId: string;
      name: string;
      path: string;
      chunks: string[];
    }>,
    mediaRecorderStarts: 0,
    mediaRecorderStops: 0,
    vncConnections: [] as string[],
    audioConnections: [] as string[],
    webrtcStatsProfile: config.webrtcStatsProfile ?? "good",
    webrtcRouteType: config.webrtcRouteType ?? "direct",
    webrtcEvents: [] as string[],
    vncRuntime: {
      scaleViewport: true,
      resizeSession: false,
      clipViewport: false,
      dragViewport: false,
      qualityLevel: 6,
      compressionLevel: 6,
      viewOnly: false,
    },
  };

  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: {
      readText: async () => auditState.clipboardLocal,
      writeText: async (next: unknown) => {
        auditState.clipboardLocal =
          typeof next === "string" ? next : String(next ?? "");
      },
    },
  });

  Object.defineProperty(window, "__desktopAudit", {
    configurable: true,
    value: auditState,
    writable: false,
  });

  Object.defineProperty(navigator, "keyboard", {
    configurable: true,
    value: {
      lock: async () => undefined,
      unlock: () => undefined,
    },
  });

  let pointerLockTarget: Element | null = null;
  Object.defineProperty(document, "pointerLockElement", {
    configurable: true,
    get: () => pointerLockTarget,
  });
  HTMLElement.prototype.requestPointerLock = async function requestPointerLock() {
    pointerLockTarget = this;
    document.dispatchEvent(new Event("pointerlockchange"));
  };
  document.exitPointerLock = () => {
    pointerLockTarget = null;
    document.dispatchEvent(new Event("pointerlockchange"));
    return Promise.resolve();
  };

  if (settings.accelerateReconnectTimers) {
    const nativeSetTimeout = window.setTimeout.bind(window);
    window.setTimeout = ((handler: TimerHandler, timeout?: number, ...args: unknown[]) => {
      const mapped =
        timeout === 1000
          ? 20
          : timeout === 2000
            ? 40
            : timeout === 6000
              ? 120
              : timeout === 8000
                ? 160
                : timeout === 12000
                  ? 240
                  : timeout === 15000
                    ? 300
            : timeout === 4000
              ? 80
                : timeout === 16000
                  ? 320
                  : timeout;
      return nativeSetTimeout(handler, mapped, ...args);
    }) as typeof window.setTimeout;
  }
  return { settings, auditState };
}

export type DesktopMockEnvironment = ReturnType<typeof initDesktopBrowserEnvironment>;
