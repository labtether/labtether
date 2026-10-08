"use client";

import type { Dispatch, RefObject, SetStateAction } from "react";
import { Ellipsis } from "lucide-react";
import { REMOTE_SHORTCUTS } from "../../types/viewer";
import type { RemoteViewToolbarProps } from "./types";
import { IconButton } from "./ToolbarButtons";
import { ToolbarDisplayPicker } from "./ToolbarDisplayControls";
import {
  ToolbarKeyboardGrab,
  ToolbarPerformance,
  ToolbarVirtualKeyboard,
} from "./ToolbarInputControls";
import {
  ToolbarTransferControls,
  type useToolbarTransferState,
} from "./ToolbarTransferControls";
import {
  moreMenuActionButtonClass,
  moreMenuRowStyle,
  moreMenuSectionStyle,
  moreMenuSectionTitleStyle,
  moreMenuShortcutButtonStyle,
} from "./toolbarStyles";

type MoreMenuProps = Pick<
  RemoteViewToolbarProps,
  | "onSendShortcut"
  | "onCtrlAltDel"
  | "keyboardGrabState"
  | "onKeyboardGrabToggle"
  | "onPerformanceOverlayToggle"
  | "showPerformanceOverlay"
  | "onClipboardPull"
  | "onClipboardPush"
  | "clipboardSyncing"
  | "onFileDrawerToggle"
  | "fileDrawerOpen"
  | "fileDownloading"
  | "onDownloadFile"
  | "isTouchDevice"
  | "onToggleVirtualKeyboard"
  | "displays"
  | "selectedDisplay"
  | "onDisplayChange"
> & {
  menuRef: RefObject<HTMLDivElement | null>;
  open: boolean;
  setOpen: Dispatch<SetStateAction<boolean>>;
  panelStyle: React.CSSProperties;
  transfer: ReturnType<typeof useToolbarTransferState>;
};

const groupStyle = {
  ...moreMenuSectionStyle,
  marginTop: 10,
  paddingTop: 10,
  borderTop: "1px solid rgba(255,255,255,0.08)",
};

export function ToolbarMoreMenu(props: MoreMenuProps) {
  const { menuRef, open, setOpen, panelStyle, transfer } = props;
  const hasKeyboard =
    props.keyboardGrabState &&
    props.keyboardGrabState !== "unsupported" &&
    props.onKeyboardGrabToggle;
  const hasTransfer =
    props.onClipboardPull ||
    props.onClipboardPush ||
    props.onFileDrawerToggle ||
    props.onDownloadFile ||
    (props.isTouchDevice && props.onToggleVirtualKeyboard);
  const hasDisplay =
    props.displays && props.displays.length > 1 && props.onDisplayChange;

  return (
    <div ref={menuRef} style={{ position: "relative", pointerEvents: "auto" }}>
      <IconButton
        onClick={() => setOpen((current) => !current)}
        active={open}
        title="More viewer tools"
      >
        <Ellipsis className="w-3.5 h-3.5" />
      </IconButton>
      {open && (
        <div style={panelStyle}>
          <div style={moreMenuSectionStyle}>
            <span style={moreMenuSectionTitleStyle}>Shortcuts</span>
            <div style={moreMenuRowStyle}>
              {REMOTE_SHORTCUTS.map((shortcut) => (
                <button
                  key={shortcut.id}
                  type="button"
                  onClick={() => {
                    if (props.onSendShortcut)
                      props.onSendShortcut(shortcut.keysyms);
                    else if (shortcut.id === "ctrl-alt-del")
                      props.onCtrlAltDel();
                    setOpen(false);
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
          {(hasKeyboard || props.onPerformanceOverlayToggle) && (
            <div style={groupStyle}>
              <span style={moreMenuSectionTitleStyle}>Viewer</span>
              <div style={moreMenuRowStyle}>
                <ToolbarKeyboardGrab {...props} />
                <ToolbarPerformance {...props} />
              </div>
            </div>
          )}
          {hasTransfer && (
            <div style={groupStyle}>
              <span style={moreMenuSectionTitleStyle}>Transfer</span>
              <div style={moreMenuRowStyle}>
                <ToolbarTransferControls {...props} transfer={transfer} />
                <ToolbarVirtualKeyboard {...props} />
              </div>
            </div>
          )}
          {hasDisplay && (
            <div style={groupStyle}>
              <span style={moreMenuSectionTitleStyle}>Display</span>
              <div style={moreMenuRowStyle}>
                <ToolbarDisplayPicker {...props} />
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
