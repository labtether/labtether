"use client";

import { Maximize, Radio, TerminalSquare } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useMemo, useRef } from "react";
import { usePaletteRegister, type PaletteProvider } from "../../contexts/PaletteContext";
import type { TerminalWorkspace } from "./useTerminalWorkspace";

type Options = {
  ws: TerminalWorkspace;
  ready: boolean;
  fullscreen: boolean;
  setFullscreen: (update: (on: boolean) => boolean) => void;
  toggleSnippets: () => void;
};

/**
 * The page's shortcuts (Ctrl+Shift+B send to all panes, Ctrl+Shift+S snippets, Ctrl+Shift+F full screen, Esc leaves
 * full screen), its ⌘K commands, and the links other pages use: ?target=<device> opens that device's shell and
 * ?quickConnect=<user@host:port> opens the address form with it.
 */
export function useTerminalPageKeys({ ws, ready, fullscreen, setFullscreen, toggleSnippets }: Options) {
  const t = useTranslations("terminal");
  const { setBroadcastActive, openTarget, setAddressRequest } = ws;
  const { handleCreateTab } = ws.tabUI;

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape" && fullscreen) {
        setFullscreen(() => false);
        return;
      }
      if (!event.ctrlKey || !event.shiftKey) return;
      const key = event.key.toUpperCase();
      if (key === "B") {
        event.preventDefault();
        setBroadcastActive((on) => !on);
      } else if (key === "S") {
        event.preventDefault();
        toggleSnippets();
      } else if (key === "F") {
        event.preventDefault();
        setFullscreen((on) => !on);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [fullscreen, setBroadcastActive, setFullscreen, toggleSnippets]);

  const handledLink = useRef(false);
  useEffect(() => {
    if (!ready || handledLink.current) return;
    handledLink.current = true;
    const params = new URLSearchParams(window.location.search);
    const target = params.get("target")?.trim();
    const address = params.get("quickConnect")?.trim();
    if (!target && !address) return;
    window.history.replaceState(window.history.state, "", window.location.pathname);
    if (target) void openTarget(target);
    else if (address) setAddressRequest({ paneKey: null, initial: address });
  }, [ready, openTarget, setAddressRequest]);

  const provider = useMemo((): PaletteProvider => ({
    id: "terminal-commands",
    group: t("palette.group"),
    priority: 1,
    search(query: string) {
      const items = [
        { id: "term-new-tab", label: t("palette.newTab"), icon: TerminalSquare, action: () => handleCreateTab(), keywords: ["tab", "new", "create"] },
        {
          id: "term-broadcast",
          label: ws.broadcastActive ? t("palette.disableBroadcast") : t("palette.enableBroadcast"),
          icon: Radio,
          action: () => setBroadcastActive((on) => !on),
          keywords: ["broadcast", "sync", "multi", "all panes"],
        },
        {
          id: "term-fullscreen",
          label: fullscreen ? t("palette.exitFullscreen") : t("palette.enterFullscreen"),
          icon: Maximize,
          action: () => setFullscreen((on) => !on),
          keywords: ["fullscreen", "maximize", "expand"],
        },
      ];
      const lower = query.trim().toLowerCase();
      if (!lower) return items;
      return items.filter((item) => item.label.toLowerCase().includes(lower) || item.keywords.some((kw) => kw.includes(lower)));
    },
  }), [fullscreen, handleCreateTab, setBroadcastActive, setFullscreen, t, ws.broadcastActive]);

  usePaletteRegister(provider);
}
