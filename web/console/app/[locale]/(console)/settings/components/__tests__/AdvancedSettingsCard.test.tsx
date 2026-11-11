import { act } from "react";
import { NextIntlClientProvider } from "next-intl";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import deSettings from "../../../../../../messages/de/settings.json";
import type { RuntimeSettingEntry } from "../../../../../console/models";
import { AdvancedSettingsCard } from "../AdvancedSettingsCard";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

function setting(key: string, label: string, description: string): RuntimeSettingEntry {
  return {
    key,
    label,
    description,
    scope: key.split(".")[0],
    type: "int",
    env_var: "",
    default_value: "5",
    effective_value: "5",
    source: "default",
    sensitive: false,
    configured: false,
  };
}

// The Hub sends its labels in English. Risk: Advanced shows every setting in English whatever the console language,
// or, once it translates, shows a blank row for a setting a newer Hub added that the console has no words for.
describe("AdvancedSettingsCard words", () => {
  it("shows the console's German words for a known setting and the Hub's words for an unknown one", async () => {
    const known = setting("console.poll_interval_seconds", "Status refresh", "Seconds between dashboard updates.");
    const unknown = setting("console.future_limit", "Future limit", "A setting from a newer hub.");
    await act(async () => root.render(
      <NextIntlClientProvider locale="de" messages={{ settings: deSettings }}>
        <AdvancedSettingsCard
          runtimeSettingsLoading={false}
          runtimeSettingsError={null}
          runtimeSettingsByScope={[["console", [known, unknown]]]}
          runtimeDraftValues={{}}
          setRuntimeDraftValues={() => undefined}
          runtimeSettingsSaving={false}
          runtimeSettingsMessage={null}
          saveRuntimeSettings={() => undefined}
          resetRuntimeSetting={() => undefined}
        />
      </NextIntlClientProvider>,
    ));
    const show = [...container.querySelectorAll("button")].find((button) => button.textContent === "Anzeigen");
    await act(async () => show?.click());

    const rowText = (key: string) => container.querySelector(`[data-setting-key="${key}"]`)?.textContent ?? "";
    expect(container.querySelector("#st-scope-console")?.textContent).toBe("Konsole");
    expect(rowText(known.key)).toContain("Statusaktualisierung");
    expect(rowText(known.key)).toContain("Sekunden zwischen den Dashboard-Aktualisierungen.");
    expect(rowText(known.key)).not.toContain("Status refresh");
    expect(rowText(unknown.key)).toContain("Future limit");
    expect(rowText(unknown.key)).toContain("A setting from a newer hub.");
  });
});
