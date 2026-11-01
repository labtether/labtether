"use client";

import { useFormatter } from "next-intl";
import { useCallback } from "react";

/** A time as status text shows it: the 24-hour clock today ("21:12"), else the day ("30 Oct"). "" when unknown. */
export function useTerminalWhen() {
  const format = useFormatter();
  return useCallback(
    (iso?: string) => {
      if (!iso) return "";
      const date = new Date(iso);
      if (Number.isNaN(date.getTime())) return "";
      const today = date.toDateString() === new Date().toDateString();
      return today
        ? format.dateTime(date, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
        : format.dateTime(date, { day: "numeric", month: "short" });
    },
    [format],
  );
}
