"use client";

import { useMemo, useState, type Dispatch, type SetStateAction } from "react";
import { useTranslations } from "next-intl";
import { runtimeSettingKeys, type RuntimeSettingEntry } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { RuntimeControl } from "./AdvancedSettingsCard";
import { runtimeSettingWords } from "./runtimeSettingWords";
import { Section, Sheet, SheetFoot } from "./SettingsKit";

type ServiceDiscoveryDefaultsCardProps = {
  runtimeSettings: RuntimeSettingEntry[];
  runtimeDraftValues: Record<string, string>;
  setRuntimeDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  runtimeSettingsLoading: boolean;
  runtimeSettingsSaving: boolean;
  /** The hub's answer to the last runtime save; shown here only after this section saved. */
  runtimeSettingsMessage?: string | null;
  saveRuntimeSettings: (keys?: string[]) => Promise<void> | void;
};

/** The discovery settings new agents start with, in the order an agent applies them. */
const serviceDiscoveryKeys: string[] = [
  runtimeSettingKeys.servicesDiscoveryDefaultDockerEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultProxyEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultProxyTraefikEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultProxyCaddyEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultProxyNPMEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultPortScanEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultPortScanIncludeListening,
  runtimeSettingKeys.servicesDiscoveryDefaultPortScanPorts,
  runtimeSettingKeys.servicesDiscoveryDefaultLANScanEnabled,
  runtimeSettingKeys.servicesDiscoveryDefaultLANScanCIDRs,
  runtimeSettingKeys.servicesDiscoveryDefaultLANScanPorts,
  runtimeSettingKeys.servicesDiscoveryDefaultLANScanMaxHosts,
];

export function ServiceDiscoveryDefaultsCard({
  runtimeSettings,
  runtimeDraftValues,
  setRuntimeDraftValues,
  runtimeSettingsLoading,
  runtimeSettingsSaving,
  runtimeSettingsMessage,
  saveRuntimeSettings,
}: ServiceDiscoveryDefaultsCardProps) {
  const t = useTranslations("settings");
  const [savedHere, setSavedHere] = useState(false);
  const entries = useMemo(() => {
    const byKey = new Map(runtimeSettings.map((entry) => [entry.key, entry]));
    return serviceDiscoveryKeys.map((key) => byKey.get(key)).filter((entry): entry is RuntimeSettingEntry => Boolean(entry));
  }, [runtimeSettings]);

  if (entries.length === 0) return null;

  const draftOf = (entry: RuntimeSettingEntry) => runtimeDraftValues[entry.key] ?? entry.effective_value;
  const dirty = entries.some((entry) => draftOf(entry).trim() !== entry.effective_value);

  return (
    <Section title={t("discoveryDefaults.title")} id="discovery">
      <Sheet>
        {entries.map((entry) => {
          const id = `st-sd-${entry.key.replace(/[^a-z0-9]+/gi, "-")}`;
          const { label, description } = runtimeSettingWords(t, entry);
          return (
            <div key={entry.key} className="st-row two">
              <div className="st-row-l">
                {entry.type === "bool" ? <span className="st-nm">{label}</span> : <label className="st-nm" htmlFor={id}>{label}</label>}
                {description ? <span className="st-g">{description}</span> : null}
              </div>
              <div className="st-row-r">
                {entry.source !== "default" ? <span className="st-g">{t(`advanced.source.${entry.source}`)}</span> : null}
                <RuntimeControl
                  id={id}
                  entry={{ ...entry, label }}
                  value={draftOf(entry)}
                  onChange={(value) => setRuntimeDraftValues((current) => ({ ...current, [entry.key]: value }))}
                  testId={`service-discovery-default-${entry.key}`}
                />
              </div>
            </div>
          );
        })}
        <SheetFoot message={savedHere ? runtimeSettingsMessage : null}>
          <Button
            size="sm"
            disabled={runtimeSettingsSaving || runtimeSettingsLoading || !dirty}
            onClick={() => {
              setSavedHere(true);
              void saveRuntimeSettings(entries.map((entry) => entry.key));
            }}
          >
            {runtimeSettingsSaving ? t("discoveryDefaults.saving") : t("discoveryDefaults.saveDefaults")}
          </Button>
        </SheetFoot>
      </Sheet>
    </Section>
  );
}
