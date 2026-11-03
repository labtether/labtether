import { describe, expect, it } from "vitest";
import { isConsoleShortcut, isShellInput, paneShortcut } from "../consoleShortcuts";

// Risks: the page's keys go to the shell instead (Ctrl+Shift+S arrives as ^S and pauses output; find, copy and paste
// never open while typing), or the page takes a key the shell needs (Ctrl+A start of line, the tmux prefix).

const key = (k: string, mods: Partial<Record<"ctrlKey" | "shiftKey" | "metaKey" | "altKey", boolean>> = {}) =>
  new KeyboardEvent("keydown", { key: k, ...mods });

describe("terminal page keys", () => {
  it("keeps Ctrl+Shift+B/S/F/C/V and ⌘F/⌘A from the shell, and leaves Ctrl+A, Ctrl+F and Ctrl+C to it", () => {
    for (const k of ["B", "S", "F", "C", "V"]) expect(isConsoleShortcut(key(k, { ctrlKey: true, shiftKey: true }), false)).toBe(true);
    expect(isConsoleShortcut(key("f", { metaKey: true }), true)).toBe(true);
    expect(isConsoleShortcut(key("a", { metaKey: true }), true)).toBe(true);
    for (const k of ["a", "f", "c", "r"]) expect(isConsoleShortcut(key(k, { ctrlKey: true }), false)).toBe(false);
    expect(isConsoleShortcut(key("a", { metaKey: true }), false)).toBe(false);
    expect(isConsoleShortcut(key("x"), true)).toBe(false);
  });

  it("maps the pane's keys and treats xterm's input as the shell, not a form field", () => {
    expect(paneShortcut(key("C", { ctrlKey: true, shiftKey: true }), false)).toBe("copy");
    expect(paneShortcut(key("V", { ctrlKey: true, shiftKey: true }), true)).toBe("paste");
    expect(paneShortcut(key("f", { metaKey: true }), true)).toBe("find");
    expect(paneShortcut(key("a", { ctrlKey: true }), false)).toBeNull();
    const shell = document.createElement("textarea");
    shell.className = "xterm-helper-textarea";
    expect(isShellInput(shell)).toBe(true);
    expect(isShellInput(document.createElement("textarea"))).toBe(false);
  });
});
