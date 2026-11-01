"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Facts, Section, Sheet } from "./SettingsKit";

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

  const uptime = data?.started_at ? formatUptime(data.started_at) : "";

  return (
    <Section title={t("about.heading")} id="about">
      <Sheet>
        <Facts
          items={[
            [t("about.version"), <span key="v" className="st-num">{data?.version ?? "—"}</span>],
            uptime ? [t("about.uptime"), <span key="u" className="st-num">{uptime}</span>] : false,
            // Sign-in lasts 24 h: a fixed hub value, not a setting yet.
            [t("about.sessionLifetime"), t("about.sessionLength")],
          ]}
        />
      </Sheet>
    </Section>
  );
}
