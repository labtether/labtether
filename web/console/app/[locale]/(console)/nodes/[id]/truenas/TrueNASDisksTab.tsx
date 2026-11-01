"use client";

// Disks with their SMART health joined by name: model, serial, type and the last SMART test in grey; a word in its
// tone replaces them only on a SMART fault. Size, temperature (amber above 50 °C) and pool. SMART tests sit behind ⋯.

import { useCallback, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePollingTask } from "../../../../../hooks/usePollingTask";
import { DiCols, DiDots, DiList, DiMain, DiNote, DiNum, DiRow } from "../proxmox/DiList";
import { DiRowMenu } from "../proxmox/DiControls";
import { sizeParts, whenText } from "../proxmox/diFormat";
import { normalizeTrueNASSmartResponse, type TrueNASSmartResponse } from "../truenasTabModel";
import { diskRows, failureText, type DiskRow } from "./truenasModel";
import { TnFailure, TnListState } from "./TrueNASParts";
import type { TrueNASDisk } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASDisk } from "./truenasTypes";

/** SMART health of every disk, polled every minute. */
function useTrueNASSmart(assetId: string) {
  const [smart, setSmart] = useState<TrueNASSmartResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const task = useCallback(async (signal: AbortSignal) => {
    setError(null);
    try {
      const res = await fetch(`/api/truenas/assets/${encodeURIComponent(assetId)}/smart`, { cache: "no-store", signal });
      const payload = normalizeTrueNASSmartResponse(await res.json().catch(() => null));
      if (!res.ok) throw new Error(payload.error ?? "");
      if (!signal.aborted) setSmart(payload);
    } catch (err) {
      if (signal.aborted) return;
      setError(err instanceof Error ? err.message : "");
      setSmart(null);
    }
  }, [assetId]);
  usePollingTask(task, 60_000);
  return { smart, error };
}

export function TrueNASDisksTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.disks");
  const k = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const { data: disks, loading, error, refresh } = useTrueNASList<TrueNASDisk>(assetId, "disks");
  const { smart, error: smartError } = useTrueNASSmart(assetId);
  const action = useTrueNASAction();
  const rows = useMemo(() => diskRows(disks, smart?.disks ?? []), [disks, smart]);

  const test = (row: DiskRow, type: "short" | "long") => {
    void action.run(row.name, () => truenasAction(
      `/api/truenas/assets/${encodeURIComponent(assetId)}/disks/${encodeURIComponent(row.name)}/smart-test`,
      "POST",
      { type },
    ), refresh);
  };

  const grey = (row: DiskRow) => {
    const last = row.lastTest?.at ? t("lastTest", { when: whenText(row.lastTest.at, locale) }) : null;
    return <DiDots parts={[row.model, row.serial ? <span className="di-mono">{row.serial}</span> : null, row.type, last]} />;
  };

  const fault = (row: DiskRow) => (row.fault
    ? { tone: row.fault.tone, text: row.fault.text ? t("fault", { status: row.fault.text }) : t(row.fault.tone === "bad" ? "failing" : "warning") }
    : null);

  return (
    <>
      {smartError ? <DiNote tone="warn">{t("smartFailed", { reason: failureText(smartError, t("noReason")) })}</DiNote> : null}
      {(smart?.warnings ?? []).map((warning) => <DiNote key={warning} tone="warn">{warning}</DiNote>)}
      <TnFailure message={action.error ? t("testFailed", { disk: action.error.key, reason: failureText(action.error.message, t("noReason")) }) : null} />
      <DiList
        hang
        label={t("label")}
        cols="minmax(0, 1fr) 72px 64px 96px 28px"
        colsM="minmax(0, 1fr) 72px 64px 28px"
        colsS="minmax(0, 1fr) 64px 28px"
      >
        <DiCols>
          <span>{t("colDisk")}</span>
          <span className="r di-s">{t("colSize")}</span>
          <span className="r">{t("colTemp")}</span>
          <span className="di-m">{t("colPool")}</span>
          <span />
        </DiCols>
        <TnListState loading={loading} error={error} count={rows.length} empty={t("empty")} onRetry={refresh} failed={(reason) => t("loadFailed", { reason })} />
        {rows.map((row) => {
          const size = sizeParts(row.size);
          const temp = row.temperature;
          return (
            <DiRow key={row.name} tone={row.fault?.tone ?? ""}>
              <DiMain name={row.name} grey={grey(row)} problem={fault(row)} />
              <DiNum value={size?.value} unit={size?.unit} className="di-s" />
              <DiNum value={typeof temp === "number" ? Number(temp.toFixed(1)) : undefined} unit="°C" hot={typeof temp === "number" && temp > 50} />
              <span className="di-tn-cell di-m">{row.pool ?? "—"}</span>
              <DiRowMenu
                label={k("actionsFor", { name: row.name })}
                items={[
                  { label: t("shortTest"), disabled: action.busy !== null, onSelect: () => test(row, "short") },
                  { label: t("longTest"), disabled: action.busy !== null, onSelect: () => test(row, "long") },
                ]}
              />
            </DiRow>
          );
        })}
      </DiList>
    </>
  );
}
