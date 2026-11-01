"use client";

// Words the PBS views share: task kinds, schedules and times in the viewer's language.

import { useCallback } from "react";
import { useLocale, useTranslations } from "next-intl";
import { whenText } from "../proxmox/diFormat";
import { scheduleWords, taskKind } from "./pbsViewModel";

export function usePBSWords() {
  const t = useTranslations("devices-proxmox.pbs");
  const tk = useTranslations("devices-proxmox.kit");
  const locale = useLocale();

  const when = useCallback((value?: number | string | null) => whenText(value, locale), [locale]);

  /** "Backup", "Garbage collection" … or the worker type as PBS writes it. */
  const kindLabel = useCallback((workerType?: string) => {
    const kind = taskKind(workerType);
    return kind ? t(`kind.${kind}`) : workerType || t("kind.task");
  }, [t]);

  /** "Every day", "Every day at 06:00" or the PBS calendar event as written. */
  const schedule = useCallback((value?: string) => {
    const words = scheduleWords(value);
    if (!words) return t("schedule.none");
    if (words.kind === "every") return t(`schedule.${words.unit}`);
    if (words.kind === "dailyAt") return t("schedule.dailyAt", { time: words.time });
    return words.text;
  }, [t]);

  return { t, tk, locale, when, kindLabel, schedule };
}
