"use client";

import { useTranslations } from "next-intl";
import { INSTALLATION_DOCS_URL } from "../lib/externalLinks";

export function DemoBanner({ demoMode }: { demoMode?: boolean }) {
  const t = useTranslations("common.demoBanner");
  if (!demoMode) return null;

  return (
    <div className="bg-blue-600 text-white text-center text-sm py-2 px-4 shrink-0">
      {t("message")}{" "}
      <a
        href={INSTALLATION_DOCS_URL}
        className="underline font-medium hover:text-blue-100"
        target="_blank"
        rel="noopener noreferrer"
      >
        {t("install")} &rarr;
      </a>
    </div>
  );
}
