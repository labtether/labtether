// The hub stores a person's terminal tabs, not which one is in front. That is kept in this browser, so a reload
// comes back to the tab that was showing.

export const activeTabStorageKey = "labtether.terminal.activeTab";

export function loadActiveTab(): string {
  try {
    return window.localStorage.getItem(activeTabStorageKey) ?? "";
  } catch {
    return "";
  }
}

export function saveActiveTab(tabId: string): void {
  try {
    window.localStorage.setItem(activeTabStorageKey, tabId);
  } catch {
    // Storage is off (private window): a reload shows the first tab.
  }
}

/** A link that opens a device or an address (?target=, ?quickConnect=) picks its own tab. */
export function pageOpensByLink(): boolean {
  const params = new URLSearchParams(window.location.search);
  return Boolean(params.get("target")?.trim() || params.get("quickConnect")?.trim());
}
