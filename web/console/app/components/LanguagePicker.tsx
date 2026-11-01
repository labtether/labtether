"use client";

import { useLocale } from "next-intl";
import { useRouter, usePathname } from "next/navigation";
import { useCallback } from "react";

export const localeLabels: Record<string, string> = {
  en: "English",
  de: "Deutsch",
  fr: "Français",
  es: "Español",
  zh: "简体中文",
};

export const supportedLocales = ["en", "de", "fr", "es", "zh"] as const;

/** The current locale and a switch to another one (the account menu's Language list). */
export function useSwitchLocale() {
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();

  const switchTo = useCallback(
    (nextLocale: string) => {
      // Set the NEXT_LOCALE cookie so middleware picks up the preference.
      document.cookie = `NEXT_LOCALE=${nextLocale};path=/;max-age=31536000;samesite=lax`;

      // Strip the current locale prefix from the pathname (if present) to get the
      // locale-independent path, then build the new URL.
      let basePath = pathname;
      for (const loc of supportedLocales) {
        if (pathname === `/${loc}` || pathname.startsWith(`/${loc}/`)) {
          basePath = pathname.slice(`/${loc}`.length) || "/";
          break;
        }
      }

      const nextPath = `/${nextLocale}${basePath === "/" ? "" : basePath}`;
      router.push(nextPath);
      router.refresh();
    },
    [pathname, router],
  );

  return { locale, switchTo };
}
