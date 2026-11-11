import type { useTranslations } from "next-intl";
import type { RuntimeSettingEntry } from "../../../../console/models";

/** The `settings` translator. */
type SettingsT = ReturnType<typeof useTranslations>;

export type RuntimeSettingWords = { label: string; description: string };

/**
 * A runtime setting's label and description in the console's language, from `advanced.fields.<key>`. The Hub sends
 * English words; a setting the console has no words for (one a newer Hub added) keeps the Hub's, so it is never blank.
 */
export function runtimeSettingWords(t: SettingsT, entry: RuntimeSettingEntry): RuntimeSettingWords {
  const path = `advanced.fields.${entry.key}`;
  if (!entry.key || !t.has(`${path}.label`)) return { label: entry.label, description: entry.description };
  return {
    label: t(`${path}.label`),
    description: t.has(`${path}.description`) ? t(`${path}.description`) : "",
  };
}

/** A scope as a sub-heading: its own words when the console has them, else "remote_access" → "Remote access". */
export function runtimeScopeTitle(t: SettingsT, scope: string): string {
  if (scope && t.has(`advanced.scopes.${scope}`)) return t(`advanced.scopes.${scope}`);
  const words = scope.replace(/[_.-]+/gu, " ").trim();
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : scope;
}
