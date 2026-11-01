"use client";

import { useEffect, useRef, useCallback } from "react";
import { X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useTheme } from "../../contexts/ThemeContext";
import { labtetherTerminalTheme } from "./terminalLook";

export type ScrollbackViewerProps = {
  persistentSessionId: string;
  title: string;
  onClose: () => void;
};

export default function ScrollbackViewer({ persistentSessionId, title, onClose }: ScrollbackViewerProps) {
  const t = useTranslations("terminal.scrollback");
  const { theme: appTheme } = useTheme();
  const noOutput = t("noOutput");
  const loadFailed = t("loadFailed");
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
        theme: labtetherTerminalTheme(appTheme),
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
            term.write(`\x1b[90m${noOutput}\x1b[0m\r\n`);
          }
        } else {
          let errorMessage = loadFailed;
          try {
            const payload = await response.json() as Record<string, unknown>;
            if (typeof payload?.error === "string" && payload.error) {
              errorMessage = payload.error;
            }
          } catch {
            // ignore parse errors
          }
          term.write(`\x1b[31m${errorMessage}\x1b[0m\r\n`);
        }
      } catch {
        if (!cancelled) {
          term.write(`\x1b[31m${loadFailed}\x1b[0m\r\n`);
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
  }, [persistentSessionId, cleanup, appTheme, noOutput, loadFailed]);

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
    <div className="tm-sb-scrim" role="dialog" aria-modal="true" aria-label={t("aria", { title })}>
      <div className="tm-sb-back" onClick={onClose} aria-hidden="true" />
      <div className="tm-sb">
        <div className="tm-sb-head">
          <span className="tm-nm">{title}</span>
          <span className="tm-sb-note">{t("readOnly")}</span>
          <button type="button" className="tm-icon" onClick={onClose} aria-label={t("close")}>
            <X aria-hidden="true" />
          </button>
        </div>
        <div className="tm-sb-body">
          <div ref={containerRef} className="xtermContainer" />
        </div>
      </div>
    </div>
  );
}
