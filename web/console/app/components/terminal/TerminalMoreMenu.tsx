"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronRight, MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
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

/** The workspace's one ⋯ menu: the focused pane's Copy all, Clear and Find, then Split, Snippets and Settings. */
export default function TerminalMoreMenu(props: TerminalMoreMenuProps) {
  const t = useTranslations("terminal.more");
  const { hasShell, layout, paneCount, snippets } = props;
  return (
    <Menu.Root>
      <Menu.Trigger className="tm-icon tm-more-btn" aria-label={t("label")} title={t("label")}>
        <MoreHorizontal aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu tm-menu" align="end" sideOffset={6}>
          <Menu.Item className="lt-menu-item" disabled={!hasShell} onSelect={props.onCopyAll}>{t("copyAll")}</Menu.Item>
          <Menu.Item className="lt-menu-item" disabled={!hasShell} onSelect={props.onClear}>{t("clear")}</Menu.Item>
          <Menu.Item className="lt-menu-item" disabled={!hasShell} onSelect={props.onFind}>
            {t("find")}
            <span className="lt-menu-hint">{shortcut("F", false, true)}</span>
          </Menu.Item>
          <Menu.Item className="lt-menu-item" onSelect={props.onChangeDevice}>{t("changeDevice")}</Menu.Item>
          <Menu.Separator className="lt-menu-sep" />
          <Menu.Sub>
            <Menu.SubTrigger className="lt-menu-item">
              {t("split")}
              <ChevronRight className="tm-sub-chev" aria-hidden="true" />
            </Menu.SubTrigger>
            <Menu.Portal>
              <Menu.SubContent className="lt-menu tm-menu" sideOffset={6}>
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
              </Menu.SubContent>
            </Menu.Portal>
          </Menu.Sub>
          <Menu.Sub>
            <Menu.SubTrigger className="lt-menu-item">
              {t("snippets")}
              <ChevronRight className="tm-sub-chev" aria-hidden="true" />
            </Menu.SubTrigger>
            <Menu.Portal>
              <Menu.SubContent className="lt-menu tm-menu" sideOffset={6}>
                {snippets.map((snippet) => (
                  <Menu.Item
                    key={snippet.id}
                    className="lt-menu-item"
                    title={snippet.command}
                    onSelect={() => props.onSnippet(snippet.command)}
                  >
                    {snippet.name}
                  </Menu.Item>
                ))}
                {snippets.length > 0 ? <Menu.Separator className="lt-menu-sep" /> : null}
                <Menu.Item className="lt-menu-item" onSelect={props.onAllSnippets}>
                  {snippets.length > 0 ? t("allSnippets") : t("noSnippets")}
                  <span className="lt-menu-hint">{shortcut("S", true)}</span>
                </Menu.Item>
              </Menu.SubContent>
            </Menu.Portal>
          </Menu.Sub>
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
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
