import type { Dispatch, SetStateAction } from "react";
import { sourceClassName } from "../../../../console/formatters";
import type { RuntimeSettingEntry } from "../../../../console/models";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { SkeletonRow } from "../../../../components/ui/Skeleton";

type AdvancedSettingsCardProps = {
  sectionHeadingClassName: string;
  runtimeSettingsLoading: boolean;
  runtimeSettingsError: string | null;
  runtimeSettingsByScope: Array<[string, RuntimeSettingEntry[]]>;
  runtimeDraftValues: Record<string, string>;
  setRuntimeDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  runtimeSettingsSaving: boolean;
  runtimeSettingsMessage: string | null;
  saveRuntimeSettings: (keys?: string[]) => Promise<void> | void;
  resetRuntimeSetting: (key: string) => Promise<void> | void;
};

export function AdvancedSettingsCard({
  sectionHeadingClassName,
  runtimeSettingsLoading,
  runtimeSettingsError,
  runtimeSettingsByScope,
  runtimeDraftValues,
  setRuntimeDraftValues,
  runtimeSettingsSaving,
  runtimeSettingsMessage,
  saveRuntimeSettings,
  resetRuntimeSetting,
}: AdvancedSettingsCardProps) {
  return (
    <Card className="mb-6">
      <h2 data-tooltip="Settings changed here override the defaults shown by the system.">Advanced Settings</h2>
      {runtimeSettingsLoading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}
      {runtimeSettingsError ? <p className="text-xs text-[var(--bad)]">{runtimeSettingsError}</p> : null}
      <div className="space-y-6">
        {runtimeSettingsByScope.map(([scope, entries]) => (
          <div key={scope}>
            <p className={sectionHeadingClassName}>{scope}</p>
            <div className="divide-y divide-[var(--line)] mt-2">
              {entries.map((entry) => {
                const draftValue = runtimeDraftValues[entry.key] ?? entry.effective_value;
                const controlWidthClass =
                  entry.type === "bool" ? "w-full sm:w-28" :
                  entry.type === "enum" ? "w-full sm:w-40" :
                  entry.type === "int" ? "w-full sm:w-24 tabular-nums" :
                  "w-full sm:w-64";

                return (
                  <div key={entry.key} className="grid gap-1.5 py-2 xl:grid-cols-[minmax(0,1fr)_auto] xl:items-start">
                    <div className="flex min-w-0 flex-col gap-1">
                      <span className="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--text)]">
                        {entry.label}
                        <button
                          type="button"
                          className="group relative inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-[var(--line)] text-[10px] text-[var(--muted)] transition-colors hover:border-[var(--text)] hover:text-[var(--text)] cursor-help focus-visible:outline-none focus-visible:border-[var(--text)] focus-visible:text-[var(--text)]"
                          aria-label={`Setting key: ${entry.key}`}
                        >
                          ?
                          <span className="pointer-events-none absolute left-1/2 top-full z-20 mt-1 w-52 max-w-[14rem] -translate-x-1/2 scale-95 rounded-md border border-[var(--line)] bg-[var(--panel)] px-2 py-1.5 text-[10px] text-[var(--muted)] opacity-0 invisible transition-[opacity,transform,visibility] duration-150 group-hover:visible group-hover:opacity-100 group-hover:scale-100">
                            <span className="font-medium text-[var(--text)]">Setting key</span>
                            <span className="block mt-0.5 break-words text-[10px] leading-relaxed">{entry.key}</span>
                          </span>
                        </button>
                      </span>
                      <span className="text-[11px] leading-snug text-[var(--muted)]">{entry.description}</span>
                    </div>
                    <div className="flex flex-none flex-wrap items-center justify-start gap-1.5 xl:justify-end">
                      {entry.type === "bool" ? (
                        <Select
                          className={controlWidthClass}
                          title={`Current value: ${draftValue}`}
                          value={draftValue}
                          onChange={(event) =>
                            setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                          }
                        >
                          <option value="true">true</option>
                          <option value="false">false</option>
                        </Select>
                      ) : entry.type === "enum" ? (
                        <Select
                          className={controlWidthClass}
                          title={`Current value: ${draftValue}`}
                          value={draftValue}
                          onChange={(event) =>
                            setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                          }
                        >
                          {(entry.allowed_values ?? []).map((option) => (
                            <option key={option} value={option}>
                              {option}
                            </option>
                          ))}
                        </Select>
                      ) : entry.type === "int" ? (
                        <Input
                          className={controlWidthClass}
                          title={`Current value: ${draftValue}`}
                          type="number"
                          step={1}
                          min={entry.min_int}
                          max={entry.max_int}
                          value={draftValue}
                          onChange={(event) =>
                            setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                          }
                        />
                      ) : (
                        <Input
                          className={controlWidthClass}
                          title={`Current value: ${draftValue}`}
                          value={draftValue}
                          onChange={(event) =>
                            setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                          }
                        />
                      )}
                      <span
                        className="shrink-0"
                        data-tooltip={entry.source === "ui" ? "Set here in Settings" : entry.source === "docker" ? "Set from system environment" : "System default value"}
                      >
                        <Badge status={sourceClassName(entry.source) || "disabled"} size="sm" />
                      </span>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="shrink-0 whitespace-nowrap px-2.5"
                        title={`Reset ${entry.key} to default`}
                        onClick={() => void resetRuntimeSetting(entry.key)}
                      >
                        Reset
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
      <div className="flex items-center gap-3 pt-4">
        <Button variant="primary" disabled={runtimeSettingsSaving} onClick={() => void saveRuntimeSettings()}>
          {runtimeSettingsSaving ? "Saving..." : "Save Advanced Settings"}
        </Button>
        {runtimeSettingsMessage ? <span className="text-xs text-[var(--muted)]">{runtimeSettingsMessage}</span> : null}
      </div>
    </Card>
  );
}
