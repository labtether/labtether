/** Unified interface all protocol viewers (VNC, RDP, SPICE, WebRTC) must implement. */
export interface ViewerHandle {
  disconnect(): void;
  sendCtrlAltDel(): void;
  focus(): void;
  sendKey?(keysym: number, down: boolean): void;
  clipboardPasteFrom?(text: string): void;
  requestPointerLock?(): void;
  exitPointerLock?(): void;
  setVolume?(volume: number): void;
}

/** Metrics displayed in the performance HUD overlay. */
export interface ViewerMetrics {
  fps: number | null;
  latencyMs: number | null;
  bitrateKbps: number | null;
  codec: string | null;
  resolution: string | null;
  transport: string;
}

/** Whether the viewer has captured keyboard input. */
export type KeyboardGrabState = "off" | "active" | "unsupported";

/** A toolbar shortcut button that sends a sequence of X11 keysyms. */
export interface ShortcutButton {
  id: string;
  label: string;
  title: string;
  /** X11 keysyms sent as a down/up sequence in order. */
  keysyms: number[];
}

/** Standard remote-desktop shortcut buttons. */
export const REMOTE_SHORTCUTS: readonly ShortcutButton[] = [
  {
    id: "ctrl-alt-del",
    label: "CAD",
    title: "Ctrl+Alt+Del",
    keysyms: [0xffe3, 0xffe9, 0xffff],
  },
  {
    id: "alt-tab",
    label: "Alt+Tab",
    title: "Alt+Tab",
    keysyms: [0xffe9, 0xff09],
  },
  {
    id: "alt-f4",
    label: "Alt+F4",
    title: "Alt+F4",
    keysyms: [0xffe9, 0xffc1],
  },
  {
    id: "super",
    label: "Win",
    title: "Win/Super",
    keysyms: [0xffeb],
  },
  {
    id: "prtsc",
    label: "PrtSc",
    title: "Print Screen",
    keysyms: [0xff61],
  },
] as const;

/** State for exponential-backoff reconnect logic. */
export interface ReconnectState {
  active: boolean;
  attempt: number;
  maxAttempts: number;
  nextRetryMs: number;
}
