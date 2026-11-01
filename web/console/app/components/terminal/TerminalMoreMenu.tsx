"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { TerminalSnippet } from "../../hooks/useTerminalSnippets";

export const LAYOUTS = ["single", "columns", "rows", "grid", "main-side", "main-bottom"] as const;

const MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || "");
/** A shortcut as this computer writes it: ⌘F / ⌃⇧S on a Mac, Ctrl+F / Ctrl+Shift+S elsewhere. */
export const shortcut = (key: string, shift = false, command = false) =>
  MAC ? `${command ? "⌘" : "⌃"}${shift ? "⇧" : ""}${key}` : `Ctrl+${shift ? "Shift+" : ""}${key}`;

type TerminalMoreMenuProps = {
  /** The focused pane has a device (its shell may still be connecting). */
  hasShell: boolean;
  layout: string;
  paneCount: number;
  snippets: TerminalSnippet[];
  broadcastActive: boolean;
  fullscreen: boolean;
  onCopyAll: () => void;
  onClear: () => void;
  onFind: () => void;
  onChangeDevice: () => void;
  onLayout: (layout: string) => void;
  onSnippet: (command: string) => void;
  onAllSnippets: () => void;
  onSettings: () => void;
  onBroadcast: (on: boolean) => void;
  onFullscreen: (on: boolean) => void;
};

type View = "main" | "split" | "snippets";

/**
 * The workspace's one ⋯ menu: the focused pane's Copy all, Clear and Find, then Split…, Snippets… and Settings….
 * Split… and Snippets… show their own list in the same menu (as the mockup does), not a second menu beside it:
 * a menu at the window's edge has no room for one. ← goes back to the first list; Esc closes.
 */
export default function TerminalMoreMenu(props: TerminalMoreMenuProps) {
  const t = useTranslations("terminal.more");
  const { hasShell, layout, paneCount, snippets } = props;
  const [view, setView] = useState<View>("main");
  const contentRef = useRef<HTMLDivElement | null>(null);

  // A new list takes the keyboard focus on its first item.
  useEffect(() => {
    if (view === "main") return;
    contentRef.current?.querySelector<HTMLElement>("[role^='menuitem']:not([data-disabled])")?.focus();
  }, [view]);

  /** Opens a list in place: the menu stays open. */
  const show = (next: View) => (event: Event) => {
    event.preventDefault();
    setView(next);
  };

  const lists: Record<View, ReactNode> = {
    main: (
      <>
        <Menu.Item className="lt-menu-item" disabled={!hasShell} onSelect={props.onCopyAll}>{t("copyAll")}</Menu.Item>
        <Menu.Item className="lt-menu-item" disabled={!hasShell} onSelect={props.onClear}>{t("clear")}</Menu.Item>
        <Menu.Item className="lt-menu-item" disabled={!hasShell} onSelect={props.onFind}>
          {t("find")}
          <span className="lt-menu-hint">{shortcut("F", false, true)}</span>
        </Menu.Item>
        <Menu.Item className="lt-menu-item" onSelect={props.onChangeDevice}>{t("changeDevice")}</Menu.Item>
        <Menu.Separator className="lt-menu-sep" />
        <Menu.Item className="lt-menu-item" onSelect={show("split")}>{t("split")}</Menu.Item>
        <Menu.Item className="lt-menu-item" onSelect={show("snippets")}>{t("snippets")}</Menu.Item>
        <Menu.Item className="lt-menu-item" onSelect={props.onSettings}>{t("settings")}</Menu.Item>
        <Menu.Separator className="lt-menu-sep" />
        {paneCount > 1 ? (
          <Menu.CheckboxItem className="lt-menu-item" checked={props.broadcastActive} onCheckedChange={props.onBroadcast}>
            {t("sendToAll")}
            <span className="lt-menu-hint">{shortcut("B", true)}</span>
            <Menu.ItemIndicator className="tm-tick">
              <Check aria-hidden="true" />
            </Menu.ItemIndicator>
          </Menu.CheckboxItem>
        ) : null}
        <Menu.CheckboxItem className="lt-menu-item" checked={props.fullscreen} onCheckedChange={props.onFullscreen}>
          {t("fullscreen")}
          <span className="lt-menu-hint">{shortcut("F", true)}</span>
          <Menu.ItemIndicator className="tm-tick">
            <Check aria-hidden="true" />
          </Menu.ItemIndicator>
        </Menu.CheckboxItem>
      </>
    ),
    split: (
      <>
        <Menu.Label className="tm-menu-h">{t("layout")}</Menu.Label>
        <Menu.RadioGroup value={layout} onValueChange={props.onLayout}>
          {LAYOUTS.map((id) => (
            <Menu.RadioItem key={id} value={id} className="lt-menu-item">
              {t(`layout_${id}`)}
              <Menu.ItemIndicator className="lt-menu-check">
                <Check aria-hidden="true" />
              </Menu.ItemIndicator>
            </Menu.RadioItem>
          ))}
        </Menu.RadioGroup>
      </>
    ),
    snippets: (
      <>
        <Menu.Label className="tm-menu-h">{t("snippetsTitle")}</Menu.Label>
        {snippets.map((snippet) => (
          <Menu.Item key={snippet.id} className="lt-menu-item" title={snippet.command} onSelect={() => props.onSnippet(snippet.command)}>
            {snippet.name}
          </Menu.Item>
        ))}
        {snippets.length > 0 ? <Menu.Separator className="lt-menu-sep" /> : null}
        <Menu.Item className="lt-menu-item" onSelect={props.onAllSnippets}>
          {snippets.length > 0 ? t("allSnippets") : t("noSnippets")}
          <span className="lt-menu-hint">{shortcut("S", true)}</span>
        </Menu.Item>
      </>
    ),
  };

  return (
    <Menu.Root onOpenChange={(open) => { if (!open) setView("main"); }}>
      <Menu.Trigger className="tm-icon tm-more-btn" aria-label={t("label")} title={t("label")}>
        <MoreHorizontal aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content
          ref={contentRef}
          className="lt-menu tm-menu"
          align="end"
          sideOffset={6}
          aria-label={view === "main" ? t("label") : view === "split" ? t("layout") : t("snippetsTitle")}
          onKeyDown={(event) => {
            if (event.key === "ArrowLeft" && view !== "main") {
              event.preventDefault();
              setView("main");
            }
          }}
        >
          {lists[view]}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
