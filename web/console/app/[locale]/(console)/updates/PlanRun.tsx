"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import type { Asset, UpdatePlan, UpdateRun } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import type { UpdateActionResult } from "../../../hooks/useUpdates";
import { DeviceLine } from "../incidents/DeviceLine";
import { SwitchRow } from "./Switch";
import { lastResults } from "./updatesModel";

/**
 * One plan before it runs: the devices it covers with their live status and what each got last time, then whether to
 * only preview, and the run itself as the page's main button.
 */
export function PlanRun({
  plan,
  runs,
  assets,
  canWrite,
  onBack,
  onRun,
}: {
  plan: UpdatePlan;
  runs: UpdateRun[];
  assets: Asset[];
  canWrite: boolean;
  onBack: () => void;
  onRun: (dryRun: boolean) => Promise<UpdateActionResult>;
}) {
  const t = useTranslations("updates.plan");
  const ts = useTranslations("updates.status");
  const [dryRun, setDryRun] = useState(plan.default_dry_run);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<{ detail?: string } | null>(null);
  const results = lastResults(runs, plan.id);
  const byId = new Map(assets.map((a) => [a.id, a]));
  const count = plan.targets.length;

  // Esc leaves the plan, as Back does.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !e.defaultPrevented) onBack(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onBack]);

  const run = async () => {
    setBusy(true);
    setFailed(null);
    const result = await onRun(dryRun);
    setBusy(false);
    if (!result.ok) setFailed({ detail: result.detail });
  };

  return (
    <>
      <section className="iu-sec" aria-labelledby="iu-plan-h">
        <div className="iu-sh">
          <h2 id="iu-plan-h">{plan.name}</h2>
          {plan.description ? <span className="iu-sh-note">{plan.description}</span> : null}
        </div>
        <div className="iu-list iu-hang" aria-label={t("devices")}>
          {plan.targets.map((id) => {
            const result = results.get(id);
            const tone = result?.status === "failed" ? " bad" : "";
            return (
              <DeviceLine
                key={id}
                id={id}
                asset={byId.get(id)}
                extra={result?.summary ? <p className={`iu-sub${tone}`}>{result.summary}</p> : null}
                right={result ? <span className={`iu-res${tone}`}>{ts.has(result.status) ? ts(result.status) : result.status}</span> : null}
              />
            );
          })}
        </div>
      </section>
      <section className="iu-sec" aria-labelledby="iu-opts-h">
        <div className="iu-sh"><h2 id="iu-opts-h">{t("options")}</h2></div>
        <div className="iu-sheet iu-opts">
          <div className="iu-opt"><span>{t("what")}</span><span className="iu-c2">{t("osPackages")}</span></div>
          <SwitchRow id="iu-dry" label={t("previewOnly")} on={dryRun} onChange={setDryRun} disabled={!canWrite} />
          {failed ? (
            <p role="alert" className="iu-err-line">{t("runFailed")}{failed.detail ? <span className="iu-c3"> {failed.detail}</span> : null}</p>
          ) : null}
          <div className="iu-sheet-foot">
            <Button variant="ghost" onClick={onBack}>{t("back")}</Button>
            {canWrite ? (
              <Button variant="primary" disabled={busy || !count} onClick={() => void run()}>
                {t(dryRun ? "preview" : "run", { count })}
              </Button>
            ) : null}
          </div>
        </div>
      </section>
    </>
  );
}
