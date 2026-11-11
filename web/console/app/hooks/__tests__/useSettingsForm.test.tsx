import { NextIntlClientProvider } from "next-intl";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import settings from "../../../messages/de/settings.json";
import { useSettingsForm } from "../useSettingsForm";

// Risk: Settings tells a German reader "Runtime settings saved." or "runtime settings update failed: 500", or a
// translated line hides the Hub's own reason for refusing a value.
let latest: ReturnType<typeof useSettingsForm>;
let root: Root;
let container: HTMLDivElement;

function Probe() {
  latest = useSettingsForm();
  return null;
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const poll = (value: string, source: "ui" | "docker") => ({
  key: "console.poll_interval_seconds", label: "Status refresh", description: "", scope: "console", type: "int",
  env_var: "LABTETHER_POLL_INTERVAL_SECONDS", default_value: "5", env_value: "5", effective_value: value, source,
});

async function mount(write: (url: string) => Promise<Response>) {
  vi.stubGlobal("fetch", vi.fn((url: string, init?: RequestInit) => {
    if (init?.method) return write(String(url));
    if (String(url).endsWith("/retention")) return Promise.resolve(json({ settings: null, presets: [] }));
    return Promise.resolve(json({ settings: [poll("5", "docker")], overrides: {} }));
  }));
  await act(async () => {
    root.render(
      <NextIntlClientProvider locale="de" messages={{ settings }}>
        <Probe />
      </NextIntlClientProvider>,
    );
  });
  await act(async () => latest.setRuntimeDraftValues({ "console.poll_interval_seconds": "8" }));
}

beforeEach(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("settings form lines", () => {
  it("says saved and reset in German, naming the Hub's environment when that sets the value", async () => {
    await mount(async (url) => json({ settings: [url.endsWith("/reset") ? poll("5", "docker") : poll("8", "ui")], overrides: {} }));
    await act(async () => latest.saveRuntimeSettings());
    expect(latest.runtimeSettingsError).toBeNull();
    expect(latest.runtimeSettingsMessage).toBe("Einstellungen gespeichert.");
    await act(async () => latest.resetRuntimeSetting("console.poll_interval_seconds"));
    expect(latest.runtimeSettingsMessage).toBe("Auf den Wert aus der Umgebung des Hubs zurückgesetzt.");
  });

  it("keeps the Hub's reason for a refusal and says the rest in German, with no status code", async () => {
    await mount(async () => json({ error: "console.poll_interval_seconds must be between 2 and 120" }, 400));
    await act(async () => latest.saveRuntimeSettings());
    expect(latest.runtimeSettingsError).toBe("console.poll_interval_seconds must be between 2 and 120");

    await mount(async () => new Response("<html>Bad Gateway</html>", { status: 502 }));
    await act(async () => latest.saveRuntimeSettings());
    expect(latest.runtimeSettingsError).toBe("Die Einstellungen konnten nicht gespeichert werden. Erneut versuchen.");
  });
});
