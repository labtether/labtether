"use client";

import { useFormatter, useTranslations } from "next-intl";
import { useCallback, useMemo } from "react";
import { viewerTimeZone } from "../../../lib/viewerTimeZone";
import type { MapDevice, MapGraph } from "./mapModel";

const DAY = 86_400_000;

/**
 * The Map's words for a device's state: a short one under its tile ("Offline", "Not responding"), and the full one
 * with its time ("Offline since 21:12") for the Inspector, titles and screen readers. Healthy devices say nothing short.
 */
export function useMapWords(graph?: MapGraph) {
  const t = useTranslations("topology");
  const format = useFormatter();

  const since = useCallback((iso: string) => {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime()) || d.getTime() <= 0) return "";
    const timeZone = viewerTimeZone();
    return Date.now() - d.getTime() < DAY
      ? format.dateTime(d, { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone })
      : format.dateTime(d, { day: "numeric", month: "short", timeZone });
  }, [format]);

  const short = useCallback((d: MapDevice) => {
    if (d.freshness === "offline") return t("status.offline");
    if (d.freshness === "unresponsive") return t("status.notResponding");
    if (d.freshness === "unknown") return t("status.noCheckIn");
    return "";
  }, [t]);

  const long = useCallback((d: MapDevice) => {
    const time = since(d.asset.last_seen_at);
    if (d.freshness === "offline") return time ? t("status.offlineSince", { time }) : t("status.offline");
    if (d.freshness === "unresponsive") return time ? t("status.notRespondingSince", { time }) : t("status.notResponding");
    if (d.freshness === "unknown") return t("status.noCheckInYet");
    return t("status.online");
  }, [t, since]);

  const zoneNames = useMemo(() => new Map((graph?.zones ?? []).map((z) => [z.id, z.label])), [graph?.zones]);
  /** A zone label key: a zone id, "" (Other) or "lone" (Not connected). */
  const label = useCallback((key: string) => (key === "lone" ? t("lone") : zoneNames.get(key) ?? t("other")), [t, zoneNames]);
  const fold = useCallback((f: number | "less") => (f === "less" ? t("fold.less") : t("fold.more", { count: f })), [t]);

  return { t, short, long, label, fold, since };
}

export type MapWords = ReturnType<typeof useMapWords>;
