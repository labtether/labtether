"use client";

import { useMemo, useState } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { AlertTabs } from "../alerts/AlertTabs";
import { Button } from "../../../components/ui/Button";
import { useStatusAssetNameMap } from "../../../contexts/StatusContext";
import { useIncidents } from "../../../hooks/useIncidents";
import { IssueList } from "./IssueList";
import {
  filterIssues, nextSort, SHOWS, sortIssues, STATES,
  type Issue, type IssueFilter, type IssueShow, type IssueSort, type IssueStatus,
} from "./issuesModel";

/** Active, Resolved or All as a segmented switch, and one exact state from the State menu. */
function IssuesToolbar({ filter, onFilter }: { filter: IssueFilter; onFilter: (value: IssueFilter) => void }) {
  const t = useTranslations("incidents");
  const exact = (STATES as string[]).includes(filter) ? (filter as IssueStatus) : null;
  return (
    <div className="iu-tools">
      <div className="iu-seg" role="group" aria-label={t("list.showLabel")}>
        {SHOWS.map((show) => (
          <button key={show} type="button" className={filter === show ? "on" : undefined} aria-pressed={filter === show} onClick={() => onFilter(show)}>
            {t(`list.show.${show}`)}
          </button>
        ))}
      </div>
      <Menu.Root>
        <Menu.Trigger className={`iu-chip${exact ? " on" : ""}`}>
          <span className="iu-chip-k">{t("list.stateFilter")}</span>
          {exact ? t(`states.${exact}`) : t("list.anyState")}
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="start" sideOffset={6}>
            <Menu.RadioGroup value={exact ?? "any"} onValueChange={(v) => onFilter((v === "any" ? "active" : v) as IssueFilter)}>
              <Menu.RadioItem value="any" className="lt-menu-item">
                {t("list.anyState")}
                <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
              </Menu.RadioItem>
              <Menu.Separator className="lt-menu-sep" />
              {STATES.map((s) => (
                <Menu.RadioItem key={s} value={s} className="lt-menu-item">
                  {t(`states.${s}`)}
                  <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
                </Menu.RadioItem>
              ))}
            </Menu.RadioGroup>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}

/**
 * Issues (2.0): one list of the hub's issues, active ones first by default, newest first. A row opens the issue page.
 */
export default function IssuesPage() {
  const t = useTranslations("incidents");
  const tNav = useTranslations("nav");
  const { incidents, loading, error, refresh } = useIncidents();
  const deviceNames = useStatusAssetNameMap();
  const [filter, setFilter] = useState<IssueFilter>("active");
  const [sort, setSort] = useState<IssueSort>({ key: "opened", dir: "desc" });

  const shown = useMemo(() => sortIssues(filterIssues(incidents as Issue[], filter), sort), [incidents, filter, sort]);
  const emptyText = (SHOWS as string[]).includes(filter) ? t(`list.empty.${filter as IssueShow}`) : t("list.empty.state");

  return (
    <>
      <PageHeader title={tNav("alerts")} />
      <AlertTabs current="issues" />
      <IssuesToolbar filter={filter} onFilter={setFilter} />
      {error ? (
        <div className="iu-inline-err iu-top-err">
          <p role="alert">{t("list.error")}</p>
          <Button size="sm" onClick={() => void refresh()}>{t("list.retry")}</Button>
        </div>
      ) : null}
      <IssueList
        issues={shown}
        loading={loading}
        sort={sort}
        onSort={(key) => setSort((current) => nextSort(current, key))}
        deviceNames={deviceNames}
        emptyText={emptyText}
      />
    </>
  );
}
