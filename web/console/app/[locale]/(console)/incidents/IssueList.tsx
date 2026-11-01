"use client";

import type { MouseEvent } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "../../../../i18n/navigation";
import { fullWhen, isActive, listTone, openedAt, shortWhen, type Issue, type IssueSort, type IssueSortKey } from "./issuesModel";

const COLUMNS: IssueSortKey[] = ["title", "state", "owner", "opened"];

function SortHead({ col, sort, onSort }: { col: IssueSortKey; sort: IssueSort; onSort: (key: IssueSortKey) => void }) {
  const t = useTranslations("incidents.list.columns");
  const on = sort.key === col;
  const Arrow = sort.dir === "asc" ? ChevronUp : ChevronDown;
  return (
    <span role="columnheader" aria-sort={on ? (sort.dir === "asc" ? "ascending" : "descending") : "none"} className={col === "opened" ? "iu-r" : undefined}>
      <button type="button" className={`iu-sort${on ? " on" : ""}`} onClick={() => onSort(col)}>
        {t(col === "title" ? "issue" : col)}
        {on ? <Arrow aria-hidden="true" strokeWidth={2.4} /> : null}
      </button>
    </span>
  );
}

function IssueRow({ issue, device, locale }: { issue: Issue; device?: string; locale: string }) {
  const t = useTranslations("incidents");
  const router = useRouter();
  const href = `/incidents/${encodeURIComponent(issue.id)}`;
  const tone = listTone(issue);
  const opened = openedAt(issue);
  const open = (event: MouseEvent<HTMLDivElement>) => {
    if ((event.target as HTMLElement).closest("a, button")) return;
    if (event.metaKey || event.ctrlKey) window.open(href, "_blank", "noopener,noreferrer");
    else router.push(href);
  };
  return (
    <div role="row" className={`iu-row iu-is${isActive(issue) ? "" : " done"}`} onClick={open}>
      <span className="iu-lead" aria-hidden="true">{tone ? <i className={`iu-dot ${tone}`} /> : null}</span>
      <div role="cell" className="iu-main">
        <Link href={href} className="iu-nm">{issue.title}</Link>
        {device ? <span className="iu-grey">{device}</span> : null}
      </div>
      <span role="cell" className="iu-sw">{t(`states.${issue.status}`)}</span>
      <span role="cell" className={issue.assignee ? "iu-own-c" : "iu-own-c iu-c3"}>{issue.assignee || "—"}</span>
      <span role="cell" className="iu-r iu-c3 iu-num"><time dateTime={opened} title={fullWhen(opened, locale)}>{shortWhen(opened, locale)}</time></span>
    </div>
  );
}

/** The issues as one list: severity dot (active critical or high only), title and device, state, owner, opened. */
export function IssueList({
  issues,
  loading,
  sort,
  onSort,
  deviceNames,
  emptyText,
}: {
  issues: Issue[];
  loading: boolean;
  sort: IssueSort;
  onSort: (key: IssueSortKey) => void;
  deviceNames: Map<string, string>;
  emptyText: string;
}) {
  const t = useTranslations("incidents");
  const locale = useLocale();
  return (
    <div className="iu-list iu-hang" role="table" aria-label={t("title")} aria-busy={loading}>
      <div role="row" className="iu-row iu-is iu-cols">
        {COLUMNS.map((col) => <SortHead key={col} col={col} sort={sort} onSort={onSort} />)}
      </div>
      {loading && !issues.length ? (
        <><div className="iu-skel" /><div className="iu-skel" /><div className="iu-skel" /></>
      ) : issues.length ? (
        issues.map((issue) => (
          <IssueRow
            key={issue.id}
            issue={issue}
            locale={locale}
            device={issue.primary_asset_id ? deviceNames.get(issue.primary_asset_id) ?? issue.primary_asset_id : undefined}
          />
        ))
      ) : (
        <div role="row">
          <div role="cell" className="iu-empty">
            <b>{emptyText}</b>
            <Link href="/alerts" className="iu-link">{t("list.openAlerts")}</Link>
          </div>
        </div>
      )}
    </div>
  );
}
