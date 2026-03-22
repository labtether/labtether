"use client";

import { useMemo, type Dispatch, type SetStateAction } from "react";
import { useTranslations } from "next-intl";
import { runtimeSettingKeys, type RuntimeSettingEntry } from "../../../../console/models";
import { sourceClassName } from "../../../../console/formatters";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";

type ServiceDiscoveryDefaultsCardProps = {
  runtimeSettings: RuntimeSettingEntry[];
  runtimeDraftValues: Record<string, string>;
  setRuntimeDraftValues: Dispatch<SetStateAction<Record<string, string>>>;
  runtimeSettingsLoading: boolean;
  runtimeSettingsSaving: boolean;
  saveRuntimeSettings: (keys?: string[]) => Promise<void> | void;
};

const serviceDiscoveryFields: Array<{
  key: string;
  type: "bool" | "string" | "int";
}> = [
  { key: runtimeSettingKeys.servicesDiscoveryDefaultDockerEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultProxyEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultProxyTraefikEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultProxyCaddyEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultProxyNPMEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultPortScanEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultPortScanIncludeListening, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultPortScanPorts, type: "string" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultLANScanEnabled, type: "bool" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultLANScanCIDRs, type: "string" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultLANScanPorts, type: "string" },
  { key: runtimeSettingKeys.servicesDiscoveryDefaultLANScanMaxHosts, type: "int" },
];

function sourcePillLabel(source: RuntimeSettingEntry["source"]): string {
  switch (source) {
    case "ui":
      return "UI";
    case "docker":
      return "Docker";
    default:
      return "Default";
  }
}

export function ServiceDiscoveryDefaultsCard({
  runtimeSettings,
  runtimeDraftValues,
  setRuntimeDraftValues,
  runtimeSettingsLoading,
  runtimeSettingsSaving,
  saveRuntimeSettings,
}: ServiceDiscoveryDefaultsCardProps) {
  const t = useTranslations("settings");
  const runtimeSettingsByKey = useMemo(() => {
    const out = new Map<string, RuntimeSettingEntry>();
    for (const entry of runtimeSettings) {
      out.set(entry.key, entry);
    }
    return out;
  }, [runtimeSettings]);

  const serviceDiscoveryEntries = useMemo(() => {
    return serviceDiscoveryFields
      .map((field) => ({ field, entry: runtimeSettingsByKey.get(field.key) }))
      .filter((item): item is { field: (typeof serviceDiscoveryFields)[number]; entry: RuntimeSettingEntry } => Boolean(item.entry));
  }, [runtimeSettingsByKey]);

  if (serviceDiscoveryEntries.length === 0) {
    return null;
  }

  return (
    <Card className="mb-6">
      <h2>{t("discoveryDefaults.title")}</h2>
      <p className="text-xs text-[var(--muted)]">
        {t("discoveryDefaults.description")}
      </p>
      <div className="mt-3 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-xs text-[var(--muted)]">
        {t("discoveryDefaults.lanScanWarning")}
      </div>

      <div className="divide-y divide-[var(--line)] mt-3">
        {serviceDiscoveryEntries.map(({ field, entry }) => {
          const draftValue = runtimeDraftValues[entry.key] ?? entry.effective_value;
          return (
            <div key={entry.key} className="grid gap-2 py-3 xl:grid-cols-[minmax(0,1fr)_auto] xl:items-start">
              <div className="min-w-0">
                <p className="text-sm font-medium text-[var(--text)]">{entry.label}</p>
                <p className="text-[11px] text-[var(--muted)]">{entry.description}</p>
                <p className="mt-1 text-[10px] text-[var(--muted)]">Key: {entry.key}</p>
              </div>
              <div className="flex items-center gap-2">
                {field.type === "bool" ? (
                  <Select
                    value={draftValue}
                    onChange={(event) =>
                      setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                    }
                    className="w-28"
                    data-testid={`service-discovery-default-${entry.key}`}
                  >
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </Select>
                ) : field.type === "int" ? (
                  <Input
                    type="number"
                    min={entry.min_int}
                    max={entry.max_int}
                    value={draftValue}
                    onChange={(event) =>
                      setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                    }
                    className="w-32"
                    data-testid={`service-discovery-default-${entry.key}`}
                  />
                ) : (
                  <Input
                    value={draftValue}
                    onChange={(event) =>
                      setRuntimeDraftValues((current) => ({ ...current, [entry.key]: event.target.value }))
                    }
                    className="w-64"
                    data-testid={`service-discovery-default-${entry.key}`}
                  />
                )}
                <span className="shrink-0" data-tooltip={`Source: ${sourcePillLabel(entry.source)}`}>
                  <Badge status={sourceClassName(entry.source) || "disabled"} size="sm" />
                </span>
              </div>
            </div>
          );
        })}
      </div>

      <div className="flex items-center gap-3 pt-3">
        <Button variant="primary" disabled={runtimeSettingsSaving || runtimeSettingsLoading} onClick={() => void saveRuntimeSettings()}>
          {runtimeSettingsSaving ? t("discoveryDefaults.saving") : t("discoveryDefaults.saveDefaults")}
        </Button>
      </div>
    </Card>
  );
}
