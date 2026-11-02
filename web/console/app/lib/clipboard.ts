/**
 * Copy text to the clipboard. navigator.clipboard exists only in secure contexts, and the console is often opened
 * over plain HTTP on a LAN address (http://192.0.2.10:3000), so fall back to a hidden text area and the browser's
 * copy command there. Rejects when neither works, so the caller can say the copy failed.
 */
export async function copyText(text: string): Promise<void> {
  if (typeof navigator !== "undefined" && typeof navigator.clipboard?.writeText === "function") {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Refused (permissions, no focus): try the copy command.
    }
  }
  if (!legacyCopy(text)) throw new Error("clipboard unavailable");
}

function legacyCopy(text: string): boolean {
  if (typeof document === "undefined" || typeof document.execCommand !== "function") return false;
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  // Inside an open dialog the text area must sit in the dialog, or its focus trap takes the focus back first.
  const host = active?.closest<HTMLElement>("[role='dialog'], [role='alertdialog']") ?? document.body;
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.setAttribute("aria-hidden", "true");
  Object.assign(area.style, { position: "fixed", top: "0", left: "0", width: "1px", height: "1px", opacity: "0" });
  host.appendChild(area);
  try {
    area.select();
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    area.remove();
    active?.focus({ preventScroll: true });
  }
}
