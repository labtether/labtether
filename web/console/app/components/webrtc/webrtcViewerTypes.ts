export interface WebRTCConnectionStats {
  rttMs: number | null;
  packetsLost: number | null;
  bitrateKbps: number | null;
  fps: number | null;
  routeClass: "direct" | "reflexive" | "relay" | null;
}

export interface WebRTCDisplayLayout {
  name: string;
  width: number;
  height: number;
  primary: boolean;
  offset_x: number;
  offset_y: number;
}

export interface WebRTCViewerProps {
  wsUrl: string | null;
  onConnect?: () => void;
  onDisconnect?: (detail: { clean: boolean; reason?: string }) => void;
  scalingMode?: "fit" | "native" | "fill";
  audioEnabled?: boolean;
  volume?: number;
  onStats?: (stats: WebRTCConnectionStats) => void;
  onStream?: (stream: MediaStream | null) => void;
  displayLayout?: WebRTCDisplayLayout[];
}

export interface WebRTCViewerHandle {
  disconnect: () => void;
  sendCtrlAltDel: () => void;
  sendKey: (keysym: number, down: boolean) => void;
  focus: () => void;
  setVolume: (volume: number) => void;
  requestClipboardText: () => Promise<string>;
  writeClipboardText: (text: string) => Promise<void>;
  uploadFile: (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => Promise<void>;
}
