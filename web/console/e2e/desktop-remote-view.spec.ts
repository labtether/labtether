import { expect, test, type Page } from "@playwright/test";

import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./helpers/consoleApiMocks";

const BASE_TS = "2026-01-01T12:00:00.000Z";

type DesktopBrowserMockOptions = {
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

function makeDesktopAsset(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-host-1",
    name: "Lab Host",
    type: "host",
    source: "agent",
    status: "online",
    platform: "linux",
    last_seen_at: BASE_TS,
    metadata: {
      hostname: "lab-host",
      webrtc_available: "true",
    },
    ...overrides,
  };
}

const DISPLAY_LIST = [
  {
    name: "Display 1",
    width: 2560,
    height: 1440,
    primary: true,
    offset_x: 0,
    offset_y: 0,
  },
  {
    name: "Display 2",
    width: 1920,
    height: 1080,
    primary: false,
    offset_x: 2560,
    offset_y: 0,
  },
];

async function installDesktopBrowserMocks(
  page: Page,
  options: DesktopBrowserMockOptions = {},
) {
  await page.addInitScript((config: DesktopBrowserMockOptions) => {
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

    HTMLMediaElement.prototype.play = function mockPlay() {
      return Promise.resolve();
    };
    HTMLMediaElement.prototype.pause = function mockPause() {
      return undefined;
    };

    class MockSourceBuffer {
      updating = false;
      mode = "sequence";
      private updateEndListeners: Array<(event: Event) => void> = [];

      addEventListener(type: string, listener: (event: Event) => void) {
        if (type === "updateend") {
          this.updateEndListeners.push(listener);
        }
      }

      appendBuffer(_buffer: ArrayBuffer) {
        this.updating = true;
        window.setTimeout(() => {
          this.updating = false;
          for (const listener of this.updateEndListeners) {
            listener(new Event("updateend"));
          }
        }, 0);
      }
    }

    class MockMediaSource {
      static isTypeSupported() {
        return true;
      }

      readyState: "closed" | "open" | "ended" = "closed";
      private sourceOpenListeners: Array<(event: Event) => void> = [];

      constructor() {
        window.setTimeout(() => {
          this.readyState = "open";
          for (const listener of this.sourceOpenListeners) {
            listener(new Event("sourceopen"));
          }
        }, 0);
      }

      addEventListener(type: string, listener: (event: Event) => void) {
        if (type === "sourceopen") {
          this.sourceOpenListeners.push(listener);
        }
      }

      removeEventListener(type: string, listener: (event: Event) => void) {
        if (type !== "sourceopen") {
          return;
        }
        this.sourceOpenListeners = this.sourceOpenListeners.filter(
          (candidate) => candidate !== listener,
        );
      }

      addSourceBuffer() {
        return new MockSourceBuffer() as unknown as SourceBuffer;
      }

      endOfStream() {
        this.readyState = "ended";
      }
    }

    Object.defineProperty(window, "MediaSource", {
      configurable: true,
      writable: true,
      value: MockMediaSource,
    });

    URL.createObjectURL = () => "blob:desktop-audio";
    URL.revokeObjectURL = () => undefined;
    const NativeWebSocket = window.WebSocket;

    class MockDesktopWebSocket {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;

      readyState = MockDesktopWebSocket.CONNECTING;
      binaryType: BinaryType = "blob";
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;
      url: string;
      private delegate: WebSocket | null = null;
      private listeners = new Map<string, Array<(event: Event) => void>>();

      constructor(url: string) {
        this.url = url;
        if (!this.url.includes("desktop-webrtc") && !this.url.includes("desktop-vnc") && !this.url.includes("desktop-audio")) {
          const socket = new NativeWebSocket(url);
          this.delegate = socket;
          socket.binaryType = this.binaryType;
          socket.addEventListener("open", (event) => {
            this.readyState = socket.readyState;
            this.onopen?.(event);
            this.dispatch("open", event);
          });
          socket.addEventListener("message", (event) => {
            this.readyState = socket.readyState;
            this.onmessage?.(event);
            this.dispatch("message", event);
          });
          socket.addEventListener("error", (event) => {
            this.readyState = socket.readyState;
            this.onerror?.(event);
            this.dispatch("error", event);
          });
          socket.addEventListener("close", (event) => {
            this.readyState = socket.readyState;
            this.onclose?.(event);
            this.dispatch("close", event);
          });
          return;
        }
        window.setTimeout(() => {
          if (this.readyState !== MockDesktopWebSocket.CONNECTING) {
            return;
          }
          this.readyState = MockDesktopWebSocket.OPEN;
          const openEvent = new Event("open");
          this.onopen?.(openEvent);
          this.dispatch("open", openEvent);
          if (this.url.includes("desktop-webrtc")) {
            auditState.webrtcEvents.push("ws:ready");
            this.emit({ type: "ready", data: {} });
            return;
          }
          if (this.url.includes("desktop-audio")) {
            auditState.audioConnections.push(this.url);
            const behavior =
              settings.audioBehaviors[auditState.audioConnections.length - 1] ??
              ({ state: "started" } as AudioBehavior);
            this.emit({
              state: behavior.state,
              ...(behavior.error ? { error: behavior.error } : {}),
            });
          }
        }, 0);
      }

      send(data: unknown) {
        if (this.delegate) {
          // TS 6's lib.dom typings tightened WebSocket.send to require
          // BufferSource, which excludes SharedArrayBuffer. Cast to the
          // narrower set of types the mock is known to pass through.
          this.delegate.send(data as string | ArrayBuffer | Blob | ArrayBufferView<ArrayBuffer>);
          return;
        }
        const payload = typeof data === "string" ? data : String(data);
        auditState.websocketMessages.push({ url: this.url, payload });
        if (!this.url.includes("desktop-webrtc")) {
          return;
        }
        try {
          const parsed = JSON.parse(payload) as { type?: string };
          if (parsed.type === "offer") {
            auditState.webrtcEvents.push("ws:offer");
            this.emit({
              type: "answer",
              data: { sdp: "mock-answer-sdp" },
            });
          }
        } catch {
          // Ignore malformed frames.
        }
      }

      close(code = 1000, reason = "") {
        if (this.delegate) {
          this.delegate.close(code, reason);
          return;
        }
        this.readyState = MockDesktopWebSocket.CLOSED;
        const closeEvent = {
          code,
          reason,
          wasClean: code === 1000,
        } as CloseEvent;
        this.onclose?.(closeEvent);
        this.dispatch("close", closeEvent);
      }

      addEventListener(type: string, listener: (event: Event) => void) {
        const existing = this.listeners.get(type) ?? [];
        existing.push(listener);
        this.listeners.set(type, existing);
      }

      removeEventListener(type: string, listener: (event: Event) => void) {
        const existing = this.listeners.get(type) ?? [];
        this.listeners.set(
          type,
          existing.filter((candidate) => candidate !== listener),
        );
      }

      private emit(payload: unknown) {
        const event = {
          data: JSON.stringify(payload),
        } as MessageEvent;
        this.onmessage?.(event);
        this.dispatch("message", event);
      }

      private dispatch(type: string, event: Event) {
        const listeners = this.listeners.get(type) ?? [];
        for (const listener of listeners) {
          listener(event);
        }
      }
    }

    class MockMediaStreamTrack {
      kind = "video";

      getSettings() {
        return {
          width: 4480,
          height: 1440,
        };
      }
    }

    class MockDataChannel {
      label: string;
      readyState = "open";
      onmessage: ((event: MessageEvent) => void) | null = null;
      onopen: ((event: Event) => void) | null = null;
      onclose: ((event: Event) => void) | null = null;

      constructor(label: string) {
        this.label = label;
        window.setTimeout(() => {
          this.onopen?.(new Event("open"));
        }, 0);
      }

      send(payload: string) {
        auditState.inputMessages.push({ label: this.label, payload });
        try {
          if (this.label === "clipboard") {
            const parsed = JSON.parse(payload) as {
              type?: string;
              text?: string;
            };
            if (parsed.type === "get") {
              this.onmessage?.({
                data: JSON.stringify({
                  type: "data",
                  text: auditState.clipboardRemoteReadText,
                }),
              } as MessageEvent);
              return;
            }
            if (parsed.type === "set") {
              auditState.clipboardRemoteWriteText = parsed.text ?? "";
              this.onmessage?.({
                data: JSON.stringify({ type: "ack" }),
              } as MessageEvent);
            }
            return;
          }
          if (this.label === "file-transfer") {
            const parsed = JSON.parse(payload) as {
              type?: string;
              request_id?: string;
              name?: string;
              path?: string;
              data?: string;
            };
            const requestId = parsed.request_id?.trim() ?? "";
            if (!requestId) {
              return;
            }
            if (parsed.type === "start") {
              auditState.fileTransfers.push({
                requestId,
                name: parsed.name ?? "",
                path: parsed.path ?? "",
                chunks: [],
              });
              this.onmessage?.({
                data: JSON.stringify({ type: "ready", request_id: requestId }),
              } as MessageEvent);
              return;
            }
            if (parsed.type === "chunk") {
              const transfer = auditState.fileTransfers.find(
                (entry) => entry.requestId === requestId,
              );
              if (transfer && typeof parsed.data === "string") {
                transfer.chunks.push(parsed.data);
              }
              this.onmessage?.({
                data: JSON.stringify({ type: "ack", request_id: requestId }),
              } as MessageEvent);
            }
          }
        } catch {
          // Ignore malformed data channel payloads.
        }
      }

      close() {
        this.readyState = "closed";
        this.onclose?.(new Event("close"));
      }
    }

    class MockRTCPeerConnection {
      connectionState: RTCPeerConnectionState = "new";
      localDescription: RTCSessionDescriptionInit | null = null;
      remoteDescription: RTCSessionDescriptionInit | null = null;
      ontrack: ((event: RTCTrackEvent) => void) | null = null;
      onicecandidate: ((event: RTCPeerConnectionIceEvent) => void) | null =
        null;
      onconnectionstatechange: (() => void) | null = null;
      private readonly stats = new Map<string, RTCStats>();

      constructor() {
      }

      addTransceiver() {
        return {};
      }

      createDataChannel(label: string) {
        return new MockDataChannel(label) as unknown as RTCDataChannel;
      }

      async createOffer() {
        auditState.webrtcEvents.push("pc:create-offer");
        return {
          type: "offer",
          sdp: "mock-offer-sdp",
        } as RTCSessionDescriptionInit;
      }

      async setLocalDescription(description: RTCSessionDescriptionInit) {
        this.localDescription = description;
      }

      async setRemoteDescription(description: RTCSessionDescriptionInit) {
        auditState.webrtcEvents.push("pc:set-remote-description");
        this.remoteDescription = description;
        const stream = new MediaStream();
        const videoTrack =
          new MockMediaStreamTrack() as unknown as MediaStreamTrack;
        Object.defineProperty(stream, "getVideoTracks", {
          configurable: true,
          value: () => [videoTrack],
        });
        Object.defineProperty(stream, "getAudioTracks", {
          configurable: true,
          value: () => [],
        });
        this.ontrack?.({
          streams: [stream],
        } as unknown as RTCTrackEvent);
        this.connectionState = "connected";
        auditState.webrtcEvents.push("pc:connected");
        this.onconnectionstatechange?.();
      }

      async addIceCandidate() {
        return;
      }

      async getStats() {
        this.stats.clear();
        const profile = auditState.webrtcStatsProfile;
        const routeType = auditState.webrtcRouteType;
        const metrics =
          profile === "poor"
            ? { roundTripTime: 0.28, packetsLost: 8, fps: 8, bytesReceived: 30720 }
            : profile === "fair"
              ? { roundTripTime: 0.13, packetsLost: 2, fps: 18, bytesReceived: 196608 }
              : { roundTripTime: 0.04, packetsLost: 0, fps: 30, bytesReceived: 409600 };

        this.stats.set("remote-inbound-video", {
          id: "remote-inbound-video",
          type: "remote-inbound-rtp",
          timestamp: Date.now(),
          kind: "video",
          roundTripTime: metrics.roundTripTime,
        } as RTCStats);
        this.stats.set("inbound-video", {
          id: "inbound-video",
          type: "inbound-rtp",
          timestamp: Date.now(),
          kind: "video",
          packetsLost: metrics.packetsLost,
          framesPerSecond: metrics.fps,
          bytesReceived: metrics.bytesReceived,
        } as RTCStats);
        this.stats.set("transport-1", {
          id: "transport-1",
          type: "transport",
          timestamp: Date.now(),
          selectedCandidatePairId: "candidate-pair-1",
        } as RTCStats);
        this.stats.set("candidate-pair-1", {
          id: "candidate-pair-1",
          type: "candidate-pair",
          timestamp: Date.now(),
          selected: true,
          localCandidateId: "local-candidate-1",
          remoteCandidateId: "remote-candidate-1",
          currentRoundTripTime: metrics.roundTripTime,
        } as RTCStats);
        this.stats.set("local-candidate-1", {
          id: "local-candidate-1",
          type: "local-candidate",
          timestamp: Date.now(),
          candidateType: routeType === "relay" ? "relay" : routeType === "reflexive" ? "srflx" : "host",
        } as RTCStats);
        this.stats.set("remote-candidate-1", {
          id: "remote-candidate-1",
          type: "remote-candidate",
          timestamp: Date.now(),
          candidateType: routeType === "relay" ? "relay" : routeType === "reflexive" ? "prflx" : "host",
        } as RTCStats);
        return this.stats;
      }

      close() {
        this.connectionState = "closed";
        this.onconnectionstatechange?.();
      }
    }

    class MockMediaRecorder {
      static isTypeSupported() {
        return true;
      }

      mimeType: string;
      ondataavailable: ((event: BlobEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onstop: (() => void) | null = null;

      constructor(_stream: MediaStream, options?: { mimeType?: string }) {
        this.mimeType = options?.mimeType ?? "video/webm";
      }

      start() {
        auditState.mediaRecorderStarts += 1;
      }

      stop() {
        auditState.mediaRecorderStops += 1;
        this.ondataavailable?.({
          data: new Blob(["desktop-recording"], { type: this.mimeType }),
        } as BlobEvent);
        this.onstop?.();
      }
    }

    class MockRFB {
      private scaleViewportValue = true;
      private resizeSessionValue = false;
      private clipViewportValue = false;
      private dragViewportValue = false;
      showDotCursor = true;
      private qualityLevelValue = 6;
      private compressionLevelValue = 6;
      private viewOnlyValue = false;
      private readonly canvas: HTMLCanvasElement;
      private disconnected = false;
      private readonly listeners = new Map<string, Array<(event: Event) => void>>();
      private readonly url: string;
      private credentialPrompt:
        | CredentialPromptPlan
        | null;
      private credentialAttempts = 0;
      private disconnectPlan: DisconnectPlan | null = null;

      constructor(
        target: HTMLDivElement,
        url: string,
        _options: { wsProtocols: string[] },
      ) {
        const canvas = document.createElement("canvas");
        canvas.tabIndex = -1;
        canvas.style.cursor = "none";
        target.appendChild(canvas);
        this.canvas = canvas;

        this.url = url;
        this.credentialPrompt = null;

        window.setTimeout(() => {
          if (this.disconnected) {
            return;
          }
          auditState.vncConnections.push(url);
          this.credentialPrompt =
            settings.vncCredentialPrompts[auditState.vncConnections.length - 1] ??
            null;
          this.disconnectPlan =
            settings.vncDisconnects[auditState.vncConnections.length - 1] ?? null;

          if (this.credentialPrompt) {
            this.emit("credentialsrequired", {
              detail: {
                types: this.credentialPrompt.types,
              },
            } as unknown as Event);
          } else if (!this.disconnectPlan?.beforeConnect) {
            this.emit("connect", new Event("connect"));
          }

          if (!this.disconnectPlan) {
            return;
          }
          window.setTimeout(() => {
            if (this.disconnected) {
              return;
            }
            this.disconnected = true;
            this.emit("disconnect", {
              detail: {
                clean: this.disconnectPlan?.clean ?? false,
                reason:
                  this.disconnectPlan?.reason ??
                  (this.disconnectPlan?.clean
                    ? "user disconnected"
                    : "network interrupted"),
              },
            } as unknown as Event);
          }, this.disconnectPlan.afterMs);
        }, 0);

        canvas.addEventListener("keydown", (event) => {
          auditState.inputMessages.push({
            label: "vnc-dom-keydown",
            payload: JSON.stringify({ key: event.key, code: event.code }),
          });
        });
        canvas.addEventListener("keyup", (event) => {
          auditState.inputMessages.push({
            label: "vnc-dom-keyup",
            payload: JSON.stringify({ key: event.key, code: event.code }),
          });
        });

      }

      private syncRuntimeState() {
        auditState.vncRuntime = {
          scaleViewport: this.scaleViewportValue,
          resizeSession: this.resizeSessionValue,
          clipViewport: this.clipViewportValue,
          dragViewport: this.dragViewportValue,
          qualityLevel: this.qualityLevelValue,
          compressionLevel: this.compressionLevelValue,
          viewOnly: this.viewOnlyValue,
        };
      }

      get scaleViewport() {
        return this.scaleViewportValue;
      }

      set scaleViewport(value: boolean) {
        this.scaleViewportValue = value;
        this.syncRuntimeState();
      }

      get resizeSession() {
        return this.resizeSessionValue;
      }

      set resizeSession(value: boolean) {
        this.resizeSessionValue = value;
        this.syncRuntimeState();
      }

      get clipViewport() {
        return this.clipViewportValue;
      }

      set clipViewport(value: boolean) {
        this.clipViewportValue = value;
        this.syncRuntimeState();
      }

      get dragViewport() {
        return this.dragViewportValue;
      }

      set dragViewport(value: boolean) {
        this.dragViewportValue = value;
        this.syncRuntimeState();
      }

      get qualityLevel() {
        return this.qualityLevelValue;
      }

      set qualityLevel(value: number) {
        this.qualityLevelValue = value;
        this.syncRuntimeState();
      }

      get compressionLevel() {
        return this.compressionLevelValue;
      }

      set compressionLevel(value: number) {
        this.compressionLevelValue = value;
        this.syncRuntimeState();
      }

      get viewOnly() {
        return this.viewOnlyValue;
      }

      set viewOnly(value: boolean) {
        this.viewOnlyValue = value;
        this.syncRuntimeState();
      }

      focus() {
        this.canvas.focus();
      }

      addEventListener(type: string, listener: (event: Event) => void) {
        const listeners = this.listeners.get(type) ?? [];
        listeners.push(listener);
        this.listeners.set(type, listeners);
      }

      disconnect() {
        if (this.disconnected) {
          return;
        }
        this.disconnected = true;
        this.emit("disconnect", {
          detail: {
            clean: true,
            reason: "user disconnected",
          },
        } as unknown as Event);
      }

      sendCtrlAltDel() {
        auditState.inputMessages.push({
          label: "vnc-ctrl-alt-del",
          payload: this.url,
        });
      }

      sendCredentials(creds: { username?: string; password?: string }) {
        auditState.inputMessages.push({
          label: "vnc-credentials",
          payload: JSON.stringify(creds),
        });
        if (!this.credentialPrompt || this.disconnected) {
          return;
        }
        this.credentialAttempts += 1;
        if (this.credentialPrompt.outcome === "securityfailure") {
          const reason =
            this.credentialPrompt.reason ?? "Authentication failed";
          this.emit("securityfailure", {
            detail: {
              status: 1,
              reason,
            },
          } as unknown as Event);
          this.disconnected = true;
          this.emit("disconnect", {
            detail: {
              clean: false,
              reason,
            },
          } as unknown as Event);
          return;
        }
        if (
          this.credentialPrompt.outcome === "repeatprompt" &&
          this.credentialAttempts === 1
        ) {
          this.emit("credentialsrequired", {
            detail: {
              types: this.credentialPrompt.types,
            },
          } as unknown as Event);
          return;
        }
        this.emit("connect", new Event("connect"));
      }

      clipboardPasteFrom(text: string) {
        auditState.inputMessages.push({
          label: "vnc-clipboard",
          payload: text,
        });
      }

      sendKey(keysym: number, _code?: string, down?: boolean) {
        auditState.inputMessages.push({
          label: "vnc-key",
          payload: JSON.stringify({ keysym, down: down ?? false }),
        });
      }

      private emit(type: string, event: Event) {
        const listeners = this.listeners.get(type) ?? [];
        for (const listener of listeners) {
          listener(event);
        }
      }
    }

    (window as unknown as { WebSocket: unknown }).WebSocket =
      MockDesktopWebSocket;
    (
      window as unknown as { RTCPeerConnection: unknown }
    ).RTCPeerConnection = MockRTCPeerConnection;
    (window as unknown as { MediaRecorder: unknown }).MediaRecorder =
      MockMediaRecorder;
    (
      window as unknown as { __labtetherTestRFBClass: unknown }
    ).__labtetherTestRFBClass = MockRFB;
  }, options);
}

async function installDesktopApiMocks(
  page: Page,
  options: {
    sessionRequests: Array<Record<string, unknown>>;
    fileUploads?: Array<{ path: string; body: string }>;
    streamTicketStatus?: number;
    vncStreamTicketStatus?: number;
    webrtcStreamTicketStatus?: number;
    vncPassword?: string;
    assetOverrides?: Record<string, unknown>;
    connectedAgentAssetIDs?: string[];
  },
) {
  const asset = makeDesktopAsset(options.assetOverrides);
  let sessionCounter = 0;
  let lastSessionProtocol = "webrtc";
  let remoteClipboardText = "remote clipboard text";
  const fileUploads = options.fileUploads ?? [];
  const connectedAgentAssetIDs = options.connectedAgentAssetIDs ?? [asset.id];

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets: [asset] }),
    liveStatusPayload: buildLiveStatusPayload({ assets: [asset] }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON, route, url }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: connectedAgentAssetIDs });
        return true;
      }
      if (pathname === `/api/displays/${encodeURIComponent(asset.id)}`) {
        await fulfillJSON({ displays: DISPLAY_LIST });
        return true;
      }
      if (pathname === `/api/v1/nodes/${asset.id}/displays`) {
        await fulfillJSON({ displays: DISPLAY_LIST });
        return true;
      }
      if (pathname === `/api/v1/nodes/${asset.id}/clipboard/get` && method === "POST") {
        await fulfillJSON({ format: "text", text: remoteClipboardText });
        return true;
      }
      if (pathname === `/api/v1/nodes/${asset.id}/clipboard/set` && method === "POST") {
        remoteClipboardText =
          typeof requestBody.text === "string" ? requestBody.text : "";
        await fulfillJSON({ ok: true });
        return true;
      }
      if (pathname === `/api/files/${asset.id}/upload` && method === "POST") {
        fileUploads.push({
          path: url.searchParams.get("path") ?? "",
          body: route.request().postData() ?? "",
        });
        await fulfillJSON({ ok: true });
        return true;
      }
      if (pathname === "/api/desktop/session" && method === "POST") {
        options.sessionRequests.push({ ...requestBody });
        sessionCounter += 1;
        if (typeof requestBody?.protocol === "string") {
          lastSessionProtocol = requestBody.protocol;
        }
        await fulfillJSON({
          session: { id: `desktop-session-${sessionCounter}` },
        });
        return true;
      }
      if (pathname === "/api/desktop/stream-ticket" && method === "POST") {
        const streamTicketStatus =
          lastSessionProtocol === "vnc"
            ? options.vncStreamTicketStatus ?? options.streamTicketStatus
            : options.webrtcStreamTicketStatus ?? options.streamTicketStatus;
        if (streamTicketStatus && streamTicketStatus >= 400) {
          await fulfillJSON(
            { error: `Failed to get stream ticket (${streamTicketStatus})` },
            streamTicketStatus,
          );
          return true;
        }
        if (lastSessionProtocol === "vnc") {
          await fulfillJSON({
            wsUrl: `ws://desktop-vnc.test/session-${sessionCounter}`,
            audioWsUrl: `ws://desktop-audio.test/session-${sessionCounter}`,
            vncPassword: options.vncPassword,
            secure: false,
          });
          return true;
        }
        await fulfillJSON({
          wsUrl: `ws://desktop-webrtc.test/session-${sessionCounter}`,
          secure: false,
        });
        return true;
      }
      return false;
    },
  });
}

test("desktop protocol switching hides stale display selections before WebRTC connect", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await expect(page.getByRole("button", { name: "Connect" })).toBeVisible();
  await expect(selectors.nth(1)).toHaveValue("webrtc");
  await expect(page.getByText("Display", { exact: true })).toHaveCount(0);

  await selectors.nth(1).selectOption("vnc");
  await expect(page.getByText("Display", { exact: true })).toBeVisible();
  await selectors.nth(2).selectOption("Display 2");

  await selectors.nth(1).selectOption("webrtc");
  await expect(page.getByText("Display", { exact: true })).toHaveCount(0);

  await page.getByRole("button", { name: "Connect" }).click();
  await expect
    .poll(() => sessionRequests.at(-1) ?? null)
    .toMatchObject({
      target: "agent-host-1",
      protocol: "webrtc",
      display: "",
      record: false,
    });
});

test("desktop Proxmox QEMU defaults to VNC while still offering SPICE", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncStreamTicketStatus: 500,
    assetOverrides: {
      id: "proxmox-vm-100",
      name: "ContainerVM",
      type: "vm",
      source: "proxmox",
      platform: "",
      metadata: {
        proxmox_type: "qemu",
      },
    },
    connectedAgentAssetIDs: [],
  });

  await page.goto("/nodes/proxmox-vm-100?panel=desktop");

  const selectors = page.locator("select");
  await expect(selectors.nth(1)).toHaveValue("vnc");
  await expect(selectors.nth(1)).toContainText("VNC (Recommended)");
  await expect(selectors.nth(1)).toContainText("SPICE (Optional)");
  await expect(page.getByText("Display", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
});

test("desktop VNC connect preserves the selected monitor in the session request", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await expect(page.getByText("Display", { exact: true })).toBeVisible();
  await selectors.nth(2).selectOption("Display 2");

  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1) ?? null)
    .toMatchObject({
      target: "agent-host-1",
      protocol: "vnc",
      display: "Display 2",
      record: false,
    });
});

test("desktop WebRTC renders multi-monitor streams as a stitched layout", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("webrtc");

  await expect(page.locator('[data-webrtc-layout="stitched"]')).toBeVisible();
  await expect(page.locator('[data-webrtc-display="Display 1"]')).toBeVisible();
  await expect(page.locator('[data-webrtc-display="Display 2"]')).toBeVisible();
});

test("desktop WebRTC native scaling uses a scrollable 1:1 container", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("webrtc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page
    .getByRole("group", { name: "Scaling mode" })
    .getByRole("button", { name: "1:1" })
    .click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const container = document.querySelector(".vncContainer");
        const stage = document.querySelector('[data-webrtc-layout="stitched"]');
        return {
          nativeClass: container?.className.includes("vncNative") ?? false,
          stageWidth:
            stage instanceof HTMLDivElement ? stage.style.width : null,
          stageHeight:
            stage instanceof HTMLDivElement ? stage.style.height : null,
          stageMaxWidth:
            stage instanceof HTMLDivElement ? stage.style.maxWidth : null,
        };
      }),
    )
    .toEqual({
      nativeClass: true,
      stageWidth: "4480px",
      stageHeight: "1440px",
      stageMaxWidth: "none",
    });
});

test("desktop VNC viewer focuses on click for input readiness", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  const viewer = page.locator(".vncContainer").first();
  await expect(viewer).toBeVisible();
  await viewer.click({ position: { x: 24, y: 24 } });

  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        return active instanceof Element
          ? Boolean(active.closest(".vncContainer"))
          : false;
      }),
    )
    .toBe(true);
});

test("desktop VNC native scaling keeps remote drag input enabled", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page
    .getByRole("group", { name: "Scaling mode" })
    .getByRole("button", { name: "1:1" })
    .click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            vncRuntime: {
              scaleViewport: boolean;
              resizeSession: boolean;
              clipViewport: boolean;
              dragViewport: boolean;
            };
          };
        };
        const runtime = win.__desktopAudit.vncRuntime;
        return {
          scaleViewport: runtime.scaleViewport,
          resizeSession: runtime.resizeSession,
          clipViewport: runtime.clipViewport,
          dragViewport: runtime.dragViewport,
        };
      }),
    )
    .toEqual({
      scaleViewport: false,
      resizeSession: false,
      clipViewport: true,
      dragViewport: false,
    });
});

test("desktop VNC toolbar shortcuts pass through to remote keys", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await page.getByTitle("Alt+Tab").click();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-key",
        );
      }),
    )
    .toEqual([
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65513, down: true }) },
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65289, down: true }) },
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65289, down: false }) },
      { label: "vnc-key", payload: JSON.stringify({ keysym: 65513, down: false }) },
    ]);
});

test("desktop VNC mouse button returns focus to the remote viewer without pointer lock", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await page.getByTitle("Send mouse to remote session").click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        return active instanceof Element
          ? Boolean(active.closest(".vncContainer"))
          : false;
      }),
    )
    .toBe(true);
  await expect
    .poll(() =>
      page.evaluate(() => document.pointerLockElement?.tagName ?? ""),
    )
    .toBe("");
});

test("desktop VNC keyboard capture returns typing to the remote viewer", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await page.getByTitle("Send keyboard to remote session").click();
  await expect
    .poll(() =>
      page.evaluate(() => document.pointerLockElement?.className ?? ""),
    )
    .toBe("");
  await page.keyboard.press("a");

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-dom-keydown",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-dom-keydown",
        payload: JSON.stringify({ key: "a", code: "KeyA" }),
      },
    ]);
});

test("desktop VNC toolbar supports clipboard sync through the agent API", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, {
    sessionRequests,
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  if ((await page.getByTitle("Pull clipboard from remote").count()) === 0) {
    await page.getByTitle("More viewer tools").click();
  }
  await expect(page.getByTitle("Pull clipboard from remote")).toBeVisible();
  await expect(page.getByTitle("Push clipboard to remote")).toBeVisible();

  await page.evaluate(async () => navigator.clipboard.writeText("local clipboard text"));
  await page.getByTitle("Push clipboard to remote").click();
  await expect
    .poll(() =>
      page.evaluate(async () => {
        const response = await fetch("/api/v1/nodes/agent-host-1/clipboard/get", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ format: "text" }),
        });
        const payload = (await response.json()) as { text?: string };
        return payload.text ?? "";
      }),
    )
    .toBe("local clipboard text");

  await page.evaluate(async () => {
    await fetch("/api/v1/nodes/agent-host-1/clipboard/set", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ format: "text", text: "remote clipboard text" }),
    });
  });
  await page.getByTitle("Pull clipboard from remote").click();
  await expect.poll(() => page.evaluate(async () => navigator.clipboard.readText())).toBe(
    "remote clipboard text",
  );
});

test("desktop VNC drag-and-drop upload falls back to the files API", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  const fileUploads: Array<{ path: string; body: string }> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests, fileUploads });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible({ timeout: 15000 });

  await page.evaluate(() => {
    const target = document.querySelector(
      ".vncViewerStage .relative.h-full.w-full",
    );
    if (!(target instanceof HTMLDivElement)) {
      throw new Error("file drop target unavailable");
    }
    const transfer = new DataTransfer();
    transfer.items.add(
      new File(["desktop upload"], "notes.txt", { type: "text/plain" }),
    );
    target.dispatchEvent(
      new DragEvent("dragenter", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("dragover", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("drop", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
  });

  await expect.poll(() => fileUploads).toEqual([
    {
      path: "~/Downloads/notes.txt",
      body: "desktop upload",
    },
  ]);
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            fileTransfers: Array<{
              requestId: string;
              name: string;
              path: string;
              chunks: string[];
            }>;
          };
        };
        return win.__desktopAudit.fileTransfers;
      }),
    )
    .toEqual([]);
});

test("desktop VNC shows a fallback cursor when the remote cursor is hidden", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("vnc");

  await expect(page.locator(".vncContainer").first()).toHaveAttribute(
    "data-cursor-fallback",
    "visible",
  );

  await page.getByTitle("Send mouse to remote session").click();

  await expect(page.locator(".vncContainer").first()).toHaveAttribute(
    "data-cursor-fallback",
    "visible",
  );
});

test("desktop WebRTC toolbar supports browser recording and clipboard sync", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("webrtc");
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: { webrtcEvents: string[] };
        };
        return win.__desktopAudit.webrtcEvents;
      }),
    )
    .toContain("pc:connected");
  await expect(page.getByTitle("Disconnect")).toBeVisible({ timeout: 15000 });
  await expect(page.getByTitle("Start recording")).toBeVisible({ timeout: 15000 });
  await expect(page.getByTitle("Pull clipboard from remote")).toBeVisible({ timeout: 15000 });
  await expect(page.getByTitle("Push clipboard to remote")).toBeVisible({ timeout: 15000 });
  await expect(page.getByLabel("Volume")).toBeVisible({ timeout: 15000 });

  await page.getByTitle("Start recording").click();
  await expect(page.getByTitle("Stop recording")).toBeVisible();
  await page.getByTitle("Stop recording").click();

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            mediaRecorderStarts: number;
            mediaRecorderStops: number;
          };
        };
        return {
          starts: win.__desktopAudit.mediaRecorderStarts,
          stops: win.__desktopAudit.mediaRecorderStops,
        };
      }),
    )
    .toEqual({ starts: 1, stops: 1 });

  await page.getByTitle("Push clipboard to remote").click();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: { clipboardRemoteWriteText: string };
        };
        return win.__desktopAudit.clipboardRemoteWriteText;
      }),
    )
    .toBe("local clipboard text");

  await page.getByTitle("Pull clipboard from remote").click();
  await expect.poll(() => page.evaluate(async () => navigator.clipboard.readText())).toBe(
    "remote clipboard text",
  );
});

test("desktop WebRTC drag-and-drop upload sends file transfer chunks", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("webrtc");
  await expect(page.getByTitle("Disconnect")).toBeVisible({ timeout: 15000 });

  await page.evaluate(() => {
    const target = document.querySelector(
      ".vncViewerStage .relative.h-full.w-full",
    );
    if (!(target instanceof HTMLDivElement)) {
      throw new Error("file drop target unavailable");
    }
    const transfer = new DataTransfer();
    transfer.items.add(
      new File(["desktop upload"], "notes.txt", { type: "text/plain" }),
    );
    target.dispatchEvent(
      new DragEvent("dragenter", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("dragover", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
    target.dispatchEvent(
      new DragEvent("drop", {
        bubbles: true,
        cancelable: true,
        dataTransfer: transfer,
      }),
    );
  });

  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            fileTransfers: Array<{
              requestId: string;
              name: string;
              path: string;
              chunks: string[];
            }>;
          };
        };
        return win.__desktopAudit.fileTransfers;
      }),
    )
    .toEqual([
      {
        requestId: expect.any(String),
        name: "notes.txt",
        path: "~/Downloads/notes.txt",
        chunks: ["ZGVza3RvcCB1cGxvYWQ="],
      },
    ]);
});

test("desktop fullscreen toolbar defaults to bottom and supports top toggle plus auto-hide", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page);
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(-1)?.protocol ?? null)
    .toBe("webrtc");

  await page.getByTitle("Fullscreen").click();
  await expect
    .poll(() => page.evaluate(() => document.fullscreenElement !== null))
    .toBe(true);

  const disconnectButton = page.getByTitle("Disconnect");
  const viewport = page.viewportSize();
  if (!viewport) {
    throw new Error("viewport unavailable");
  }

  const bottomBox = await disconnectButton.boundingBox();
  if (!bottomBox) {
    throw new Error("disconnect button not rendered");
  }
  expect(bottomBox.y).toBeGreaterThan(viewport.height * 0.65);

  await expect(page.getByTitle("More viewer tools")).toBeVisible();
  await page.getByTitle("More viewer tools").click();
  await expect(page.getByTitle("Alt+Tab")).toBeVisible();

  await page.getByTitle("Move toolbar to top").click();
  const topBox = await disconnectButton.boundingBox();
  if (!topBox) {
    throw new Error("disconnect button missing after moving toolbar");
  }
  expect(topBox.y).toBeLessThan(viewport.height * 0.35);

  await page.getByTitle("Enable auto-hide").click();
  await expect(page.getByTitle("Disable auto-hide")).toBeVisible();
  await page.waitForTimeout(5200);
  await expect(disconnectButton).toBeHidden();
  await expect(page.getByLabel("Show remote view tools")).toBeVisible();

  await page.mouse.move(Math.round(viewport.width / 2), 2);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
});

test("desktop relay-backed WebRTC falls back to VNC and recovers back to WebRTC", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    accelerateReconnectTimers: true,
    webrtcStatsProfile: "fair",
    webrtcRouteType: "relay",
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(0)?.protocol ?? null)
    .toBe("webrtc");
  await expect(page.getByText("webrtc relay")).toBeVisible();

  await expect
    .poll(() => sessionRequests.some((entry) => entry.protocol === "vnc"))
    .toBe(true);

  await page.evaluate(() => {
    const win = window as unknown as {
      __desktopAudit: {
        webrtcStatsProfile: "good" | "fair" | "poor";
        webrtcRouteType: "direct" | "reflexive" | "relay";
      };
    };
    win.__desktopAudit.webrtcStatsProfile = "good";
    win.__desktopAudit.webrtcRouteType = "direct";
  });

  await expect
    .poll(
      () => sessionRequests.filter((entry) => entry.protocol === "webrtc").length,
    )
    .toBe(2);
});

test("desktop VNC reconnect now preserves the chosen monitor and clears the queued retry", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncDisconnects: [
      {
        afterMs: 300,
        clean: false,
        reason: "network interrupted",
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await selectors.nth(2).selectOption("Display 2");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(0) ?? null)
    .toMatchObject({
      protocol: "vnc",
      display: "Display 2",
    });
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("Reconnecting (attempt 1/5)...")).toBeVisible();

  await page.getByRole("button", { name: "Reconnect Now" }).click();

  await expect
    .poll(() => sessionRequests.length)
    .toBe(2);
  await expect(sessionRequests[1]).toMatchObject({
    target: "agent-host-1",
    protocol: "vnc",
    display: "Display 2",
    record: false,
  });
  await expect(page.getByText("Reconnecting (attempt 1/5)...")).toHaveCount(0);
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page.waitForTimeout(1100);
  expect(sessionRequests).toHaveLength(2);
});

test("desktop VNC audio unavailable state clears after reconnect recovery", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    audioBehaviors: [
      {
        state: "unavailable",
        error: "Desktop audio unavailable.",
      },
      {
        state: "started",
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.at(0)?.protocol ?? null)
    .toBe("vnc");
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByTitle("Audio unavailable")).toBeDisabled();
  await expect(page.getByLabel("Volume")).toHaveCount(0);

  await page.getByTitle("Disconnect").click();
  await expect(page.getByRole("button", { name: "Connect" })).toBeVisible();

  await page.getByRole("button", { name: "Connect" }).click();
  await expect.poll(() => sessionRequests.length).toBe(2);
  await expect(page.getByTitle("Audio unavailable")).toHaveCount(0);
  await expect(page.getByTitle("Mute audio")).toBeVisible();
  await expect(page.getByLabel("Volume")).toBeVisible();
});

test("desktop VNC reconnect exhaustion surfaces Try Again and can recover", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    accelerateReconnectTimers: true,
    vncDisconnects: [
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
      {
        afterMs: 20,
        clean: false,
        reason: "upstream reset",
        beforeConnect: true,
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect
    .poll(() => sessionRequests.length)
    .toBe(6);
  await expect(page.getByText("Connection lost", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Unable to reconnect after 5 attempts", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Try Again" })).toBeVisible();

  await page.getByRole("button", { name: "Try Again" }).click();
  await expect
    .poll(() => sessionRequests.length)
    .toBe(7);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByRole("button", { name: "Try Again" })).toHaveCount(0);
});

test("desktop VNC credentials prompt supports retry after auth failure", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["username", "password"],
        outcome: "securityfailure",
        reason: "Authentication failed",
      },
      {
        types: ["username", "password"],
        outcome: "success",
      },
    ],
  });
  await installDesktopApiMocks(page, { sessionRequests });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect(page.getByText("Authentication Required", { exact: true })).toBeVisible();
  await page.getByPlaceholder("Username").fill("operator");
  await page.getByPlaceholder("Password").fill("wrong-password");
  await page.getByRole("button", { name: "Authenticate" }).click();

  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Authentication failed", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          username: "operator",
          password: "wrong-password",
        }),
      },
    ]);

  await page.getByRole("button", { name: "Retry" }).click({ force: true });
  await expect(page.getByText("Authentication Required", { exact: true })).toBeVisible();
  await page.getByPlaceholder("Username").fill("operator");
  await page.getByPlaceholder("Password").fill("correct-password");
  await page.getByRole("button", { name: "Authenticate" }).click();

  await expect.poll(() => sessionRequests.length).toBe(2);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
});

test("desktop VNC password-only challenge auto-submits the session password", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["password"],
        outcome: "success",
      },
    ],
  });
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncPassword: "session-secret",
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect.poll(() => sessionRequests.length).toBe(1);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("VNC Password Required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
    ]);
});

test("desktop VNC password-only auto-submit resets across reconnects", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["password"],
        outcome: "success",
      },
      {
        types: ["password"],
        outcome: "success",
      },
    ],
  });
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncPassword: "session-secret",
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect.poll(() => sessionRequests.length).toBe(1);
  await expect(page.getByTitle("Disconnect")).toBeVisible();

  await page.getByTitle("Disconnect").click();
  await expect(page.getByRole("button", { name: "Connect" })).toBeVisible();

  await page.getByRole("button", { name: "Connect" }).click();

  await expect.poll(() => sessionRequests.length).toBe(2);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect(page.getByText("VNC Password Required", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Authentication Required", { exact: true })).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
    ]);
});

test("desktop VNC stored password falls back to the visible prompt after one failed auto-submit", async ({
  page,
}) => {
  const sessionRequests: Array<Record<string, unknown>> = [];
  await installDesktopBrowserMocks(page, {
    vncCredentialPrompts: [
      {
        types: ["password"],
        outcome: "repeatprompt",
      },
    ],
  });
  await installDesktopApiMocks(page, {
    sessionRequests,
    vncPassword: "session-secret",
  });

  await page.goto("/nodes/agent-host-1?panel=desktop");

  const selectors = page.locator("select");
  await selectors.nth(1).selectOption("vnc");
  await page.getByRole("button", { name: "Connect" }).click();

  await expect.poll(() => sessionRequests.length).toBe(1);
  await expect(page.getByText("VNC Password Required", { exact: true })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
    ]);

  await page.getByPlaceholder("Password").fill("manual-secret");
  await page.getByRole("button", { name: "Authenticate" }).click();

  await expect(page.getByText("VNC Password Required", { exact: true })).toHaveCount(0);
  await expect(page.getByTitle("Disconnect")).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => {
        const win = window as unknown as {
          __desktopAudit: {
            inputMessages: Array<{ label: string; payload: string }>;
          };
        };
        return win.__desktopAudit.inputMessages.filter(
          (entry) => entry.label === "vnc-credentials",
        );
      }),
    )
    .toEqual([
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "session-secret",
        }),
      },
      {
        label: "vnc-credentials",
        payload: JSON.stringify({
          password: "manual-secret",
        }),
      },
    ]);
});
