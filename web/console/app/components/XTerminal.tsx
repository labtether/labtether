"use client";

import { useEffect, useRef, useCallback, useImperativeHandle, forwardRef } from "react";
import type { ITheme } from "@xterm/xterm";

export type XTerminalHandle = {
  /** Write data to the terminal display (does NOT send to remote) */
  write: (data: string) => void;
  /** Focus the terminal input */
  focus: () => void;
  /** Fit terminal to container size */
  fit: () => void;
  /** Search forward for query in scrollback */
  searchNext: (query: string, options?: { regex?: boolean; caseSensitive?: boolean }) => boolean;
  /** Search backward for query in scrollback */
  searchPrevious: (query: string, options?: { regex?: boolean; caseSensitive?: boolean }) => boolean;
  /** Clear active search highlighting */
  clearSearch: () => void;
  /** Send data to the remote WebSocket (as if the user typed it) */
  sendData: (data: string) => void;
  /** Copy current terminal selection to clipboard. */
  copySelection: () => Promise<TerminalClipboardActionResult>;
  /** Read clipboard text and send it to the remote terminal. */
  pasteFromClipboard: () => Promise<TerminalClipboardActionResult>;
  /** Select all terminal content. */
  selectAll: () => void;
  /** Clear terminal viewport and scrollback. */
  clearScrollback: () => void;
  /** Whether the terminal currently has a selection. */
  hasSelection: () => boolean;
};

export type TerminalClipboardActionResult =
  | { ok: true }
  | {
      ok: false;
      reason: "no-selection" | "permission-denied" | "unavailable" | "empty" | "disconnected";
    };

type XTerminalProps = {
  /** WebSocket URL to connect to for the SSH session */
  wsUrl: string | null;
  /** Called when the WebSocket connection opens */
  onConnected?: () => void;
  /** Called when the WebSocket connection closes or errors */
  onDisconnected?: (reason?: string) => void;
  /** Called when the WebSocket encounters an error */
  onError?: (message: string) => void;
  /** Called when backend emits a connection status event. */
  onStreamStatus?: (status: TerminalStreamStatusEvent) => void;
  /** Called when backend signals shell readiness or first output arrives. */
  onStreamReady?: (message?: string) => void;
  /** Terminal color theme */
  theme?: ITheme;
  /** CSS font-family string */
  fontFamily?: string;
  /** Font size in pixels */
  fontSize?: number;
  /** Cursor style */
  cursorStyle?: "block" | "underline" | "bar";
  /** Whether cursor blinks */
  cursorBlink?: boolean;
  /** Scrollback buffer size in lines */
  scrollback?: number;
  /** Called when the user types data (for broadcast). Fires with the raw data string sent to the WebSocket. */
  onDataCapture?: (data: string) => void;
};

export type TerminalStreamStatusEvent = {
  lt_event?: string;
  type?: string;
  stage?: string;
  message?: string;
  attempt?: number;
  attempts?: number;
  elapsed_ms?: number;
};

const defaultTheme: ITheme = {
  background: "#080808",
  foreground: "#e0e0e0",
  cursor: "#58a6ff",
  selectionBackground: "rgba(88, 166, 255, 0.3)",
  black: "#080808",
  red: "#ff6b6b",
  green: "#51cf66",
  yellow: "#fcc419",
  blue: "#58a6ff",
  magenta: "#da77f2",
  cyan: "#66d9ef",
  white: "#e0e0e0",
  brightBlack: "#6c7086",
  brightRed: "#ff8787",
  brightGreen: "#69db7c",
  brightYellow: "#ffd43b",
  brightBlue: "#74c7ec",
  brightMagenta: "#e599f7",
  brightCyan: "#89dceb",
  brightWhite: "#ffffff",
};

const XTerminal = forwardRef<XTerminalHandle, XTerminalProps>(function XTerminal(
  {
    wsUrl,
    onConnected,
    onDisconnected,
    onError,
    onStreamStatus,
    onStreamReady,
    theme,
    fontFamily,
    fontSize,
    cursorStyle,
    cursorBlink,
    scrollback,
    onDataCapture,
  },
  ref
) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<import("@xterm/xterm").Terminal | null>(null);
  const fitAddonRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null);
  const searchAddonRef = useRef<import("@xterm/addon-search").SearchAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const disposablesRef = useRef<Array<{ dispose: () => void }>>([]);

  // Store callbacks in refs so the terminal effect never needs them as deps.
  // This prevents the terminal from being torn down on every parent re-render.
  const onConnectedRef = useRef(onConnected);
  const onDisconnectedRef = useRef(onDisconnected);
  const onErrorRef = useRef(onError);
  const onStreamStatusRef = useRef(onStreamStatus);
  const onStreamReadyRef = useRef(onStreamReady);
  const onDataCaptureRef = useRef(onDataCapture);
  onConnectedRef.current = onConnected;
  onDisconnectedRef.current = onDisconnected;
  onErrorRef.current = onError;
  onStreamStatusRef.current = onStreamStatus;
  onStreamReadyRef.current = onStreamReady;
  onDataCaptureRef.current = onDataCapture;

  // Expose imperative handle
  useImperativeHandle(ref, () => ({
    write(data: string) {
      termRef.current?.write(data);
    },
    focus() {
      termRef.current?.focus();
    },
    fit() {
      fitAddonRef.current?.fit();
    },
    searchNext(query: string, options?: { regex?: boolean; caseSensitive?: boolean }) {
      if (!searchAddonRef.current) return false;
      return searchAddonRef.current.findNext(query, {
        regex: options?.regex ?? false,
        caseSensitive: options?.caseSensitive ?? false,
      });
    },
    searchPrevious(query: string, options?: { regex?: boolean; caseSensitive?: boolean }) {
      if (!searchAddonRef.current) return false;
      return searchAddonRef.current.findPrevious(query, {
        regex: options?.regex ?? false,
        caseSensitive: options?.caseSensitive ?? false,
      });
    },
    clearSearch() {
      searchAddonRef.current?.clearDecorations();
    },
    sendData(data: string) {
      const socket = socketRef.current;
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(data);
        onDataCaptureRef.current?.(data);
      }
    },
    async copySelection() {
      const term = termRef.current;
      if (!term || !term.hasSelection()) return { ok: false, reason: "no-selection" };
      return writeClipboardText(term.getSelection());
    },
    async pasteFromClipboard() {
      const clipboardRead = await readClipboardText();
      if (!clipboardRead.ok) return clipboardRead;
      const text = clipboardRead.text;
      if (!text) return { ok: false, reason: "empty" };
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) return { ok: false, reason: "disconnected" };
      socket.send(text);
      onDataCaptureRef.current?.(text);
      return { ok: true };
    },
    selectAll() {
      termRef.current?.selectAll();
      termRef.current?.focus();
    },
    clearScrollback() {
      termRef.current?.clear();
    },
    hasSelection() {
      return termRef.current?.hasSelection() ?? false;
    },
  }));

  // Live-update terminal options when props change (without recreating the terminal)
  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (theme) term.options.theme = theme;
  }, [theme]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (fontFamily !== undefined) term.options.fontFamily = fontFamily;
  }, [fontFamily]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (fontSize !== undefined) term.options.fontSize = fontSize;
    fitAddonRef.current?.fit();
  }, [fontSize]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (cursorStyle !== undefined) term.options.cursorStyle = cursorStyle;
  }, [cursorStyle]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (cursorBlink !== undefined) term.options.cursorBlink = cursorBlink;
  }, [cursorBlink]);

  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (scrollback !== undefined) term.options.scrollback = scrollback;
  }, [scrollback]);

  const cleanup = useCallback(() => {
    const socket = socketRef.current;
    if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onclose = null;
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
        socket.close();
      }
      socketRef.current = null;
    }
    for (const d of disposablesRef.current) {
      d.dispose();
    }
    disposablesRef.current = [];
    if (termRef.current) {
      termRef.current.dispose();
      termRef.current = null;
    }
    fitAddonRef.current = null;
    searchAddonRef.current = null;
  }, []);

  useEffect(() => {
    if (!containerRef.current) return;

    let cancelled = false;

    const init = async () => {
      const { Terminal } = await import("@xterm/xterm");
      const { FitAddon } = await import("@xterm/addon-fit");
      const { WebLinksAddon } = await import("@xterm/addon-web-links");
      const { SearchAddon } = await import("@xterm/addon-search");
      const { Unicode11Addon } = await import("@xterm/addon-unicode11");

      if (cancelled || !containerRef.current) return;

      const term = new Terminal({
        cursorBlink: cursorBlink ?? true,
        cursorStyle: cursorStyle ?? "block",
        fontSize: fontSize ?? 13,
        fontFamily: fontFamily ?? '"JetBrains Mono", "SF Mono", Menlo, Monaco, Consolas, monospace',
        theme: theme ?? defaultTheme,
        scrollback: scrollback ?? 5000,
        convertEol: false,
        allowProposedApi: true,
      });

      const fitAddon = new FitAddon();
      const searchAddon = new SearchAddon();
      const unicode11Addon = new Unicode11Addon();

      term.loadAddon(fitAddon);
      term.loadAddon(new WebLinksAddon());
      term.loadAddon(searchAddon);
      term.loadAddon(unicode11Addon);

      term.unicode.activeVersion = "11";

      term.open(containerRef.current);
      fitAddon.fit();
      term.focus();

      termRef.current = term;
      fitAddonRef.current = fitAddon;
      searchAddonRef.current = searchAddon;

      // Handle resize events
      const resizeObserver = new ResizeObserver(() => {
        if (fitAddonRef.current) {
          try {
            fitAddonRef.current.fit();
          } catch {
            // ignore resize errors during teardown
          }
        }
      });
      resizeObserver.observe(containerRef.current);
      disposablesRef.current.push({ dispose: () => resizeObserver.disconnect() });

      // Connect WebSocket if URL provided
      if (wsUrl) {
        connectWebSocket(term, wsUrl);
      }

      const clickFocus = () => term.focus();
      containerRef.current.addEventListener("mousedown", clickFocus);
      disposablesRef.current.push({
        dispose: () => containerRef.current?.removeEventListener("mousedown", clickFocus),
      });
    };

    const connectWebSocket = (term: import("@xterm/xterm").Terminal, url: string) => {
      const socket = new WebSocket(url);
      socketRef.current = socket;
      let streamReadySignaled = false;

      const markStreamReady = (message?: string) => {
        if (streamReadySignaled) return;
        streamReadySignaled = true;
        onStreamReadyRef.current?.(message);
      };

      socket.binaryType = "arraybuffer";

      socket.onopen = () => {
        if (cancelled) return;
        onConnectedRef.current?.();
        term.focus();

        // Send initial terminal size
        const dims = fitAddonRef.current?.proposeDimensions();
        if (dims) {
          socket.send(JSON.stringify({ type: "resize", cols: dims.cols, rows: dims.rows }));
        }
      };

      socket.onmessage = (event) => {
        if (cancelled) return;
        if (event.data instanceof ArrayBuffer) {
          markStreamReady();
          term.write(new Uint8Array(event.data));
        } else if (typeof event.data === "string") {
          const payload = parseTerminalStreamEvent(event.data);
          if (
            payload
            && payload.lt_event === "terminal"
            && typeof payload.type === "string"
          ) {
            const typeLabel = payload.type.trim().toLowerCase();
            if (typeLabel === "status") {
              onStreamStatusRef.current?.(payload);
              return;
            }
            if (typeLabel === "ready") {
              onStreamStatusRef.current?.(payload);
              markStreamReady(payload.message);
              return;
            }
            if (typeLabel === "error") {
              const message = payload.message?.trim() || "Terminal connection error";
              onStreamStatusRef.current?.(payload);
              onErrorRef.current?.(message);
              return;
            }
          }
          markStreamReady();
          term.write(event.data);
        }
      };

      socket.onerror = () => {
        if (cancelled) return;
        onErrorRef.current?.("Terminal connection error");
      };

      socket.onclose = (event) => {
        if (cancelled) return;
        socketRef.current = null;
        const reason = event.reason || (event.code !== 1000 ? `Connection closed (code ${event.code})` : undefined);
        onDisconnectedRef.current?.(reason);
        term.write("\r\n\x1b[90m--- Session ended ---\x1b[0m\r\n");
      };

      // Forward terminal input to WebSocket
      const dataDisposable = term.onData((data) => {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(data);
          onDataCaptureRef.current?.(data);
        }
      });
      disposablesRef.current.push(dataDisposable);

      // Forward binary input (for special keys)
      const binaryDisposable = term.onBinary((data) => {
        if (socket.readyState === WebSocket.OPEN) {
          const buffer = new Uint8Array(data.length);
          for (let i = 0; i < data.length; i++) {
            buffer[i] = data.charCodeAt(i) & 0xff;
          }
          socket.send(buffer.buffer);
        }
      });
      disposablesRef.current.push(binaryDisposable);

      // Forward resize events to backend
      const resizeDisposable = term.onResize(({ cols, rows }) => {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify({ type: "resize", cols, rows }));
        }
      });
      disposablesRef.current.push(resizeDisposable);
    };

    void init();

    return () => {
      cancelled = true;
      cleanup();
    };
    // Terminal config props (cursorBlink, fontSize, etc.) are intentionally
    // captured at mount time only — they are live-patched by separate effects
    // above. Only wsUrl and cleanup trigger a full teardown/rebuild.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wsUrl, cleanup]);

  return <div ref={containerRef} className="xtermContainer" />;
});

function parseTerminalStreamEvent(payload: string): TerminalStreamStatusEvent | null {
  const trimmed = payload.trim();
  if (!trimmed || trimmed[0] !== "{") {
    return null;
  }
  try {
    const parsed = JSON.parse(trimmed) as TerminalStreamStatusEvent;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export default XTerminal;

function classifyClipboardError(error: unknown): "permission-denied" | "unavailable" {
  if (typeof DOMException !== "undefined" && error instanceof DOMException) {
    if (error.name === "NotAllowedError" || error.name === "SecurityError") {
      return "permission-denied";
    }
  }
  if (error instanceof Error) {
    const message = error.message.toLowerCase();
    if (message.includes("permission") || message.includes("denied") || message.includes("notallowed")) {
      return "permission-denied";
    }
  }
  return "unavailable";
}

async function readClipboardText(): Promise<
  | { ok: true; text: string }
  | { ok: false; reason: "permission-denied" | "unavailable" }
> {
  if (typeof navigator === "undefined" || !navigator.clipboard?.readText) {
    return { ok: false, reason: "unavailable" };
  }
  try {
    return { ok: true, text: await navigator.clipboard.readText() };
  } catch (error) {
    return { ok: false, reason: classifyClipboardError(error) };
  }
}

async function writeClipboardText(text: string): Promise<TerminalClipboardActionResult> {
  if (!text) return { ok: false, reason: "empty" };
  let fallbackReason: "permission-denied" | "unavailable" = "unavailable";
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return { ok: true };
    }
  } catch (error) {
    fallbackReason = classifyClipboardError(error);
  }

  try {
    if (typeof document === "undefined") {
      return { ok: false, reason: fallbackReason };
    }
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "true");
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    textarea.style.pointerEvents = "none";
    document.body.appendChild(textarea);
    textarea.select();
    const copied = document.execCommand("copy");
    document.body.removeChild(textarea);
    return copied ? { ok: true } : { ok: false, reason: fallbackReason };
  } catch {
    return { ok: false, reason: fallbackReason };
  }
}
