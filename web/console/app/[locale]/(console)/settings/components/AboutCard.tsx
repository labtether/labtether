"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";

type VersionPayload = {
  version?: string;
  started_at?: string;
};

function formatUptime(startedAt: string): string {
  const startMs = new Date(startedAt).getTime();
  if (Number.isNaN(startMs)) return "";
  const diffMs = Date.now() - startMs;
  if (diffMs < 0) return "";
  const totalSeconds = Math.floor(diffMs / 1000);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function AboutCard() {
  const t = useTranslations("settings");
  const [data, setData] = useState<VersionPayload | null>(null);

  useEffect(() => {
    fetch("/api/version")
      .then((r) => (r.ok ? r.json() : null))
      .then((payload) => {
        if (payload && typeof payload === "object" && !("error" in payload)) {
          setData(payload as VersionPayload);
        }
      })
      .catch(() => {});
  }, []);

  const version = data?.version ?? "—";
  const uptime = data?.started_at ? formatUptime(data.started_at) : null;

  return (
    <Card className="mb-6">
      <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t("about.heading")}</p>
      <p className="text-xs text-[var(--muted)]">
        {t("about.version", { version })}
        {uptime ? ` · ${t("about.uptime", { uptime })}` : ""}
        {/* Session lifetime is hardcoded to 24 h; making it configurable requires a backend runtime setting */}
        {` · ${t("about.sessionLifetime", { duration: "24h" })}`}
      </p>
    </Card>
  );
}
