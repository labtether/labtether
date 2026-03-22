"use client";

import { useLocale } from "next-intl";
import { useRouter, usePathname } from "next/navigation";
import { useCallback, type ChangeEvent } from "react";
import { Globe } from "lucide-react";

const localeLabels: Record<string, string> = {
  en: "English",
  de: "Deutsch",
  fr: "Français",
  es: "Español",
  zh: "简体中文",
};

const supportedLocales = ["en", "de", "fr", "es", "zh"] as const;

export function LanguagePicker() {
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();

  const handleChange = useCallback(
    (e: ChangeEvent<HTMLSelectElement>) => {
      const nextLocale = e.target.value;

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

  return (
    <div className="flex items-center gap-1.5 px-2">
      <Globe className="w-3 h-3 text-[var(--muted)] shrink-0" strokeWidth={1.5} />
      <select
        value={locale}
        onChange={handleChange}
        className="flex-1 min-w-0 bg-transparent text-[10px] font-mono text-[var(--muted)] outline-none cursor-pointer appearance-none hover:text-[var(--text)] transition-colors"
        aria-label="Select language"
      >
        {supportedLocales.map((loc) => (
          <option key={loc} value={loc} className="bg-[var(--bg)] text-[var(--text)]">
            {localeLabels[loc]}
          </option>
        ))}
      </select>
    </div>
  );
}
