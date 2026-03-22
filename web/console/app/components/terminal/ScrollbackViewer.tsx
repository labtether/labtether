"use client";

import { useEffect, useRef, useCallback } from "react";
import { X } from "lucide-react";

export type ScrollbackViewerProps = {
  persistentSessionId: string;
  title: string;
  onClose: () => void;
};

export default function ScrollbackViewer({ persistentSessionId, title, onClose }: ScrollbackViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<import("@xterm/xterm").Terminal | null>(null);
  const fitAddonRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null);
  const disposablesRef = useRef<Array<{ dispose: () => void }>>([]);

  const cleanup = useCallback(() => {
    for (const d of disposablesRef.current) {
      d.dispose();
    }
    disposablesRef.current = [];
    if (termRef.current) {
      termRef.current.dispose();
      termRef.current = null;
    }
    fitAddonRef.current = null;
  }, []);

  useEffect(() => {
    // Capture the DOM node at effect setup time so the cleanup function
    // references a stable value even after the ref changes during teardown.
    const container = containerRef.current;
    if (!container) return;

    let cancelled = false;

    const init = async () => {
      const { Terminal } = await import("@xterm/xterm");
      const { FitAddon } = await import("@xterm/addon-fit");
      const { WebLinksAddon } = await import("@xterm/addon-web-links");
      const { Unicode11Addon } = await import("@xterm/addon-unicode11");

      if (cancelled || !container) return;

      const term = new Terminal({
        cursorBlink: false,
        cursorStyle: "block",
        fontSize: 13,
        fontFamily: '"JetBrains Mono", "SF Mono", Menlo, Monaco, Consolas, monospace',
        theme: {
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
        },
        scrollback: 10000,
        convertEol: false,
        allowProposedApi: true,
        // Read-only: disable user input
        disableStdin: true,
      });

      const fitAddon = new FitAddon();
      const unicode11Addon = new Unicode11Addon();

      term.loadAddon(fitAddon);
      term.loadAddon(new WebLinksAddon());
      term.loadAddon(unicode11Addon);

      term.unicode.activeVersion = "11";

      term.open(container);
      fitAddon.fit();

      termRef.current = term;
      fitAddonRef.current = fitAddon;

      // Handle resize
      const resizeObserver = new ResizeObserver(() => {
        if (fitAddonRef.current) {
          try {
            fitAddonRef.current.fit();
          } catch {
            // ignore resize errors during teardown
          }
        }
      });
      resizeObserver.observe(container);
      disposablesRef.current.push({ dispose: () => resizeObserver.disconnect() });

      // Fetch and write scrollback data
      try {
        const response = await fetch(`/api/terminal/sessions/${encodeURIComponent(persistentSessionId)}/scrollback`);
        if (cancelled) return;

        if (response.ok) {
          const buffer = await response.arrayBuffer();
          if (cancelled) return;
          if (buffer.byteLength > 0) {
            term.write(new Uint8Array(buffer));
          } else {
            term.write("\x1b[90m(no scrollback data available)\x1b[0m\r\n");
          }
        } else {
          let errorMessage = "Failed to load scrollback";
          try {
            const payload = await response.json() as Record<string, unknown>;
            if (typeof payload?.error === "string" && payload.error) {
              errorMessage = payload.error;
            }
          } catch {
            // ignore parse errors
          }
          term.write(`\x1b[31mError: ${errorMessage}\x1b[0m\r\n`);
        }
      } catch (err) {
        if (!cancelled) {
          const message = err instanceof Error ? err.message : "Network error";
          term.write(`\x1b[31mError: ${message}\x1b[0m\r\n`);
        }
      }

      // Scroll to top after writing all data
      if (!cancelled) {
        term.scrollToTop();
      }
    };

    void init();

    return () => {
      cancelled = true;
      // Use the captured `container` variable rather than `containerRef.current`
      // so this cleanup always references the node that was mounted when the
      // effect ran, not whatever the ref might point to at teardown time.
      while (container.firstChild) {
        container.removeChild(container.firstChild);
      }
      cleanup();
    };
  }, [persistentSessionId, cleanup]);

  // Close on Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 60,
        display: "flex",
        flexDirection: "column",
        backgroundColor: "rgba(0, 0, 0, 0.85)",
        backdropFilter: "blur(4px)",
        WebkitBackdropFilter: "blur(4px)",
      }}
      role="dialog"
      aria-modal="true"
      aria-label={`Scrollback viewer: ${title}`}
    >
      {/* Overlay backdrop — click outside closes */}
      <div
        style={{ position: "absolute", inset: 0 }}
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Main panel */}
      <div
        style={{
          position: "relative",
          margin: "auto",
          width: "calc(100% - 3rem)",
          maxWidth: "1400px",
          height: "calc(100vh - 3rem)",
          display: "flex",
          flexDirection: "column",
          backgroundColor: "#080808",
          border: "1px solid rgba(255, 255, 255, 0.1)",
          borderRadius: 8,
          overflow: "hidden",
          boxShadow: "0 24px 80px rgba(0, 0, 0, 0.7)",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header bar */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "8px 12px",
            borderBottom: "1px solid rgba(255, 255, 255, 0.08)",
            backgroundColor: "#0f0f12",
            flexShrink: 0,
            gap: 12,
          }}
        >
          {/* Left: title */}
          <div style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
            <span
              style={{
                fontSize: 13,
                fontWeight: 500,
                color: "rgba(255, 255, 255, 0.85)",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {title}
            </span>
            <span
              style={{
                fontSize: 10,
                fontWeight: 500,
                textTransform: "uppercase",
                letterSpacing: "0.07em",
                color: "rgba(255, 255, 255, 0.4)",
                backgroundColor: "rgba(255, 255, 255, 0.06)",
                border: "1px solid rgba(255, 255, 255, 0.1)",
                borderRadius: 3,
                padding: "1px 5px",
                flexShrink: 0,
              }}
            >
              Read-only archived session
            </span>
          </div>

          {/* Right: close button */}
          <button
            type="button"
            onClick={onClose}
            aria-label="Close scrollback viewer"
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: 28,
              height: 28,
              borderRadius: 4,
              border: "none",
              backgroundColor: "transparent",
              color: "rgba(255, 255, 255, 0.5)",
              cursor: "pointer",
              flexShrink: 0,
              transition: "background-color 0.12s, color 0.12s",
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.backgroundColor = "rgba(255, 255, 255, 0.08)";
              e.currentTarget.style.color = "rgba(255, 255, 255, 0.9)";
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = "transparent";
              e.currentTarget.style.color = "rgba(255, 255, 255, 0.5)";
            }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Terminal area */}
        <div style={{ flex: 1, overflow: "hidden", padding: 4 }}>
          <div ref={containerRef} className="xtermContainer" />
        </div>
      </div>
    </div>
  );
}
