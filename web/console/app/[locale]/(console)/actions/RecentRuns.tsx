"use client";

import { useLocale, useTranslations } from "next-intl";
import { Fragment, useState } from "react";
import { Link } from "../../../../i18n/navigation";
import type { ActionRun, ConnectorDescriptor } from "../../../console/models";
import { pastWhen } from "./_parts/autoTime";
import { runTone } from "./actionsModel";

const SHOWN = 10;

/** An action id the page has no name for, in words: "container.restart" → "Container restart". */
const inWords = (id: string) => {
  const words = id.replace(/[._]+/g, " ").trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
};

/**
 * The hub's latest action runs (integration actions and commands): what ran, on which device, and the result as a word
 * and a time. A run with output or an error opens to show them; a failed run links to Logs.
 */
export function RecentRuns({
  runs,
  connectors,
  actionNames,
  deviceName,
}: {
  runs: ActionRun[];
  connectors: ConnectorDescriptor[];
  /** Action names the page knows, keyed "connector:action". Others show their id in words ("Vm snapshot"). */
  actionNames: Map<string, string>;
  deviceName: (id: string) => string;
}) {
  const t = useTranslations("actions.runs");
  const locale = useLocale();
  const [open, setOpen] = useState<string | null>(null);
  const connectorName = (id: string) => connectors.find((c) => c.id === id)?.display_name ?? id;
  const latest = [...runs].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, SHOWN);

  return (
    <section className="au-sec" aria-labelledby="au-runs-h">
      <div className="au-sh"><h2 id="au-runs-h">{t("title")}</h2></div>
      <div className="au-list">
        {latest.length === 0 ? <p className="au-empty">{t("empty")}</p> : null}
        {latest.map((run) => {
          const tone = runTone(run);
          const time = pastWhen(run.completed_at || run.updated_at || run.created_at, new Date(), locale);
          const action = run.action_id ?? "";
          const what = run.connector_id
            ? `${connectorName(run.connector_id)} · ${actionNames.get(`${run.connector_id}:${action}`) ?? inWords(action)}`
            : run.command || run.type;
          const mono = !run.connector_id && Boolean(run.command);
          const detail = Boolean(run.output || run.error);
          const isOpen = open === run.id;
          return (
            <Fragment key={run.id}>
              <div className={`au-row au-r${isOpen ? " open" : ""}`} onClick={() => detail && setOpen(isOpen ? null : run.id)}>
                <span className="au-ln">
                  {detail ? (
                    <button type="button" className={`au-nm au-tg${mono ? " au-mono" : ""}`} aria-expanded={isOpen} onClick={(e) => { e.stopPropagation(); setOpen(isOpen ? null : run.id); }}>{what}</button>
                  ) : <span className={`au-nm${mono ? " au-mono" : ""}`}>{what}</span>}
                  {run.target ? <span className="au-sub">{deviceName(run.target)}</span> : null}
                </span>
                <span className="au-last">
                  {tone === "busy" ? <span className="au-word busy">{t("running")}</span>
                    : tone === "wait" ? <span className="au-word">{t("queued")}</span>
                      : tone === "bad" ? <><span className="au-word bad">{t("failed")}</span><span className="num"> · {time}</span></>
                        : <span className="num">{time}</span>}
                </span>
                {tone === "bad" ? <Link href="/logs" className="au-link" onClick={(e) => e.stopPropagation()}>{t("logs")}</Link> : <span />}
              </div>
              {isOpen ? (
                <div className="au-panel">
                  <div className="au-pc">
                    {run.error ? <p className="au-err">{run.error}</p> : null}
                    {run.output ? <pre className="au-code au-out">{run.output}</pre> : null}
                  </div>
                </div>
              ) : null}
            </Fragment>
          );
        })}
      </div>
    </section>
  );
}
