"use client";

import { useMemo, useState } from "react";
import { Search } from "lucide-react";
import { useTranslations } from "next-intl";
import type { Asset } from "../../../console/models";
import { Button } from "../../../components/ui/Button";
import type { UpdateActionResult } from "../../../hooks/useUpdates";
import { deviceState } from "../incidents/DeviceLine";
import { SwitchRow } from "./Switch";
import { pickableDevices } from "./updatesModel";

/**
 * A new plan: its name, the devices it updates (picked from Devices) and whether its runs only preview by default.
 * The plan updates OS packages, the one kind of update the hub runs.
 */
export function PlanForm({
  assets,
  defaultDryRun,
  onCancel,
  onSave,
}: {
  assets: Asset[];
  defaultDryRun: boolean;
  onCancel: () => void;
  onSave: (input: { name: string; targets: string[]; dryRun: boolean }) => Promise<UpdateActionResult>;
}) {
  const t = useTranslations("updates.form");
  const [name, setName] = useState("");
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<Set<string>>(() => new Set());
  const [dryRun, setDryRun] = useState(defaultDryRun);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<Exclude<UpdateActionResult, { ok: true }> | null>(null);
  const devices = useMemo(() => pickableDevices(assets, query), [assets, query]);

  const flip = (id: string) => setPicked((current) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });
  const save = async () => {
    setBusy(true);
    const result = await onSave({ name, targets: [...picked], dryRun });
    setBusy(false);
    setProblem(result.ok ? null : result);
  };

  return (
    <section className="iu-sec" aria-labelledby="iu-form-h">
      <div className="iu-sh"><h2 id="iu-form-h">{t("title")}</h2></div>
      <div className="iu-sheet iu-form">
        <div className="iu-field">
          <label htmlFor="iu-plan-name">{t("name")}</label>
          <input
            id="iu-plan-name"
            className="iu-input"
            value={name}
            autoComplete="off"
            onChange={(e) => setName(e.target.value)}
            aria-invalid={problem?.reason === "name" || undefined}
            aria-describedby={problem?.reason === "name" ? "iu-plan-name-err" : undefined}
          />
          {problem?.reason === "name" ? <p className="iu-hint bad" id="iu-plan-name-err">{t("nameError")}</p> : null}
        </div>
        <div className="iu-field">
          <span className="iu-lbl" id="iu-pick-l">{t("devices")}</span>
          <div className="iu-pick" role="group" aria-labelledby="iu-pick-l" aria-describedby={problem?.reason === "devices" ? "iu-pick-err" : undefined}>
            <label className="iu-filter iu-pick-filter">
              <Search aria-hidden="true" strokeWidth={2} />
              <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("filter")} aria-label={t("filter")} />
            </label>
            <div className="iu-pick-list">
              {devices.map((asset) => {
                const { tone } = deviceState(asset);
                return (
                  <label key={asset.id} className="iu-pk">
                    <input type="checkbox" checked={picked.has(asset.id)} onChange={() => flip(asset.id)} />
                    <span className="iu-nm">{asset.name || asset.id}</span>
                    <span className={`iu-grey${tone ? ` ${tone}` : ""}`}>{asset.metadata?.os_pretty_name ?? ""}</span>
                  </label>
                );
              })}
              {!devices.length ? <p className="iu-quiet iu-pick-none">{query ? t("noMatch") : t("noDevices")}</p> : null}
            </div>
            <p className="iu-pick-foot">{t("selected", { count: picked.size })}</p>
          </div>
          {problem?.reason === "devices" ? <p className="iu-hint bad" id="iu-pick-err">{t("devicesError")}</p> : null}
        </div>
        <div className="iu-opts">
          <div className="iu-opt"><span>{t("what")}</span><span className="iu-c2">{t("osPackages")}</span></div>
          <SwitchRow id="iu-plan-dry" label={t("previewOnly")} on={dryRun} onChange={setDryRun} />
        </div>
        {problem?.reason === "hub" ? (
          <p role="alert" className="iu-err-line">{t("failed")}{problem.detail ? <span className="iu-c3"> {problem.detail}</span> : null}</p>
        ) : null}
        <div className="iu-sheet-foot">
          <Button variant="ghost" onClick={onCancel}>{t("cancel")}</Button>
          <Button variant="primary" disabled={busy} onClick={() => void save()}>{busy ? t("saving") : t("save")}</Button>
        </div>
      </div>
    </section>
  );
}
