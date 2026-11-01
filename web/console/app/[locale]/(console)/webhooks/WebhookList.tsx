"use client";

import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";
import { pastWhen, timeValue } from "../actions/_parts/autoTime";
import { SortHead, type SortState } from "../actions/_parts/Editor";
import { RowMenu, Switch } from "../actions/_parts/RowMenu";
import { eventParts } from "./webhookEvents";
import type { WebhookRecord } from "./webhookPayload";

type Key = "name" | "last";

function sorted(list: WebhookRecord[], sort: SortState<Key>): WebhookRecord[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  const byName = (a: WebhookRecord, b: WebhookRecord) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
  if (sort.key === "name") return [...list].sort((a, b) => sign * byName(a, b));
  // Newest first on the first click; never-sent webhooks last either way.
  return [...list].sort((a, b) => {
    const x = timeValue(a.last_triggered_at, 0);
    const y = timeValue(b.last_triggered_at, 0);
    if (!x || !y) return x === y ? byName(a, b) : x ? -1 : 1;
    return -sign * (x - y) || byName(a, b);
  });
}

/** Webhooks as one list: name over the address, the events in words, the last time it sent, an On switch, ⋯. */
export function WebhookList({
  webhooks,
  loading,
  failed,
  onEdit,
  onToggle,
  onDelete,
  onRetry,
}: {
  webhooks: WebhookRecord[];
  loading: boolean;
  failed: boolean;
  onEdit: (webhook: WebhookRecord) => void;
  onToggle: (webhook: WebhookRecord, on: boolean) => void;
  onDelete: (webhook: WebhookRecord) => void;
  onRetry: () => void;
}) {
  const t = useTranslations("webhooks.list");
  const te = useTranslations("webhooks.events");
  const tc = useTranslations("actions.common");
  const locale = useLocale();
  const [sort, setSort] = useState<SortState<Key>>({ key: "name", dir: "asc" });
  const now = new Date();
  const events = (list: string[]) => eventParts(list).map((p) => ("key" in p ? te(p.key) : p.raw)).join(", ");

  return (
    <div className="au-list" role="region" aria-label={t("label")}>
      <div className="au-cols au-w">
        <SortHead label={t("webhook")} column="name" sort={sort} onSort={setSort} />
        <span className="au-ev">{t("events")}</span>
        <SortHead label={t("last")} column="last" sort={sort} onSort={setSort} />
        <span>{tc("on")}</span>
        <span />
      </div>
      {loading && webhooks.length === 0 ? <p className="au-empty" role="status">{tc("loading")}</p> : null}
      {failed ? (
        <div className="au-empty">
          <p role="alert" className="au-err">{t("loadFailed")}</p>
          <button type="button" className="au-link" onClick={onRetry}>{tc("retry")}</button>
        </div>
      ) : null}
      {!loading && !failed && webhooks.length === 0 ? <p className="au-empty">{t("empty")}</p> : null}
      {sorted(webhooks, sort).map((w) => {
        const last = pastWhen(w.last_triggered_at, now, locale);
        return (
          <div key={w.id} className={`au-row au-w${w.enabled ? "" : " off"}`} onClick={(e) => { if (!(e.target as HTMLElement).closest("button, a")) onEdit(w); }}>
            <span className="au-ln">
              <button type="button" className="au-nm au-tg" onClick={() => onEdit(w)}>{w.name}</button>
              <span className="au-sub">{w.url}</span>
            </span>
            <span className="au-ev" title={w.events.join(", ") || undefined}>{events(w.events)}</span>
            <span className="au-last">{last ? <span className="num">{last}</span> : <span className="au-dim">{tc("never")}</span>}</span>
            <Switch on={w.enabled} label={t("onLabel", { name: w.name })} onChange={(on) => onToggle(w, on)} />
            <RowMenu
              label={tc("rowMenu", { name: w.name })}
              items={[
                { label: tc("edit"), onSelect: () => onEdit(w) },
                { label: w.enabled ? t("turnOff") : t("turnOn"), onSelect: () => onToggle(w, !w.enabled) },
                { separator: true },
                { label: tc("deleteMenu"), onSelect: () => onDelete(w), danger: true },
              ]}
            />
          </div>
        );
      })}
    </div>
  );
}
