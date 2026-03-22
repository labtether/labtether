"use client";

import { useTranslations } from "next-intl";
import { ExternalLink } from "lucide-react";
import { Card } from "../../../../components/ui/Card";
import { Link } from "../../../../../i18n/navigation";

export function ApiReferenceCard() {
  const t = useTranslations("settings");

  return (
    <Card className="mb-6">
      <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
        {t("apiReference.heading")}
      </p>
      <p className="text-xs text-[var(--muted)] mb-3">{t("apiReference.description")}</p>
      <div className="flex items-center gap-3">
        <Link
          href="/api-docs"
          className="inline-flex items-center gap-2 rounded-lg px-2.5 py-1 text-xs font-medium bg-transparent border border-[var(--control-border)] text-[var(--control-fg)] hover:bg-[var(--control-bg-hover)] hover:border-[var(--text-secondary)] transition-[color,background-color,border-color,box-shadow,opacity] duration-[var(--dur-fast)]"
        >
          <ExternalLink size={13} />
          {t("apiReference.viewDocs")}
        </Link>
        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
          {t("apiReference.endpointCount")}
        </span>
      </div>
    </Card>
  );
}
