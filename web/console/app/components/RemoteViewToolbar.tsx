"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  BarChart3,
  Camera,
  ChevronsDown,
  ChevronsUp,
  Circle,
  ClipboardCopy,
  ClipboardPaste,
  Clock3,
  Download,
  Ellipsis,
  Eye,
  EyeOff,
  FolderOpen,
  Keyboard,
  Loader2,
  Maximize,
  Minimize,
  Monitor,
  MousePointer,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import { REMOTE_SHORTCUTS } from "../types/viewer";
import type { KeyboardGrabState } from "../types/viewer";

// ── Types ──

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

// ── Helpers ──

function latencyColor(ms: number | null): string {
  if (ms === null) return "var(--muted)";
  if (ms < 50) return "var(--ok)";
  if (ms <= 150) return "var(--warn)";
  return "var(--bad)";
}

function networkQualityColor(
  quality: "good" | "fair" | "poor" | null | undefined,
): string {
  if (quality === "good") return "var(--ok)";
  if (quality === "fair") return "var(--warn)";
  if (quality === "poor") return "var(--bad)";
  return "rgba(255,255,255,0.25)";
}

const AUTOHIDE_DELAY = 5000;
const HOT_ZONE_PX = 64;

// ── Shared inline-style fragments ──

const toolbarBg: React.CSSProperties = {
  background: "linear-gradient(180deg, rgba(15, 15, 22, 0.82) 0%, rgba(8, 8, 14, 0.90) 100%)",
  backdropFilter: "blur(20px) saturate(1.4)",
  WebkitBackdropFilter: "blur(20px) saturate(1.4)",
  border: "1px solid rgba(255, 255, 255, 0.07)",
};

const dividerStyle: React.CSSProperties = {
  width: 1,
  alignSelf: "stretch",
  background: "rgba(255, 255, 255, 0.06)",
  margin: "6px 2px",
};

// ── Sub-components ──

function SegmentGroup({
  options,
  value,
  onChange,
  label,
}: {
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
  label: string;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex items-center rounded-md overflow-hidden border border-white/10"
    >
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            className={`px-2 py-0.5 font-medium transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)] ${
              active
                ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                : "text-white/50 hover:text-white/80 hover:bg-white/5"
            }`}
            style={{ fontSize: 11 }}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

function IconButton({
  onClick,
  active,
  disabled,
  danger,
  title,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  danger?: boolean;
  title: string;
  children: React.ReactNode;
}) {
  const base =
    "flex items-center justify-center w-7 h-7 rounded-lg transition-all duration-150 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]";
  const variant = disabled
    ? "text-white/20 cursor-not-allowed"
    : danger
      ? "text-red-400 hover:text-red-300 hover:bg-red-500/15"
      : active
        ? "bg-white/12 text-white/95 shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]"
        : "text-white/45 hover:text-white/85 hover:bg-white/8";
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-label={title}
      disabled={disabled}
      className={`${base} ${variant}`}
    >
      {children}
    </button>
  );
}

function LabeledIconButton({
  onClick,
  active,
  disabled,
  title,
  label,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  title: string;
  label: string;
  children: React.ReactNode;
}) {
  const base =
    "flex items-center gap-1.5 h-7 rounded-md px-2 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]";
  const variant = disabled
    ? "text-white/25 bg-white/5 cursor-not-allowed"
    : active
      ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
      : "text-white/65 hover:text-white/90 hover:bg-white/8";
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-label={title}
      disabled={disabled}
      className={`${base} ${variant}`}
    >
      <span className="shrink-0">{children}</span>
      <span className="text-[10px] font-semibold uppercase tracking-[0.14em]">
        {label}
      </span>
    </button>
  );
}

// ── Main component ──

export function RemoteViewToolbar({
  layout = "overlay",
  connectionState,
  latencyMs,
  transportLabel,
  networkQuality,
  protocol,
  quality,
  onQualityChange,
  scalingMode,
  onScalingModeChange,
  pointerLocked,
  pointerLockSupported = true,
  onPointerLockToggle,
  viewOnly,
  onViewOnlyToggle,
  recording = false,
  onToggleRecording,
  onScreenshot,
  audioMuted = false,
  onAudioToggle,
  audioUnavailable = false,
  volume = 1,
  onVolumeChange,
  isFullscreen,
  onFullscreenToggle,
  onCtrlAltDel,
  onDisconnect,
  keyboardGrabState,
  onKeyboardGrabToggle,
  onSendShortcut,
  showPerformanceOverlay = false,
  onPerformanceOverlayToggle,
  onToggleVirtualKeyboard,
  isTouchDevice = false,
  clipboardSyncing = false,
  clipboardLastSync = "idle",
  onClipboardPull,
  onClipboardPush,
  onDownloadFile,
  fileDownloading = false,
  fileDrawerOpen = false,
  onFileDrawerToggle,
  displays,
  selectedDisplay,
  onDisplayChange,
}: RemoteViewToolbarProps) {
  const isOverlayLayout = layout === "overlay";
  const [visible, setVisible] = useState(true);
  const [overlayPosition, setOverlayPosition] = useState<"top" | "bottom">(
    "bottom",
  );
  const [autoHideEnabled, setAutoHideEnabled] = useState(false);
  const [clipboardFlash, setClipboardFlash] = useState<
    "success" | "error" | null
  >(null);
  const [showMoreMenu, setShowMoreMenu] = useState(false);
  const [showDownloadInput, setShowDownloadInput] = useState(false);
  const [downloadPath, setDownloadPath] = useState("");
  const downloadInputRef = useRef<HTMLInputElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clipboardFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const toolbarRef = useRef<HTMLDivElement>(null);
  const moreMenuRef = useRef<HTMLDivElement>(null);
  const visibleRef = useRef(visible);
  visibleRef.current = visible;

  // Flash clipboard status on sync result change
  const prevLastSync = useRef(clipboardLastSync);
  useEffect(() => {
    if (
      clipboardLastSync !== prevLastSync.current &&
      clipboardLastSync !== "idle"
    ) {
      setClipboardFlash(clipboardLastSync);
      if (clipboardFlashTimerRef.current)
        clearTimeout(clipboardFlashTimerRef.current);
      clipboardFlashTimerRef.current = setTimeout(
        () => setClipboardFlash(null),
        1500,
      );
    }
    prevLastSync.current = clipboardLastSync;
  }, [clipboardLastSync]);

  // Auto-hide timer management
  const resetTimer = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    setVisible(true);
    if (isOverlayLayout && autoHideEnabled) {
      timerRef.current = setTimeout(() => setVisible(false), AUTOHIDE_DELAY);
    }
  }, [autoHideEnabled, isOverlayLayout]);

  // Start timer on mount / when overlay behavior changes
  useEffect(() => {
    if (!isOverlayLayout) {
      setVisible(true);
      return;
    }
    if (!autoHideEnabled) {
      if (timerRef.current) clearTimeout(timerRef.current);
      setVisible(true);
      return;
    }
    resetTimer();
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [autoHideEnabled, isOverlayLayout, resetTimer]);

  // Track mouse movement in container (parent)
  useEffect(() => {
    if (!isOverlayLayout) return;
    const container = toolbarRef.current?.parentElement;
    if (!container) return;

    const handleMouseMove = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect();
      const relativeY = e.clientY - rect.top;
      const inRevealZone =
        overlayPosition === "top"
          ? relativeY <= HOT_ZONE_PX
          : relativeY >= rect.height - HOT_ZONE_PX;

      if (!visibleRef.current && autoHideEnabled && inRevealZone) {
        resetTimer();
      } else if (visibleRef.current && autoHideEnabled) {
        resetTimer();
      }
    };

    container.addEventListener("mousemove", handleMouseMove);
    return () => container.removeEventListener("mousemove", handleMouseMove);
  }, [autoHideEnabled, isOverlayLayout, overlayPosition, resetTimer]);

  useEffect(() => {
    if (!showMoreMenu) return;

    const handlePointerDown = (event: MouseEvent) => {
      if (!moreMenuRef.current?.contains(event.target as Node)) {
        setShowMoreMenu(false);
      }
    };

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setShowMoreMenu(false);
      }
    };

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleEscape);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleEscape);
    };
  }, [showMoreMenu]);

  // Only render when connected
  if (connectionState !== "connected") return null;

  const dotColor = latencyColor(latencyMs);
  const statusGroup = (
    <div
      className="flex items-center gap-1.5"
      style={isOverlayLayout ? { pointerEvents: "none" } : undefined}
    >
      <span
        className="rounded-full"
        style={{
          width: 6,
          height: 6,
          backgroundColor: dotColor,
          boxShadow: `0 0 6px ${dotColor}`,
          flexShrink: 0,
        }}
      />
      {latencyMs !== null && (
        <span
          style={{
            fontSize: 11,
            color: "rgba(255,255,255,0.8)",
            whiteSpace: "nowrap",
          }}
        >
          {latencyMs}ms
        </span>
      )}
      {!isOverlayLayout && (
        <span
          style={{
            fontSize: 10,
            color: "rgba(255,255,255,0.4)",
            whiteSpace: "nowrap",
          }}
        >
          {transportLabel}
        </span>
      )}
      {!isOverlayLayout && (
        <>
          <span
            className="rounded-sm border border-white/20 px-1 py-[1px] font-semibold uppercase tracking-wide"
            style={{ fontSize: 9, color: "rgba(255,255,255,0.75)" }}
          >
            {protocol}
          </span>
          {networkQuality && (
            <span
              className="rounded-sm border px-1 py-[1px] font-semibold uppercase tracking-wide"
              style={{
                fontSize: 9,
                color: networkQualityColor(networkQuality),
                borderColor: networkQualityColor(networkQuality),
              }}
            >
              {networkQuality}
            </span>
          )}
        </>
      )}
    </div>
  );

  const overlayPositionButton = isOverlayLayout ? (
    <IconButton
      onClick={() => {
        setOverlayPosition((current) => (current === "top" ? "bottom" : "top"));
        setVisible(true);
      }}
      active={overlayPosition === "top"}
      title={
        overlayPosition === "top"
          ? "Move toolbar to bottom"
          : "Move toolbar to top"
      }
    >
      {overlayPosition === "top" ? (
        <ChevronsDown className="w-3.5 h-3.5" />
      ) : (
        <ChevronsUp className="w-3.5 h-3.5" />
      )}
    </IconButton>
  ) : null;

  const autoHideButton = isOverlayLayout ? (
    <IconButton
      onClick={() => {
        setAutoHideEnabled((current) => !current);
        setVisible(true);
      }}
      active={autoHideEnabled}
      title={autoHideEnabled ? "Disable auto-hide" : "Enable auto-hide"}
    >
      <Clock3 className="w-3.5 h-3.5" />
    </IconButton>
  ) : null;

  const qualitySelector = (
    <SegmentGroup
      label="Stream quality"
      options={[
        { value: "low", label: "Low" },
        { value: "medium", label: "Med" },
        { value: "high", label: "High" },
      ]}
      value={quality}
      onChange={onQualityChange}
    />
  );

  const displayPicker =
    displays && displays.length > 1 && onDisplayChange ? (
      <div className="flex items-center gap-1">
        <Monitor className="w-3.5 h-3.5 text-white/50 flex-shrink-0" />
        <select
          aria-label="Select display"
          value={selectedDisplay || ""}
          onChange={(e) => onDisplayChange(e.target.value)}
          className="rounded-md border border-white/10 bg-transparent text-white/80 px-1.5 py-0.5 outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)] cursor-pointer appearance-none"
          style={{
            fontSize: 11,
            backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='rgba(255,255,255,0.5)' stroke-width='2'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`,
            backgroundRepeat: "no-repeat",
            backgroundPosition: "right 4px center",
            paddingRight: 18,
          }}
        >
          {displays.map((d) => (
            <option
              key={d.name}
              value={d.name}
              style={{ background: "#1a1a1a", color: "#fff" }}
            >
              {d.name} ({d.width}x{d.height}){d.primary ? " \u2605" : ""}
            </option>
          ))}
        </select>
      </div>
    ) : null;

  const overlayStatusBadges = (
    <div className="flex items-center gap-1">
      <span
        className="rounded-sm border border-white/15 px-1.5 py-[2px] font-semibold uppercase tracking-wide"
        style={{ fontSize: 9, color: "rgba(255,255,255,0.78)" }}
      >
        {protocol}
      </span>
      <span
        className="rounded-sm border border-white/15 px-1.5 py-[2px] font-semibold uppercase tracking-wide"
        style={{ fontSize: 9, color: "rgba(255,255,255,0.65)" }}
      >
        {quality}
      </span>
      {networkQuality && (
        <span
          className="rounded-sm border px-1.5 py-[2px] font-semibold uppercase tracking-wide"
          style={{
            fontSize: 9,
            color: networkQualityColor(networkQuality),
            borderColor: networkQualityColor(networkQuality),
          }}
        >
          {networkQuality}
        </span>
      )}
    </div>
  );

  const scalingSelector = (
    <SegmentGroup
      label="Scaling mode"
      options={[
        { value: "fit", label: "Fit" },
        { value: "native", label: "1:1" },
        { value: "fill", label: "Fill" },
      ]}
      value={scalingMode}
      onChange={(v) => onScalingModeChange(v as ScalingMode)}
    />
  );

  const pointerLockButton = pointerLockSupported ? (
    <LabeledIconButton
      onClick={onPointerLockToggle}
      active={pointerLocked}
      title={
        pointerLocked
          ? "Mouse ready in remote session"
          : "Send mouse to remote session"
      }
      label="Mouse"
    >
      <MousePointer className="w-3.5 h-3.5" />
    </LabeledIconButton>
  ) : null;

  const viewOnlyButton = (
    <IconButton
      onClick={onViewOnlyToggle}
      active={viewOnly}
      title={viewOnly ? "Enable input" : "View only"}
    >
      {viewOnly ? (
        <EyeOff className="w-3.5 h-3.5" />
      ) : (
        <Eye className="w-3.5 h-3.5" />
      )}
    </IconButton>
  );

  const recordingButton = onToggleRecording ? (
    <IconButton
      onClick={onToggleRecording}
      active={recording}
      title={recording ? "Stop recording" : "Start recording"}
    >
      <Circle className="w-3.5 h-3.5" />
    </IconButton>
  ) : null;

  const screenshotButton = onScreenshot ? (
    <LabeledIconButton
      onClick={onScreenshot}
      title="Take screenshot"
      label="Screenshot"
    >
      <Camera className="w-3.5 h-3.5" />
    </LabeledIconButton>
  ) : null;

  const audioControls =
    onAudioToggle || audioUnavailable ? (
      <>
        <IconButton
          onClick={onAudioToggle ?? (() => {})}
          active={!audioMuted && !audioUnavailable}
          disabled={audioUnavailable}
          title={
            audioUnavailable
              ? "Audio unavailable"
              : audioMuted
                ? "Unmute audio"
                : "Mute audio"
          }
        >
          {audioMuted || audioUnavailable ? (
            <VolumeX className="w-3.5 h-3.5" />
          ) : (
            <Volume2 className="w-3.5 h-3.5" />
          )}
        </IconButton>
        {onVolumeChange && !audioUnavailable && (
          <input
            aria-label="Volume"
            type="range"
            min={0}
            max={1}
            step={0.05}
            value={Math.max(0, Math.min(1, volume))}
            onChange={(event) => onVolumeChange(Number(event.target.value))}
            className="w-20 accent-[var(--accent)]"
            title="Volume"
          />
        )}
      </>
    ) : null;

  const shortcutButtons = (
    <div className="flex items-center gap-0.5">
      {REMOTE_SHORTCUTS.map((shortcut) => (
        <button
          key={shortcut.id}
          type="button"
          onClick={() => {
            if (onSendShortcut) {
              onSendShortcut(shortcut.keysyms);
            } else if (shortcut.id === "ctrl-alt-del") {
              onCtrlAltDel();
            }
          }}
          title={shortcut.title}
          aria-label={shortcut.title}
          className="flex items-center justify-center h-7 rounded-md px-1.5 font-mono font-semibold text-white/60 hover:text-white/90 hover:bg-white/8 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]"
          style={{ fontSize: 10 }}
        >
          {shortcut.label}
        </button>
      ))}
    </div>
  );

  const keyboardGrabButton =
    keyboardGrabState &&
    keyboardGrabState !== "unsupported" &&
    onKeyboardGrabToggle ? (
      <LabeledIconButton
        onClick={onKeyboardGrabToggle}
        active={keyboardGrabState === "active"}
        title={
          keyboardGrabState === "active"
            ? "Release keyboard from remote session (Ctrl+Alt+Shift)"
            : "Send keyboard to remote session"
        }
        label="Keyboard"
      >
        <Keyboard className="w-3.5 h-3.5" />
      </LabeledIconButton>
    ) : null;

  const performanceButton = onPerformanceOverlayToggle ? (
    <IconButton
      onClick={onPerformanceOverlayToggle}
      active={showPerformanceOverlay}
      title="Performance overlay (Ctrl+F1)"
    >
      <BarChart3 className="w-3.5 h-3.5" />
    </IconButton>
  ) : null;

  const clipboardControls =
    onClipboardPull || onClipboardPush ? (
      <div
        className="flex items-center gap-0.5 rounded-md transition-[box-shadow,border-color,opacity] duration-300"
        style={{
          boxShadow:
            clipboardFlash === "success"
              ? "0 0 6px var(--ok)"
              : clipboardFlash === "error"
                ? "0 0 6px var(--bad)"
                : "none",
          border:
            clipboardFlash === "success"
              ? "1px solid var(--ok)"
              : clipboardFlash === "error"
                ? "1px solid var(--bad)"
                : "1px solid transparent",
          opacity: clipboardSyncing ? 0.6 : 1,
          animation: clipboardSyncing
            ? "pulse 1.5s ease-in-out infinite"
            : "none",
        }}
      >
        {onClipboardPull && (
          <IconButton
            onClick={onClipboardPull}
            title="Pull clipboard from remote"
          >
            <ClipboardPaste className="w-3.5 h-3.5" />
          </IconButton>
        )}
        {onClipboardPush && (
          <IconButton
            onClick={onClipboardPush}
            title="Push clipboard to remote"
          >
            <ClipboardCopy className="w-3.5 h-3.5" />
          </IconButton>
        )}
      </div>
    ) : null;

  const downloadControls = onFileDrawerToggle ? (
    <IconButton
      onClick={onFileDrawerToggle}
      active={fileDrawerOpen}
      title={fileDrawerOpen ? "Close file browser" : "Browse & transfer files"}
    >
      {fileDownloading ? (
        <Loader2 className="w-3.5 h-3.5 animate-spin" />
      ) : (
        <FolderOpen className="w-3.5 h-3.5" />
      )}
    </IconButton>
  ) : onDownloadFile ? (
    <>
      <IconButton
        onClick={() => {
          setShowDownloadInput((prev) => !prev);
          if (!showDownloadInput) {
            setTimeout(() => downloadInputRef.current?.focus(), 0);
          }
        }}
        active={showDownloadInput}
        title="Download file from remote"
      >
        {fileDownloading ? (
          <Loader2 className="w-3.5 h-3.5 animate-spin" />
        ) : (
          <Download className="w-3.5 h-3.5" />
        )}
      </IconButton>
      {showDownloadInput && (
        <input
          ref={downloadInputRef}
          type="text"
          placeholder="/path/to/file"
          value={downloadPath}
          onChange={(e) => setDownloadPath(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && downloadPath.trim()) {
              onDownloadFile(downloadPath.trim());
              setDownloadPath("");
              setShowDownloadInput(false);
            } else if (e.key === "Escape") {
              setShowDownloadInput(false);
              setDownloadPath("");
            }
            e.stopPropagation();
          }}
          onBlur={() => {
            setShowDownloadInput(false);
            setDownloadPath("");
          }}
          className="rounded-md border border-white/20 bg-white/5 text-white/90 placeholder:text-white/30 px-2 py-0.5 outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)]"
          style={{ fontSize: 11, width: 160 }}
        />
      )}
    </>
  ) : null;

  const virtualKeyboardButton =
    isTouchDevice && onToggleVirtualKeyboard ? (
      <button
        type="button"
        onClick={onToggleVirtualKeyboard}
        title="On-screen keyboard"
        aria-label="On-screen keyboard"
        className="flex items-center justify-center h-7 rounded-md px-1.5 font-mono font-semibold text-white/60 hover:text-white/90 hover:bg-white/8 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]"
        style={{ fontSize: 10 }}
      >
        KB
      </button>
    ) : null;

  const fullscreenButton = (
    <IconButton
      onClick={onFullscreenToggle}
      title={isFullscreen ? "Exit fullscreen" : "Fullscreen"}
    >
      {isFullscreen ? (
        <Minimize className="w-3.5 h-3.5" />
      ) : (
        <Maximize className="w-3.5 h-3.5" />
      )}
    </IconButton>
  );

  const disconnectButton = (
    <IconButton onClick={onDisconnect} danger title="Disconnect">
      <X className="w-3.5 h-3.5" />
    </IconButton>
  );

  const dockShellStyle: React.CSSProperties = {
    ...toolbarBg,
    position: "relative",
    zIndex: 20,
    display: "flex",
    flexDirection: "column",
    gap: 10,
    width: "100%",
    padding: "12px 14px",
    borderRadius: 14,
    border: "1px solid rgba(255,255,255,0.08)",
    boxShadow: "0 10px 28px rgba(0,0,0,0.28)",
  };

  const dockHeaderStyle: React.CSSProperties = {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    flexWrap: "wrap",
  };

  const dockTitleStyle: React.CSSProperties = {
    fontSize: 10,
    fontWeight: 700,
    letterSpacing: "0.14em",
    textTransform: "uppercase",
    color: "rgba(255,255,255,0.42)",
  };

  const dockBodyStyle: React.CSSProperties = {
    display: "flex",
    flexWrap: "wrap",
    gap: 8,
    alignItems: "stretch",
  };

  const dockGroupStyle: React.CSSProperties = {
    display: "flex",
    alignItems: "center",
    gap: 6,
    padding: "7px 8px",
    borderRadius: 10,
    border: "1px solid rgba(255,255,255,0.08)",
    background: "rgba(255,255,255,0.04)",
    minHeight: 42,
  };

  const moreMenuPanelStyle: React.CSSProperties = {
    ...toolbarBg,
    position: "absolute",
    right: 0,
    minWidth: 248,
    maxWidth: 320,
    zIndex: 80,
    padding: "10px",
    borderRadius: 12,
    border: "1px solid rgba(255,255,255,0.12)",
    boxShadow: "0 16px 40px rgba(0,0,0,0.35)",
    background: "linear-gradient(180deg, rgba(15, 15, 22, 0.92) 0%, rgba(8, 8, 14, 0.96) 100%)",
    backdropFilter: "blur(20px) saturate(1.4)",
    top: overlayPosition === "top" ? "calc(100% + 8px)" : undefined,
    bottom: overlayPosition === "bottom" ? "calc(100% + 8px)" : undefined,
  };

  const moreMenuSectionStyle: React.CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: 8,
  };

  const moreMenuSectionTitleStyle: React.CSSProperties = {
    fontSize: 10,
    fontWeight: 700,
    letterSpacing: "0.12em",
    textTransform: "uppercase",
    color: "rgba(255,255,255,0.42)",
  };

  const moreMenuRowStyle: React.CSSProperties = {
    display: "flex",
    flexWrap: "wrap",
    gap: 6,
    alignItems: "center",
  };

  const moreMenuShortcutButtonStyle = {
    fontSize: 10,
  } satisfies React.CSSProperties;

  const moreMenuActionButtonClass =
    "flex items-center justify-center h-7 rounded-md px-2 font-mono font-semibold text-white/60 hover:text-white/90 hover:bg-white/8 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]";

  const hasMoreControls = Boolean(
    shortcutButtons ||
      keyboardGrabButton ||
      performanceButton ||
      clipboardControls ||
      downloadControls ||
      virtualKeyboardButton,
  );

  if (!isOverlayLayout) {
    return (
      <div
        ref={toolbarRef}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 3,
          padding: "5px 12px",
          background: "linear-gradient(180deg, rgba(15, 15, 22, 0.85) 0%, rgba(8, 8, 14, 0.92) 100%)",
          backdropFilter: "blur(20px) saturate(1.4)",
          WebkitBackdropFilter: "blur(20px) saturate(1.4)",
          borderTop: "1px solid rgba(255,255,255,0.05)",
          boxShadow: "0 -4px 24px rgba(0,0,0,0.3)",
        }}
      >
        {/* Status */}
        {statusGroup}

        <div style={dividerStyle} />

        {/* Quality */}
        {qualitySelector}

        <div style={dividerStyle} />

        {/* Scaling */}
        {scalingSelector}

        <div style={dividerStyle} />

        {/* Primary actions */}
        {pointerLockButton}
        {viewOnlyButton}
        {recordingButton}
        {screenshotButton}
        {audioControls}

        {/* More menu */}
        {hasMoreControls && (
          <>
            <div style={dividerStyle} />
            <div
              ref={moreMenuRef}
              style={{ position: "relative", pointerEvents: "auto" }}
            >
              <IconButton
                onClick={() => setShowMoreMenu((current) => !current)}
                active={showMoreMenu}
                title="More viewer tools"
              >
                <Ellipsis className="w-3.5 h-3.5" />
              </IconButton>
              {showMoreMenu && (
                <div style={{ ...moreMenuPanelStyle, bottom: "calc(100% + 8px)" }}>
                  <div style={moreMenuSectionStyle}>
                    <span style={moreMenuSectionTitleStyle}>Shortcuts</span>
                    <div style={moreMenuRowStyle}>
                      {REMOTE_SHORTCUTS.map((shortcut) => (
                        <button
                          key={shortcut.id}
                          type="button"
                          onClick={() => {
                            if (onSendShortcut) {
                              onSendShortcut(shortcut.keysyms);
                            } else if (shortcut.id === "ctrl-alt-del") {
                              onCtrlAltDel();
                            }
                            setShowMoreMenu(false);
                          }}
                          title={shortcut.title}
                          aria-label={shortcut.title}
                          className={moreMenuActionButtonClass}
                          style={moreMenuShortcutButtonStyle}
                        >
                          {shortcut.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {(keyboardGrabButton || performanceButton) && (
                    <div style={{ ...moreMenuSectionStyle, marginTop: 10, paddingTop: 10, borderTop: "1px solid rgba(255,255,255,0.08)" }}>
                      <span style={moreMenuSectionTitleStyle}>Viewer</span>
                      <div style={moreMenuRowStyle}>
                        {keyboardGrabButton}
                        {performanceButton}
                      </div>
                    </div>
                  )}

                  {(clipboardControls || downloadControls || virtualKeyboardButton) && (
                    <div style={{ ...moreMenuSectionStyle, marginTop: 10, paddingTop: 10, borderTop: "1px solid rgba(255,255,255,0.08)" }}>
                      <span style={moreMenuSectionTitleStyle}>Transfer</span>
                      <div style={moreMenuRowStyle}>
                        {clipboardControls}
                        {downloadControls}
                        {virtualKeyboardButton}
                      </div>
                    </div>
                  )}

                  {displayPicker && (
                    <div style={{ ...moreMenuSectionStyle, marginTop: 10, paddingTop: 10, borderTop: "1px solid rgba(255,255,255,0.08)" }}>
                      <span style={moreMenuSectionTitleStyle}>Display</span>
                      <div style={moreMenuRowStyle}>{displayPicker}</div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </>
        )}

        <div style={dividerStyle} />

        {fullscreenButton}

        <div style={dividerStyle} />

        {disconnectButton}
      </div>
    );
  }

  const overlayContainerStyle: React.CSSProperties = {
    position: "absolute",
    left: "50%",
    transform: "translateX(-50%)",
    zIndex: 50,
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    pointerEvents: "none",
    top: overlayPosition === "top" ? 0 : undefined,
    bottom: overlayPosition === "bottom" ? 0 : undefined,
  };

  const overlayBodyStyle: React.CSSProperties = {
    ...toolbarBg,
    borderRadius:
      overlayPosition === "top" ? "0 0 10px 10px" : "10px 10px 0 0",
    padding: "5px 12px",
    display: "flex",
    alignItems: "center",
    gap: 3,
    opacity: visible ? 1 : 0,
    visibility: visible ? "visible" : "hidden",
    transform: visible
      ? "translateY(0)"
      : overlayPosition === "top"
        ? "translateY(-100%)"
        : "translateY(100%)",
    transition: "opacity 200ms ease, transform 200ms ease",
    pointerEvents: visible ? "auto" : "none",
    boxShadow: "0 4px 16px rgba(0,0,0,0.5)",
  };

  const overlayHandleStyle: React.CSSProperties = {
    ...toolbarBg,
    minWidth: visible ? 72 : 96,
    height: visible ? 18 : 24,
    borderRadius:
      overlayPosition === "top" ? "0 0 999px 999px" : "999px 999px 0 0",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    cursor: "pointer",
    pointerEvents: "auto",
    opacity: visible ? 0.68 : 0.95,
    transition: "opacity 200ms ease, min-width 200ms ease, height 200ms ease",
    boxShadow: visible ? "none" : "0 6px 20px rgba(0,0,0,0.35)",
    border: "none",
    padding: visible ? "0 10px" : "0 12px",
  };

  const handleLabel = autoHideEnabled ? "Tools" : "Toolbar";

  return (
    <div
      ref={toolbarRef}
      style={overlayContainerStyle}
    >
      {overlayPosition === "bottom" && autoHideEnabled && (
        <button
          type="button"
          onClick={() => {
            resetTimer();
          }}
          onMouseEnter={() => {
            resetTimer();
          }}
          title="Show remote view tools"
          aria-label="Show remote view tools"
          style={overlayHandleStyle}
        >
          <div
            style={{
              width: 24,
              height: 3,
              borderRadius: 2,
              backgroundColor: "rgba(255,255,255,0.35)",
            }}
          />
          <span
            style={{
              fontSize: 10,
              fontWeight: 700,
              letterSpacing: "0.08em",
              textTransform: "uppercase",
              color: "rgba(255,255,255,0.82)",
              whiteSpace: "nowrap",
            }}
          >
            {handleLabel}
          </span>
        </button>
      )}

      <div style={overlayBodyStyle}>
        {statusGroup}
        {overlayStatusBadges}
        {overlayPositionButton}
        {autoHideButton}

        <div style={dividerStyle} />

        {qualitySelector}

        {displayPicker && (
          <>
            <div style={dividerStyle} />
            {displayPicker}
          </>
        )}

        <div style={dividerStyle} />

        {scalingSelector}

        <div style={dividerStyle} />

        {pointerLockButton}
        {viewOnlyButton}
        {recordingButton}
        {screenshotButton}
        {audioControls}
        {hasMoreControls && (
          <>
            <div style={dividerStyle} />
            <div
              ref={moreMenuRef}
              style={{ position: "relative", pointerEvents: "auto" }}
            >
              <IconButton
                onClick={() => setShowMoreMenu((current) => !current)}
                active={showMoreMenu}
                title="More viewer tools"
              >
                <Ellipsis className="w-3.5 h-3.5" />
              </IconButton>
              {showMoreMenu && (
                <div style={moreMenuPanelStyle}>
                  <div style={moreMenuSectionStyle}>
                    <span style={moreMenuSectionTitleStyle}>Shortcuts</span>
                    <div style={moreMenuRowStyle}>
                      {REMOTE_SHORTCUTS.map((shortcut) => (
                        <button
                          key={shortcut.id}
                          type="button"
                          onClick={() => {
                            if (onSendShortcut) {
                              onSendShortcut(shortcut.keysyms);
                            } else if (shortcut.id === "ctrl-alt-del") {
                              onCtrlAltDel();
                            }
                            setShowMoreMenu(false);
                          }}
                          title={shortcut.title}
                          aria-label={shortcut.title}
                          className={moreMenuActionButtonClass}
                          style={moreMenuShortcutButtonStyle}
                        >
                          {shortcut.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {(keyboardGrabButton || performanceButton) && (
                    <div
                      style={{
                        ...moreMenuSectionStyle,
                        marginTop: 10,
                        paddingTop: 10,
                        borderTop: "1px solid rgba(255,255,255,0.08)",
                      }}
                    >
                      <span style={moreMenuSectionTitleStyle}>Viewer</span>
                      <div style={moreMenuRowStyle}>
                        {keyboardGrabButton}
                        {performanceButton}
                      </div>
                    </div>
                  )}

                  {(clipboardControls || downloadControls || virtualKeyboardButton) && (
                    <div
                      style={{
                        ...moreMenuSectionStyle,
                        marginTop: 10,
                        paddingTop: 10,
                        borderTop: "1px solid rgba(255,255,255,0.08)",
                      }}
                    >
                      <span style={moreMenuSectionTitleStyle}>Transfer</span>
                      <div style={moreMenuRowStyle}>
                        {clipboardControls}
                        {downloadControls}
                        {virtualKeyboardButton}
                      </div>
                    </div>
                  )}

                  {displayPicker && (
                    <div
                      style={{
                        ...moreMenuSectionStyle,
                        marginTop: 10,
                        paddingTop: 10,
                        borderTop: "1px solid rgba(255,255,255,0.08)",
                      }}
                    >
                      <span style={moreMenuSectionTitleStyle}>Display</span>
                      <div style={moreMenuRowStyle}>{displayPicker}</div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </>
        )}

        {fullscreenButton}

        <div style={dividerStyle} />

        {disconnectButton}
      </div>

      {overlayPosition === "top" && autoHideEnabled && (
        <button
          type="button"
          onClick={() => {
            resetTimer();
          }}
          onMouseEnter={() => {
            resetTimer();
          }}
          title="Show remote view tools"
          aria-label="Show remote view tools"
          style={overlayHandleStyle}
        >
          <div
            style={{
              width: 24,
              height: 3,
              borderRadius: 2,
              backgroundColor: "rgba(255,255,255,0.35)",
            }}
          />
          <span
            style={{
              fontSize: 10,
              fontWeight: 700,
              letterSpacing: "0.08em",
              textTransform: "uppercase",
              color: "rgba(255,255,255,0.82)",
              whiteSpace: "nowrap",
            }}
          >
            {handleLabel}
          </span>
        </button>
      )}
    </div>
  );
}
