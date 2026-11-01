"use client";

// The TrueNAS system: host, version, CPU, memory, uptime and total storage as key-value lines; services set to start
// on boot that are stopped; then TrueNAS's own alerts, problems first. Polled every 30 s.

import { useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { DiEmpty, DiKV, DiList, DiMain, DiNote, DiRow, DiSection, DiSheet } from "../proxmox/DiList";
import { ofParts, percentOf, sizeText } from "../proxmox/diFormat";
import { failureText, levelTone } from "./truenasModel";
import { useTrueNASOverview } from "./useTrueNASData";

export function TrueNASOverviewTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.system");
  const k = useTranslations("devices-proxmox.kit");
  const { data, loading, error, refresh } = useTrueNASOverview(assetId);

  if (!data) {
    return (
      <DiSheet kind="flush">
        {error ? (
          <DiEmpty
            title={t("loadFailed", { reason: failureText(error, t("noReason")) })}
            action={<Button size="sm" onClick={refresh}>{k("retry")}</Button>}
          />
        ) : (
          <DiEmpty title={loading ? k("loading") : t("empty")} />
        )}
      </DiSheet>
    );
  }

  const total = data.storage_total_bytes ?? 0;
  const used = data.storage_used_bytes ?? 0;
  const of = total > 0 ? ofParts(used, total) : null;
  const pct = percentOf(used, total);
  const services = data.services ?? [];
  const running = services.filter((svc) => svc.running === true).length;
  const stoppedOnBoot = services.filter((svc) => svc.running === false && svc.enabled).map((svc) => svc.name);
  const alerts = [...(data.alerts ?? [])].sort((a, b) => {
    const rank = (level: string, dismissed: boolean) => (dismissed ? 3 : levelTone(level) === "bad" ? 0 : levelTone(level) === "warn" ? 1 : 2);
    return rank(a.level, a.dismissed) - rank(b.level, b.dismissed);
  });

  return (
    <>
      {stoppedOnBoot.length > 0 ? <DiNote tone="warn">{t("stoppedOnBoot", { names: stoppedOnBoot.join(", ") })}</DiNote> : null}
      <DiSection title={t("details")}>
        <DiSheet kind="tight">
          <DiKV
            columns={2}
            items={[
              [t("hostname"), data.hostname],
              [t("version"), data.version],
              [t("cpu"), data.model],
              [t("cores"), data.cpu_cores != null ? String(data.cpu_cores) : ""],
              [t("memory"), sizeText(data.memory_bytes)],
              [t("ecc"), data.ecc_enabled != null ? (data.ecc_enabled ? k("on") : k("off")) : ""],
              [t("uptime"), data.uptime],
              [t("storage"), of && pct != null ? t("storageValue", { of: k("of", of), pct: Math.round(pct) }) : ""],
              [t("services"), services.length > 0 ? t("servicesValue", { running, total: services.length }) : ""],
            ]}
          />
        </DiSheet>
      </DiSection>
      {alerts.length > 0 ? (
        <DiSection title={t("alerts")}>
          <DiList hang label={t("alerts")} cols="minmax(0, 1fr)" colsS="minmax(0, 1fr)">
            {alerts.slice(0, 20).map((alert, index) => (
              <DiRow key={`${alert.message}-${index}`} className="di-tn-wrap" tone={alert.dismissed ? "" : levelTone(alert.level)}>
                <DiMain name={alert.message} plain grey={alert.dismissed ? t("dismissed") : undefined} />
              </DiRow>
            ))}
          </DiList>
        </DiSection>
      ) : null}
    </>
  );
}
