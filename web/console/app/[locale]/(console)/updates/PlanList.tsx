"use client";

import { useState, type MouseEvent } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, Search } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import type { UpdatePlan, UpdateRun } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import { fullWhen, shortWhen } from "../incidents/issuesModel";
import { lastRunByPlan, runTone } from "./updatesModel";

/** Last run as a word and a time: "Done 21:12", "Failed 18 Oct", "Running since 21:40". */
export function useRunWhen() {
  const t = useTranslations("updates.lastRun");
  const locale = useLocale();
  return (run: UpdateRun) => {
    const time = shortWhen(run.completed_at || run.created_at, locale);
    const key = ["queued", "running", "succeeded", "failed"].includes(run.status) ? run.status : "other";
    return t(key, { time });
  };
}

/** The page's main button: pick a plan to run; it opens that plan, where the run starts. */
export function RunMenu({ plans, onOpen }: { plans: UpdatePlan[]; onOpen: (planId: string) => void }) {
  const t = useTranslations("updates.plans");
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <Button variant="primary" disabled={!plans.length}>
          {t("run")}
          <ChevronDown aria-hidden="true" size={14} strokeWidth={2} />
        </Button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={6}>
          {plans.map((plan) => (
            <Menu.Item key={plan.id} className="lt-menu-item" onSelect={() => onOpen(plan.id)}>
              {plan.name}
              <span className="lt-menu-hint">{plan.targets.length}</span>
            </Menu.Item>
          ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/**
 * Every update plan: a dot while its last run is going (or when it failed), its name over the devices it covers, how
 * many, and how its last run went. A row opens the plan, where the run starts. The filter matches plan or device names.
 */
export function PlanList({
  plans,
  runs,
  deviceNames,
  onOpen,
  onNew,
  canWrite,
}: {
  plans: UpdatePlan[];
  runs: UpdateRun[];
  deviceNames: Map<string, string>;
  onOpen: (planId: string) => void;
  onNew: () => void;
  canWrite: boolean;
}) {
  const t = useTranslations("updates.plans");
  const locale = useLocale();
  const runWhen = useRunWhen();
  const [query, setQuery] = useState("");
  const last = lastRunByPlan(runs);

  if (!plans.length) {
    return (
      <div className="iu-list">
        <div className="iu-empty">
          <b>{t("empty")}</b>
          {canWrite ? <Button variant="primary" onClick={onNew}>{t("new")}</Button> : null}
        </div>
      </div>
    );
  }

  const names = (plan: UpdatePlan) => plan.targets.map((id) => deviceNames.get(id) ?? id);
  const q = query.trim().toLowerCase();
  const shown = q ? plans.filter((p) => [p.name, ...names(p)].some((v) => v.toLowerCase().includes(q))) : plans;
  const open = (id: string) => (event: MouseEvent<HTMLDivElement>) => {
    if ((event.target as HTMLElement).closest("a, button")) return;
    onOpen(id);
  };

  return (
    <>
      <div className="iu-tools">
        <label className="iu-filter">
          <Search aria-hidden="true" strokeWidth={2} />
          <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("filter")} aria-label={t("filter")} />
        </label>
      </div>
      <div className="iu-list iu-hang" role="table" aria-label={t("label")}>
        <div role="row" className="iu-row iu-pl iu-cols">
          <span role="columnheader">{t("columns.plan")}</span>
          <span role="columnheader" className="iu-r iu-pl-n">{t("columns.devices")}</span>
          <span role="columnheader">{t("columns.lastRun")}</span>
        </div>
        {shown.map((plan) => {
          const run = last.get(plan.id);
          const tone = run ? runTone(run) : "";
          const at = run ? run.completed_at || run.created_at : "";
          return (
            <div key={plan.id} role="row" className="iu-row iu-pl" onClick={open(plan.id)}>
              <span className="iu-lead" aria-hidden="true">{tone ? <i className={`iu-dot ${tone}`} /> : null}</span>
              <div role="cell" className="iu-main">
                <button type="button" className="iu-nm" onClick={() => onOpen(plan.id)}>{plan.name}</button>
                <span className="iu-grey">{plan.default_dry_run ? `${names(plan).join(", ")} · ${t("previewDefault")}` : names(plan).join(", ")}</span>
              </div>
              <span role="cell" className="iu-r iu-num iu-pl-n">{plan.targets.length}</span>
              <span role="cell" className={`iu-when${tone === "bad" ? " bad" : ""}`}>
                {run ? <time dateTime={at} title={fullWhen(at, locale)}>{runWhen(run)}</time> : <span className="iu-c3">{t("never")}</span>}
              </span>
            </div>
          );
        })}
        {!shown.length ? <div role="row"><p role="cell" className="iu-empty">{t("noMatch", { query: query.trim() })}</p></div> : null}
      </div>
    </>
  );
}
