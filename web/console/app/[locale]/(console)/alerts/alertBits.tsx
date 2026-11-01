"use client";

import { useTranslations } from "next-intl";
import { sevTone, splitSeconds } from "./alertsModel";

/** An 8 px status dot. "off" is the hollow ring for silenced and resolved alerts. */
export function Dot({ tone }: { tone: "bad" | "warn" | "ok" | "off" | "" }) {
  return tone ? <i className={`al-dot ${tone}`} aria-hidden="true" /> : <span className="al-dot-none" aria-hidden="true" />;
}

/** "● Critical" in red, "● High" in amber, a plain "Medium" or "Low". */
export function SevLabel({ severity }: { severity: string }) {
  const t = useTranslations("alerts.severity");
  const tone = sevTone(severity);
  return (
    <span className={`al-sev${tone ? ` ${tone}` : ""}`}>
      {tone ? <Dot tone={tone} /> : null}
      {t.has(severity) ? t(severity) : severity}
    </span>
  );
}

/** Seconds in their largest whole unit: "5 min", "2 h", "90 s". */
export function useDuration() {
  const t = useTranslations("alerts.duration");
  return (seconds: number) => {
    const { unit, n } = splitSeconds(seconds);
    return t(unit, { n });
  };
}
