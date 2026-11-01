"use client";

// Home Assistant tab helpers: the manual sync (a collector run, then a status refresh) and the entity metadata words.

import { useCallback, useState } from "react";
import { useTranslations } from "next-intl";
import { startCollectorRun, waitForCollectorRun } from "../../../../../components/AddDeviceModal/collectorSync";
import { formatMetadataLabel, formatMetadataValue } from "../../../../../console/formatters";
import type { Asset } from "../../../../../console/models";
import { useStatusControls } from "../../../../../contexts/StatusContext";
import { sanitizeErrorMessage } from "../../../../../lib/sanitizeErrorMessage";

export type SyncFeedback = { kind: "error" | "warning" | "success"; message: string };

const UNREACHABLE = [
  "connection refused", "no route to host", "network is unreachable", "deadline exceeded", "timed out", "timeout", "failed to fetch",
];

/** Sync now: start the Home Assistant collector, wait for its pass, refresh status and say what happened. */
export function useHomeAssistantSync(asset: Asset) {
  const t = useTranslations("devices-proxmox.ha.sync");
  const { fetchStatus } = useStatusControls();
  const [syncing, setSyncing] = useState(false);
  const [feedback, setFeedback] = useState<SyncFeedback | null>(null);
  const collectorID = asset.metadata?.collector_id?.trim() ?? "";

  const failure = useCallback((raw: string) => {
    const unreachable = t("unreachable");
    const sanitized = sanitizeErrorMessage(raw, unreachable);
    const normalized = sanitized.toLowerCase();
    const reach = UNREACHABLE.some((word) => normalized.includes(word)) || /home assistant api returned 5\d\d/.test(normalized);
    return `${reach || sanitized === unreachable ? unreachable : t("failedWith", { reason: sanitized })} ${t("showingLast")}`;
  }, [t]);

  const sync = useCallback(async () => {
    if (!collectorID) {
      setFeedback({ kind: "error", message: t("noCollector") });
      return;
    }
    setSyncing(true);
    setFeedback(null);
    try {
      const startedAtMs = await startCollectorRun(collectorID);
      const result = await waitForCollectorRun(collectorID, startedAtMs);
      await fetchStatus();
      if (!result.ok) {
        setFeedback({ kind: "error", message: failure(result.error ?? "") });
      } else if (result.warning) {
        const warning = sanitizeErrorMessage(result.warning, t("warningFallback"));
        setFeedback({ kind: "warning", message: t("warning", { warning }) });
      } else {
        setFeedback({ kind: "success", message: t("done") });
      }
    } catch (error) {
      await fetchStatus();
      setFeedback({ kind: "error", message: failure(error instanceof Error ? error.message : "") });
    } finally {
      setSyncing(false);
    }
  }, [collectorID, failure, fetchStatus, t]);

  return { feedback, sync, syncing };
}

/** A metadata value as display text, or null when it is empty. */
export function metadataValue(metadata: Record<string, string> | undefined, key: string): string | null {
  const raw = metadata?.[key];
  if (typeof raw !== "string" || raw.trim() === "") return null;
  return formatMetadataValue(key, raw.trim());
}

/** Every other metadata entry, sorted by key, as [label, value]. */
export function otherMetadata(metadata: Record<string, string> | undefined, shown: Set<string>): Array<[string, string]> {
  if (!metadata) return [];
  return Object.entries(metadata)
    .filter(([key, value]) => value.trim() !== "" && !shown.has(key))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => [formatMetadataLabel(key), formatMetadataValue(key, value)]);
}

/** "23 °C": the state with its unit, or null when the entity has no state. */
export function stateText(metadata: Record<string, string> | undefined): string | null {
  const state = metadataValue(metadata, "state");
  if (!state) return null;
  const unit = metadataValue(metadata, "unit_of_measurement");
  return unit ? `${state} ${unit}` : state;
}

export const UNAVAILABLE_STATES = new Set(["unknown", "unavailable", "offline"]);
