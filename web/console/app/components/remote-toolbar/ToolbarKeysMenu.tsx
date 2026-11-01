"use client";

import { Keyboard } from "lucide-react";
import { useTranslations } from "next-intl";
import { REMOTE_SHORTCUTS } from "../../types/viewer";
import type { RemoteViewToolbarProps } from "./types";
import { ToolCheck, ToolItem, ToolMenu, ToolSep } from "./ToolbarButtons";

type KeysMenuProps = Pick<
  RemoteViewToolbarProps,
  | "onSendShortcut"
  | "onCtrlAltDel"
  | "keyboardGrabState"
  | "onKeyboardGrabToggle"
  | "pointerLocked"
  | "pointerLockSupported"
  | "onPointerLockToggle"
  | "isTouchDevice"
  | "onToggleVirtualKeyboard"
> & { onOpenChange: (open: boolean) => void };

/**
 * Send keys: the key combinations the browser keeps for itself, then how keyboard and mouse reach the remote screen
 * (capture the keyboard, capture the mouse, the on-screen keyboard on touch devices). Every item acts on the remote
 * screen, so focus returns to it.
 */
export function ToolbarKeysMenu(props: KeysMenuProps) {
  const t = useTranslations("desktop.tools");
  const grab = props.keyboardGrabState && props.keyboardGrabState !== "unsupported" && props.onKeyboardGrabToggle;
  const mouse = props.pointerLockSupported ?? true;
  const touch = props.isTouchDevice && props.onToggleVirtualKeyboard;
  return (
    <ToolMenu icon={<Keyboard aria-hidden="true" />} label={t("sendKeys")} keepFocus onOpenChange={props.onOpenChange}>
      {REMOTE_SHORTCUTS.map((shortcut) => (
        <ToolItem
          key={shortcut.id}
          onSelect={() => {
            if (props.onSendShortcut) props.onSendShortcut(shortcut.keysyms);
            else if (shortcut.id === "ctrl-alt-del") props.onCtrlAltDel();
          }}
        >
          {shortcut.title}
        </ToolItem>
      ))}
      {grab || mouse || touch ? <ToolSep /> : null}
      {grab ? (
        <ToolCheck
          checked={props.keyboardGrabState === "active"}
          onChange={props.onKeyboardGrabToggle!}
          hint={props.keyboardGrabState === "active" ? t("releaseKeys") : undefined}
        >
          {t("captureKeyboard")}
        </ToolCheck>
      ) : null}
      {mouse ? (
        <ToolCheck checked={props.pointerLocked} onChange={props.onPointerLockToggle}>
          {t("captureMouse")}
        </ToolCheck>
      ) : null}
      {touch ? <ToolItem onSelect={props.onToggleVirtualKeyboard!}>{t("touchKeyboard")}</ToolItem> : null}
    </ToolMenu>
  );
}
