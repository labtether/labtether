"use client";

import { useTranslations } from "next-intl";
import { useEffect, useMemo, useState } from "react";
import { Button } from "../../../../components/ui/Button";
import { ConfirmDialog } from "../../../../components/ui/ConfirmDialog";
import { usePackages } from "../../../../hooks/usePackages";
import { copyText, FilterField, FilterFoot, ListState, plainError, RefreshButton, RowMenu } from "./systemViewParts";

type PackagesTabProps = {
  nodeId: string;
  backend?: string;
};

type PackageAction = "install" | "remove" | "upgrade";
type Ask = { action: PackageAction; names: string[] } | null;

const parseNames = (input: string) => input.split(/[,\s]+/).map((value) => value.trim()).filter((value) => value.length > 0);

/** Installed packages with install, remove and upgrade through the device's package manager. */
export function PackagesTab({ nodeId, backend = "" }: PackagesTabProps) {
  const t = useTranslations("devices.system");
  const { packages, loading, error, refresh, performAction } = usePackages(nodeId);
  const [filter, setFilter] = useState("");
  const [action, setAction] = useState<PackageAction>("install");
  const [packageInput, setPackageInput] = useState("");
  const [actionBusy, setActionBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionOutput, setActionOutput] = useState<string | null>(null);
  const [rebootRequired, setRebootRequired] = useState(false);
  const [ask, setAsk] = useState<Ask>(null);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const rows = useMemo(() => {
    const sorted = [...packages].sort((a, b) => a.name.localeCompare(b.name));
    const q = filter.trim().toLowerCase();
    if (!q) return sorted;
    return sorted.filter((pkg) => pkg.name.toLowerCase().includes(q) || pkg.version.toLowerCase().includes(q) || pkg.status.toLowerCase().includes(q));
  }, [packages, filter]);

  const backendLabel = backend.trim().toLowerCase() === "brew" ? "Homebrew" : backend.trim();

  /** names empty with upgrade: every package with a newer version. */
  const runAction = async (selected: PackageAction, names: string[]) => {
    if ((selected === "install" || selected === "remove") && names.length === 0) {
      setActionError(t("packages.needNames"));
      return;
    }
    setActionBusy(true);
    setActionError(null);
    setActionOutput(null);
    setRebootRequired(false);
    try {
      const result = await performAction(selected, names);
      setActionOutput(result.output?.trim() || t("packages.done"));
      setRebootRequired(Boolean(result.reboot_required));
      if (selected !== "upgrade") setPackageInput("");
    } catch (err) {
      const detail = plainError(err instanceof Error ? err.message : "");
      setActionError(`${t("packages.failed")}${detail ? `: ${detail}` : ""}`);
    } finally {
      setActionBusy(false);
    }
  };

  const submit = () => {
    const names = parseNames(packageInput);
    if (action === "remove" && names.length > 0) setAsk({ action, names });
    else void runAction(action, names);
  };

  const askTitle = !ask ? "" : ask.names.length === 0
    ? t("packages.upgradeAllTitle")
    : t("packages.removeTitle", { names: ask.names.join(", ") });

  return (
    <div className="dsy">
      <div className="dv-bar">
        <FilterField value={filter} onChange={setFilter} placeholder={t("packages.filter")} />
        <span className="dv-sp" />
        <Button size="sm" disabled={actionBusy} onClick={() => setAsk({ action: "upgrade", names: [] })}>{t("packages.upgradeAll")}</Button>
        <RefreshButton loading={loading} onClick={() => void refresh()} />
      </div>

      <form className="dv-bar dsy-pkform" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <input
          className="dv-input"
          type="text"
          value={packageInput}
          onChange={(e) => setPackageInput(e.target.value)}
          placeholder={t(action === "upgrade" ? "packages.namesUpgrade" : "packages.names")}
          aria-label={t("packages.namesLabel")}
          autoComplete="off"
          spellCheck={false}
        />
        <select className="dv-input" value={action} onChange={(e) => setAction(e.target.value as PackageAction)} aria-label={t("packages.actionLabel")}>
          <option value="install">{t("packages.install")}</option>
          <option value="remove">{t("packages.remove")}</option>
          <option value="upgrade">{t("packages.upgrade")}</option>
        </select>
        <Button size="sm" type="submit" disabled={actionBusy}>{actionBusy ? t("packages.running") : t(`packages.${action}`)}</Button>
      </form>

      {actionError ? <p className="dv-err dsy-msg" role="alert">{actionError}</p> : null}
      {rebootRequired ? <p className="dv-prob warn dsy-msg">{t("packages.reboot")}</p> : null}
      {actionOutput ? <pre className="dsy-out" aria-label={t("packages.output")}>{actionOutput}</pre> : null}

      <ListState
        loading={loading}
        hasRows={packages.length > 0}
        error={error}
        loadError={t("packages.loadError")}
        empty={t("packages.empty")}
        emptyHint={backendLabel ? t("packages.backend", { name: backendLabel }) : undefined}
        onRetry={() => void refresh()}
      >
        <div className="dv-list dsy-pkg" role="table" aria-label={t("packages.label")}>
          <div className="dv-cols" role="row">
            <span role="columnheader">{t("packages.cols.package")}</span>
            <span role="columnheader" className="c-ver">{t("packages.cols.version")}</span>
            <span role="columnheader" aria-label={t("actions")} />
          </div>
          {rows.map((pkg) => {
            const status = pkg.status.trim();
            return (
              <div key={`${pkg.name}-${pkg.version}`} className="dv-row" role="row" tabIndex={0}>
                <span role="cell" className="dv-ln">
                  <span className="dv-nm">{pkg.name}</span>
                  {status && status.toLowerCase() !== "installed" ? <span className="dv-grey">{status}</span> : null}
                </span>
                <span role="cell" className="dv-txt c-ver" title={pkg.version || undefined}>{pkg.version || "—"}</span>
                <RowMenu
                  name={pkg.name}
                  items={[
                    { label: t("packages.upgrade"), disabled: actionBusy, onSelect: () => void runAction("upgrade", [pkg.name]) },
                    { label: t("packages.copyName"), onSelect: () => copyText(pkg.name) },
                    "sep",
                    { label: t("packages.removeAsk"), danger: true, disabled: actionBusy, onSelect: () => setAsk({ action: "remove", names: [pkg.name] }) },
                  ]}
                />
              </div>
            );
          })}
          {rows.length === 0 ? <div className="dv-empty"><b>{t("packages.noMatch")}</b></div> : null}
          <FilterFoot shown={rows.length} total={packages.length} onClear={() => setFilter("")} />
        </div>
      </ListState>

      <ConfirmDialog
        open={ask !== null}
        onClose={() => setAsk(null)}
        onConfirm={() => { const target = ask; setAsk(null); if (target) void runAction(target.action, target.names); }}
        title={askTitle}
        message={ask?.names.length === 0 ? t("packages.upgradeAllBody") : t("packages.removeBody")}
        confirmLabel={ask?.action === "remove" ? t("packages.remove") : t("packages.upgrade")}
        danger={ask?.action === "remove"}
      />
    </div>
  );
}
