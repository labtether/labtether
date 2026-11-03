// Keys the Terminal page keeps for itself while a shell has focus. xterm would otherwise send them to the shell
// (Ctrl+Shift+S as ^S pauses output) and stop them, so the page's own handlers never saw them.
// Plain Ctrl+A and Ctrl+F stay with the shell (start of line, tmux and screen prefixes); on a Mac ⌘A and ⌘F are free.

const isMac = () => typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || "");

/** Ctrl+Shift+B/S/F (send to all, snippets, full screen), Ctrl+Shift+C/V (copy, paste), and ⌘F/⌘A on a Mac. */
export function isConsoleShortcut(event: KeyboardEvent, mac = isMac()): boolean {
  if (event.type !== "keydown" || event.altKey) return false;
  const key = event.key.toLowerCase();
  if (event.ctrlKey && event.shiftKey && !event.metaKey) return ["b", "s", "f", "c", "v"].includes(key);
  return mac && event.metaKey && !event.ctrlKey && !event.shiftKey && (key === "f" || key === "a");
}

/** The pane's own action for a key: find and select all (⌘F, ⌘A on a Mac), copy and paste (Ctrl+Shift+C, V). */
export function paneShortcut(event: KeyboardEvent, mac = isMac()): "find" | "selectAll" | "copy" | "paste" | null {
  if (event.altKey) return null;
  const key = event.key.toLowerCase();
  if (event.ctrlKey && event.shiftKey && !event.metaKey) return key === "c" ? "copy" : key === "v" ? "paste" : null;
  if (mac && event.metaKey && !event.ctrlKey && !event.shiftKey) return key === "f" ? "find" : key === "a" ? "selectAll" : null;
  return null;
}

/** xterm's hidden input: typing in the shell, not in a form field. */
export function isShellInput(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.classList.contains("xterm-helper-textarea");
}
