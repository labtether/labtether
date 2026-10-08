import type { KeyboardGrabState } from "../../types/viewer";

export type ScalingMode = "fit" | "native" | "fill";

export type RemoteViewToolbarLayout = "overlay" | "dock";

export interface RemoteViewToolbarProps {
  layout?: RemoteViewToolbarLayout;
  connectionState:
    | "idle"
    | "connecting"
    | "authenticating"
    | "connected"
    | "error";
  latencyMs: number | null;
  transportLabel: string;
  networkQuality?: "good" | "fair" | "poor" | null;
  protocol: "vnc" | "rdp" | "spice" | "webrtc";
  quality: string;
  onQualityChange: (q: string) => void;
  scalingMode: ScalingMode;
  onScalingModeChange: (mode: ScalingMode) => void;
  pointerLocked: boolean;
  pointerLockSupported?: boolean;
  onPointerLockToggle: () => void;
  viewOnly: boolean;
  onViewOnlyToggle: () => void;
  recording?: boolean;
  onToggleRecording?: () => void;
  onScreenshot?: () => void;
  audioMuted?: boolean;
  onAudioToggle?: () => void;
  audioUnavailable?: boolean;
  volume?: number;
  onVolumeChange?: (volume: number) => void;
  isFullscreen: boolean;
  onFullscreenToggle: () => void;
  onCtrlAltDel: () => void;
  onDisconnect: () => void;
  keyboardGrabState?: KeyboardGrabState;
  onKeyboardGrabToggle?: () => void;
  onSendShortcut?: (keysyms: number[]) => void;
  showPerformanceOverlay?: boolean;
  onPerformanceOverlayToggle?: () => void;
  onToggleVirtualKeyboard?: () => void;
  isTouchDevice?: boolean;
  clipboardSyncing?: boolean;
  clipboardLastSync?: "idle" | "success" | "error";
  onClipboardPull?: () => void;
  onClipboardPush?: () => void;
  onDownloadFile?: (path: string) => void;
  fileDownloading?: boolean;
  fileDrawerOpen?: boolean;
  onFileDrawerToggle?: () => void;
  displays?: Array<{
    name: string;
    width: number;
    height: number;
    primary: boolean;
  }>;
  selectedDisplay?: string;
  onDisplayChange?: (displayName: string) => void;
}
