import type { Dispatch, SetStateAction } from "react";
import { retentionFieldDefs, type RetentionSettingsPayload } from "../../../../console/models";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { SkeletonRow } from "../../../../components/ui/Skeleton";

type RetentionSettingsCardProps = {
  sectionHeadingClassName: string;
  retentionPresets: RetentionSettingsPayload["presets"];
  retentionDraftValues: Record<string, string>;
  setRetentionDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  retentionLoading: boolean;
  retentionSaving: boolean;
  retentionMessage: string | null;
  applyRetentionPreset: (presetID: string) => Promise<void> | void;
  saveRetentionSettings: () => Promise<void> | void;
};

export function RetentionSettingsCard({
  sectionHeadingClassName,
  retentionPresets,
  retentionDraftValues,
  setRetentionDraftValues,
  retentionLoading,
  retentionSaving,
  retentionMessage,
  applyRetentionPreset,
  saveRetentionSettings,
}: RetentionSettingsCardProps) {
  return (
    <Card className="mb-6">
      <h2>Data Cleanup</h2>
      {retentionLoading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}
      <div className="space-y-6">
        <div>
          <p className={sectionHeadingClassName}>Quick Presets</p>
          <div className="flex items-center gap-2 mt-2">
            {retentionPresets.map((preset) => (
              <Button
                key={preset.id}
                disabled={retentionSaving}
                onClick={() => void applyRetentionPreset(preset.id)}
              >
                {preset.name}
              </Button>
            ))}
          </div>
        </div>
        <div>
          <p className={sectionHeadingClassName}>Retention Windows</p>
          <div className="divide-y divide-[var(--line)] mt-2">
            {retentionFieldDefs.map((field) => (
              <div key={field.key} className="flex items-start justify-between gap-4 py-3">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">{field.label}</span>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Input
                    value={retentionDraftValues[field.key] ?? ""}
                    onChange={(event) =>
                      setRetentionDraftValues((current) => ({ ...current, [field.key]: event.target.value }))
                    }
                    placeholder="e.g. 24h, 7d"
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-3 pt-4">
        <Button variant="primary" disabled={retentionSaving} onClick={() => void saveRetentionSettings()}>
          {retentionSaving ? "Saving..." : "Save Cleanup Rules"}
        </Button>
        {retentionMessage ? <span className="text-xs text-[var(--muted)]">{retentionMessage}</span> : null}
      </div>
    </Card>
  );
}
