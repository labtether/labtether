// The focused pane's own keys. xterm takes typing through a hidden textarea, and the pane's handler used to skip every
// textarea, so none of these worked while someone typed in the shell.
//
// While typing in the shell, the shell keeps its keys: plain Ctrl+A and Ctrl+F (start of line, vim and less paging,
// tmux and screen prefixes), and the browser pastes into it by itself (Ctrl+Shift+V, ⌘V), which needs no clipboard
// permission and works over plain HTTP. The pane adds what the shell lacks: Ctrl+Shift+C copies the selection, and on
// a Mac ⌘F finds and ⌘A selects all. With the focus elsewhere on the page, Ctrl or ⌘ with F, A, Shift+C and Shift+V
// find, select all, copy and paste, as before.

const isMac = () => typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || "");

export type PaneAction = "find" | "selectAll" | "copy" | "paste";

/** The pane's action for a key press, or null when the key is not the pane's (inShell: the focus is in xterm's input). */
export function paneShortcut(event: KeyboardEvent, inShell: boolean, mac = isMac()): PaneAction | null {
  if (event.altKey || !(event.ctrlKey || event.metaKey)) return null;
  const key = event.key.toLowerCase();
  if (inShell) {
    if (event.shiftKey) return event.ctrlKey && !event.metaKey && key === "c" ? "copy" : null;
    if (!(mac && event.metaKey && !event.ctrlKey)) return null;
    return key === "f" ? "find" : key === "a" ? "selectAll" : null;
  }
  if (event.shiftKey) return key === "c" ? "copy" : key === "v" ? "paste" : null;
  return key === "f" ? "find" : key === "a" ? "selectAll" : null;
}

/** xterm's hidden input: typing in the shell, not in a form field. */
export function isShellInput(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.classList.contains("xterm-helper-textarea");
}
