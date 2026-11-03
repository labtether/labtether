import type { useTranslations } from "next-intl";
import type { AddDeviceCompatPrefill } from "./types";

/** The `auth.addDevice` translator that the setup steps share. */
export type AddDeviceT = ReturnType<typeof useTranslations>;

/** Text for a validator's key (see validation.ts), or "" when there is no problem. */
export function validationText(t: AddDeviceT, key: string): string {
  return key ? t(`validation.${key}`) : "";
}

/** The first non-empty message, or "". */
export function firstError(...messages: string[]): string {
  return messages.find((message) => message) ?? "";
}

/** One option in the "Detected address" list: its name, its URL and how sure discovery is. */
export function compatPrefillOption(t: AddDeviceT, prefill: AddDeviceCompatPrefill): string {
  return t("form.detectedOption", {
    name: prefill.serviceName || prefill.baseURL,
    url: prefill.baseURL,
    match: Math.max(0, Math.min(1, prefill.confidence)),
  });
}
