import { describe, expect, it } from "vitest";
import { isShellInput, paneShortcut } from "../consoleShortcuts";

// Risks: the pane's keys do nothing while someone types in the shell (the old handler skipped xterm's textarea), or the
// pane takes a key the shell needs (Ctrl+A start of line and tmux prefix, Ctrl+F paging, the browser's own paste).

const key = (k: string, mods: Partial<Record<"ctrlKey" | "shiftKey" | "metaKey" | "altKey", boolean>> = {}) =>
  new KeyboardEvent("keydown", { key: k, ...mods });
const SHELL = true, PAGE = false, MAC = true, PC = false;

describe("terminal pane keys", () => {
  it("in the shell: copies with Ctrl+Shift+C, finds and selects all with ⌘ on a Mac, and leaves the shell its keys", () => {
    expect(paneShortcut(key("C", { ctrlKey: true, shiftKey: true }), SHELL, PC)).toBe("copy");
    expect(paneShortcut(key("f", { metaKey: true }), SHELL, MAC)).toBe("find");
    expect(paneShortcut(key("a", { metaKey: true }), SHELL, MAC)).toBe("selectAll");
    for (const k of ["a", "f", "c", "r"]) expect(paneShortcut(key(k, { ctrlKey: true }), SHELL, PC)).toBeNull();
    expect(paneShortcut(key("f", { ctrlKey: true }), SHELL, MAC)).toBeNull();
    // The browser's own paste reaches the shell without the clipboard permission, so the pane stays out of it.
    expect(paneShortcut(key("V", { ctrlKey: true, shiftKey: true }), SHELL, PC)).toBeNull();
    expect(paneShortcut(key("v", { metaKey: true }), SHELL, MAC)).toBeNull();
    expect(paneShortcut(key("f", { metaKey: true }), SHELL, PC)).toBeNull();
  });

  it("with the focus elsewhere on the page: Ctrl or ⌘ with F, A, Shift+C and Shift+V, as before", () => {
    expect(paneShortcut(key("f", { ctrlKey: true }), PAGE, PC)).toBe("find");
    expect(paneShortcut(key("a", { ctrlKey: true }), PAGE, PC)).toBe("selectAll");
    expect(paneShortcut(key("C", { ctrlKey: true, shiftKey: true }), PAGE, PC)).toBe("copy");
    expect(paneShortcut(key("V", { metaKey: true, shiftKey: true }), PAGE, MAC)).toBe("paste");
    expect(paneShortcut(key("f"), PAGE, PC)).toBeNull();
    expect(paneShortcut(key("f", { ctrlKey: true, altKey: true }), PAGE, PC)).toBeNull();
  });

  it("treats xterm's input as the shell, not a form field", () => {
    const shell = document.createElement("textarea");
    shell.className = "xterm-helper-textarea";
    expect(isShellInput(shell)).toBe(true);
    expect(isShellInput(document.createElement("textarea"))).toBe(false);
  });
});
