"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Link } from "../../../i18n/navigation";
import { formatAge } from "../../console/formatters";
import type { AttentionItem, RunningItem } from "../../[locale]/(console)/dashboardModel";

/** A section title on the canvas, with its one link on the right. */
export function DashSection({ id, title, more, children }: { id: string; title: string; more?: { href: string; label: string }; children: ReactNode }) {
  return (
    <section className="db-sec" aria-labelledby={id}>
      <div className="db-sh">
        <h2 id={id}>{title}</h2>
        {more && <Link href={more.href} className="db-more">{more.label}</Link>}
      </div>
      {children}
    </section>
  );
}

const clock = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
};

/** Two-line rows: the problem in a status tone, then its facts. The whole row opens the alert or the device. */
export function NeedsAttention({ items }: { items: AttentionItem[] }) {
  const t = useTranslations("dashboard.attention");
  const words = (part: AttentionItem["title"] | NonNullable<AttentionItem["sub"]>) =>
    "text" in part ? part.text : t(part.key, part.key === "lastSeen" ? { age: formatAge(part.values.at) } : part.values);
  return (
    <div className="db-list">
      {items.map((item) => (
        <Link key={item.key} href={item.href} className="db-row two">
          <span className="db-lead">{item.tone && <i className={`db-dot ${item.tone}`} aria-hidden="true" />}</span>
          <span className="db-txt">
            <span className="db-nm">{words(item.title)}</span>
            {item.sub && <span className="db-sub">{words(item.sub)}</span>}
          </span>
          <time className="db-at" dateTime={item.at}>{clock(item.at)}</time>
        </Link>
      ))}
    </div>
  );
}

/** Jobs in progress, each with a turning ring (the hub reports no share done), what runs and since when. */
export function Running({ items }: { items: RunningItem[] }) {
  const t = useTranslations("dashboard.running");
  return (
    <div className="db-list">
      {items.map((item) => (
        <Link key={item.key} href={item.href} className="db-row one">
          <span className="db-lead">
            <svg className="db-ring" width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
              <circle className="db-ring-t" cx="7" cy="7" r="5.5" />
              <circle className="db-ring-a" cx="7" cy="7" r="5.5" strokeDasharray="8.6 34.6" />
            </svg>
          </span>
          <span className="db-txt">
            <span className="db-nm plain">{item.target ? t("on", { label: item.label, target: item.target }) : item.label}</span>
          </span>
          <time className="db-at" dateTime={item.at}>{t("started", { time: clock(item.at) })}</time>
        </Link>
      ))}
    </div>
  );
}
