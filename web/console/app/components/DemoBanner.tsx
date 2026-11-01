"use client";

import { useTranslations } from "next-intl";
import { INSTALLATION_DOCS_URL } from "../lib/externalLinks";

/** 2.0 demo line: one quiet line above the console with a link to install. */
export function DemoBanner({ demoMode }: { demoMode?: boolean }) {
  const t = useTranslations("common.demoBanner");
  if (!demoMode) return null;

  return (
    <div className="kt-demo">
      <span>{t("message")}</span>
      <a href={INSTALLATION_DOCS_URL} target="_blank" rel="noopener noreferrer">
        {t("install")}
      </a>
    </div>
  );
}
