import type { DesktopMockEnvironment } from "./desktopBrowserEnvironment";

// Serialized into the same init script as the shared environment.
export function initDesktopWebRTCMocks({ auditState }: DesktopMockEnvironment) {
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

  (window as unknown as { RTCPeerConnection: unknown }).RTCPeerConnection = MockRTCPeerConnection;
}
