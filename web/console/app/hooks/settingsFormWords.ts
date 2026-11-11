"use client";

import { useTranslations } from "next-intl";
import { useMemo, useRef } from "react";

import type { RuntimeSettingEntry } from "../console/models";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";
import { HubRefusal } from "./connectorWords";

export type SettingsFormWords = {
  runtimeLoadFailed: (err: unknown) => string;
  runtimeSaveFailed: (err: unknown) => string;
  runtimeResetFailed: (err: unknown) => string;
  runtimeSaved: () => string;
  runtimeNoChanges: () => string;
  /** After Reset: the value now comes from the Hub's environment, or it is the default. */
  runtimeReset: (entry: RuntimeSettingEntry | undefined) => string;
  retentionLoadFailed: (err: unknown) => string;
  retentionSaveFailed: (err: unknown) => string;
  retentionSaved: () => string;
  retentionNoSettings: () => string;
  presetApplied: (presetID: string) => string;
  presetFailed: (err: unknown) => string;
};

const hubReason = (err: unknown) => (err instanceof HubRefusal ? err.message : "");

/**
 * The lines Settings shows after runtime and retention settings load, save and reset, in the person's language.
 * A refusal keeps the Hub's own reason; anything else (no network, a reply that is not JSON) shows the console's
 * line. The returned object keeps its identity, so it is safe in hook dependency lists.
 */
export function useSettingsFormWords(): SettingsFormWords {
  const t = useTranslations("settings");
  const tRef = useRef(t);
  tRef.current = t;

  return useMemo<SettingsFormWords>(() => {
    const say = (key: string, values?: Record<string, string>) => tRef.current(key, values);
    const failed = (key: string) => (err: unknown) => sanitizeErrorMessage(hubReason(err), say(key));
    return {
      runtimeLoadFailed: failed("advanced.lines.loadFailed"),
      runtimeSaveFailed: failed("advanced.lines.saveFailed"),
      runtimeResetFailed: failed("advanced.lines.resetFailed"),
      runtimeSaved: () => say("advanced.lines.saved"),
      runtimeNoChanges: () => say("advanced.lines.noChanges"),
      runtimeReset: (entry) => say(entry?.source === "docker" ? "advanced.lines.resetEnv" : "advanced.lines.resetDefault"),
      retentionLoadFailed: failed("retention.lines.loadFailed"),
      retentionSaveFailed: failed("retention.lines.saveFailed"),
      retentionSaved: () => say("retention.lines.saved"),
      retentionNoSettings: () => say("retention.lines.noSettings"),
      presetApplied: (presetID) => {
        const known = `retention.presetNames.${presetID}`;
        return say("retention.lines.presetApplied", { name: presetID && tRef.current.has(known) ? say(known) : presetID });
      },
      presetFailed: failed("retention.lines.presetFailed"),
    };
  }, []);
}
