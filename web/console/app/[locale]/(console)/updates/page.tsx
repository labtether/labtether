"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { useAuth } from "../../../contexts/AuthContext";
import { useFastStatus, useStatusAssetNameMap } from "../../../contexts/StatusContext";
import { useUpdates } from "../../../hooks/useUpdates";
import { hasWriteRole } from "../../../lib/roles";
import { PlanForm } from "./PlanForm";
import { PlanList, RunMenu } from "./PlanList";
import { PlanRun } from "./PlanRun";
import { RunHistory } from "./RunHistory";

type Tab = "plans" | "history";
type Stage = { kind: "list" } | { kind: "plan"; id: string } | { kind: "new" };

/**
 * Updates (2.0): OS package updates across devices. Plans name the devices; opening one shows them with their
 * status and last result, and runs it (or only previews it). Runs in progress sit at the top; History lists every run
 * with what each device got.
 */
export default function UpdatesPage() {
  const t = useTranslations("updates");
  const tn = useTranslations("nav");
  const { user } = useAuth();
  const canWrite = hasWriteRole(user?.role);
  const assets = useFastStatus()?.assets ?? [];
  const deviceNames = useStatusAssetNameMap();
  const { plans, runs, loaded, defaultDryRun, createPlan, runPlan } = useUpdates();
  const [tab, setTab] = useState<Tab>("plans");
  const [stage, setStage] = useState<Stage>({ kind: "list" });
  const [notice, setNotice] = useState("");

  const plan = stage.kind === "plan" ? plans.find((p) => p.id === stage.id) : undefined;
  const back = () => setStage({ kind: "list" });
  const listing = stage.kind === "list" || (stage.kind === "plan" && !plan);

  const openPlan = (id: string) => { setNotice(""); setStage({ kind: "plan", id }); };
  const action = listing && tab === "plans" && canWrite && plans.length ? (
    <div className="iu-acts">
      <Button onClick={() => { setNotice(""); setStage({ kind: "new" }); }}>
        <Plus aria-hidden="true" size={14} strokeWidth={2} />
        {t("newPlan")}
      </Button>
      <RunMenu plans={plans} onOpen={openPlan} />
    </div>
  ) : undefined;

  return (
    <>
      <nav className="iu-crumb" aria-label={t("crumbs")}>
        <Link href="/nodes">{tn("devices")}</Link>
        <span aria-hidden="true">/</span>
        <span className="iu-crumb-here" aria-current="page">{t("title")}</span>
      </nav>
      <PageHeader title={t("title")} action={action} />
      {listing ? (
        <div className="iu-tabs" role="tablist" aria-label={t("tabs.label")}>
          {(["plans", "history"] as const).map((key) => (
            <button key={key} type="button" role="tab" aria-selected={tab === key} aria-controls="iu-up-panel" className={tab === key ? "on" : undefined} onClick={() => setTab(key)}>
              {t(`tabs.${key}`)}
            </button>
          ))}
        </div>
      ) : null}
      {notice ? <p role="status" className="iu-notice">{notice}</p> : null}
      <div className="iu-page" id="iu-up-panel" role={listing ? "tabpanel" : undefined}>
        {stage.kind === "new" ? (
          <PlanForm
            assets={assets}
            defaultDryRun={defaultDryRun}
            onCancel={back}
            onSave={async (input) => {
              const result = await createPlan(input);
              if (result.ok) { setNotice(t("form.saved", { plan: input.name.trim() })); back(); }
              return result;
            }}
          />
        ) : plan ? (
          <PlanRun
            key={plan.id}
            plan={plan}
            runs={runs}
            assets={assets}
            canWrite={canWrite}
            onBack={back}
            onRun={async (dryRun) => {
              const result = await runPlan(plan.id, dryRun);
              if (result.ok) { setNotice(t(dryRun ? "plan.previewStarted" : "plan.started", { plan: plan.name })); setTab("plans"); back(); }
              return result;
            }}
          />
        ) : !loaded ? (
          <div className="iu-list" aria-busy="true"><div className="iu-skel" /><div className="iu-skel" /><div className="iu-skel" /></div>
        ) : tab === "history" ? (
          <RunHistory runs={runs} deviceNames={deviceNames} />
        ) : (
          <PlanList
            plans={plans}
            runs={runs}
            deviceNames={deviceNames}
            canWrite={canWrite}
            onNew={() => { setNotice(""); setStage({ kind: "new" }); }}
            onOpen={openPlan}
          />
        )}
      </div>
    </>
  );
}
