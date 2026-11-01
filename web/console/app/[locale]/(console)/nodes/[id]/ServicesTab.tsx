"use client";

import { useTranslations } from "next-intl";
import { useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "../../../../components/ui/ConfirmDialog";
import { useServices } from "../../../../hooks/useServices";
import type { ServiceInfo } from "../../../../hooks/useServices";
import { copyText, FilterField, FilterFoot, ListState, plainError, RefreshButton, RowMenu, ShowSwitch, type RowMenuItem } from "./systemViewParts";

type Verb = "start" | "stop" | "restart";
type Show = "all" | "stopped";

const RUNNING = new Set(["active"]);
/** systemd's in-between states and the words for them. */
const BUSY: Record<string, string> = { activating: "starting", deactivating: "stopping", reloading: "reloading", refreshing: "reloading" };
/** While LabTether runs an action on a unit. */
const PENDING: Record<Verb, string> = { start: "starting", stop: "stopping", restart: "restarting" };
const ENABLED = new Set(["enabled", "enabled-runtime", "alias"]);
const KNOWN_ENABLED = new Set(["disabled", "static", "masked", "indirect", "generated", "transient"]);

const notRunning = (svc: ServiceInfo) => !RUNNING.has(svc.active_state.trim().toLowerCase());
const base = (name: string) => name.replace(/\.service$/, "").replace(/@.*/, "");
/** Units whose stop cuts the way in, LabTether itself, or the device's containers: stopping them asks first. */
function stopRisk(name: string): "ssh" | "agent" | "docker" | null {
  const unit = base(name).toLowerCase();
  if (unit === "ssh" || unit === "sshd") return "ssh";
  if (unit.startsWith("labtether-agent")) return "agent";
  if (unit === "docker") return "docker";
  return null;
}

/** The state word and its tone; running units show no word. */
function stateOf(svc: ServiceInfo, pending: Verb | null): { word: string; tone: "" | "bad" | "busy" | "off" } {
  if (pending) return { word: PENDING[pending], tone: "busy" };
  const active = svc.active_state.trim().toLowerCase();
  if (active === "failed") return { word: "failed", tone: "bad" };
  if (BUSY[active]) return { word: BUSY[active], tone: "busy" };
  if (active === "inactive") return { word: "stopped", tone: "off" };
  if (RUNNING.has(active)) {
    const sub = svc.sub_state.trim().toLowerCase();
    return { word: !sub || sub === "running" ? "" : `active:${sub}`, tone: "" };
  }
  return { word: active ? `other:${active}` : "", tone: "off" };
}

/** The device's services (systemd units on Linux): state, enabled at boot, and start, stop and restart from the ⋯. */
export function ServicesTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("devices.system");
  const { services, loading, error, refresh, performAction } = useServices(nodeId);
  const [filter, setFilter] = useState("");
  const [show, setShow] = useState<Show>("all");
  const [busy, setBusy] = useState<{ name: string; verb: Verb } | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [ask, setAsk] = useState<{ name: string; risk: "ssh" | "agent" | "docker" } | null>(null);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const stoppedCount = services.filter(notRunning).length;
  const showing: Show = show === "stopped" && stoppedCount > 0 ? "stopped" : "all";

  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return [...services]
      .filter((svc) => showing === "all" || notRunning(svc))
      .filter((svc) => !q || svc.name.toLowerCase().includes(q) || svc.description.toLowerCase().includes(q) || svc.active_state.toLowerCase().includes(q))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [services, filter, showing]);

  const run = async (name: string, verb: Verb) => {
    setBusy({ name, verb });
    setActionError(null);
    try {
      await performAction(name, verb);
    } catch (err) {
      const detail = plainError(err instanceof Error ? err.message : "");
      setActionError(`${t(`units.failed.${verb}`, { name })}${detail ? `: ${detail}` : ""}`);
    } finally {
      setBusy(null);
    }
  };

  const stateWord = (word: string) => {
    if (!word) return "";
    if (word.startsWith("active:")) return t("units.state.activeSub", { sub: word.slice(7) });
    if (word.startsWith("other:")) return word.slice(6);
    return t(`units.state.${word}`);
  };
  const enabledWord = (enabled: string) => {
    const value = enabled.trim().toLowerCase();
    if (!value || ENABLED.has(value)) return "";
    return KNOWN_ENABLED.has(value) ? t(`units.enabled.${value}`) : value;
  };

  const itemsFor = (svc: ServiceInfo): RowMenuItem[] => {
    const active = svc.active_state.trim().toLowerCase();
    const isActive = RUNNING.has(active);
    const isFailed = active === "failed";
    const disabled = busy !== null;
    const items: RowMenuItem[] = [];
    if (active === "inactive" || isFailed) items.push({ label: t("units.start"), disabled, onSelect: () => void run(svc.name, "start") });
    if (isActive || isFailed) items.push({ label: t("units.restart"), disabled, onSelect: () => void run(svc.name, "restart") });
    if (isActive) {
      const risk = stopRisk(svc.name);
      items.push(risk
        ? { label: t("units.stopAsk"), danger: true, disabled, onSelect: () => setAsk({ name: svc.name, risk }) }
        : { label: t("units.stop"), disabled, onSelect: () => void run(svc.name, "stop") });
    }
    if (items.length) items.push("sep");
    items.push({ label: t("units.copyName"), onSelect: () => copyText(svc.name) });
    return items;
  };

  return (
    <div className="dsy">
      <div className="dv-bar">
        <FilterField value={filter} onChange={setFilter} placeholder={t("units.filter")} />
        <ShowSwitch<Show> items={stoppedCount > 0 ? [["all", t("units.show.all")], ["stopped", t("units.show.stopped")]] : []} value={showing} onChange={setShow} label={t("units.show.label")} />
        <span className="dv-sp" />
        <RefreshButton loading={loading} onClick={() => void refresh()} />
      </div>
      {actionError ? <p className="dv-err dsy-msg" role="alert">{actionError}</p> : null}
      <ListState
        loading={loading}
        hasRows={services.length > 0}
        error={error}
        loadError={t("units.loadError")}
        empty={t("units.empty")}
        onRetry={() => void refresh()}
      >
        <div className="dv-list dsy-hang dsy-unit" role="table" aria-label={t("units.label")}>
          <div className="dv-cols" role="row">
            <span role="columnheader">{t("units.cols.service")}</span>
            <span role="columnheader">{t("units.cols.state")}</span>
            <span role="columnheader" className="c-en">{t("units.cols.enabled")}</span>
            <span role="columnheader" aria-label={t("actions")} />
          </div>
          {rows.map((svc) => {
            const pending = busy?.name === svc.name ? busy.verb : null;
            const state = stateOf(svc, pending);
            const load = svc.load_state.trim().toLowerCase();
            return (
              <div key={svc.name} className="dv-row" role="row" tabIndex={0}>
                <span className="dsy-lead" aria-hidden="true">{state.tone ? <i className={`dv-dot ${state.tone}`} /> : null}</span>
                <span role="cell" className="dv-ln">
                  <span className="dv-nm">{svc.name}</span>
                  {load && load !== "loaded" ? <span className="dv-prob warn">{load}</span> : null}
                  {svc.description ? <span className="dv-grey">{svc.description}</span> : null}
                </span>
                <span role="cell" className={`dsy-word${state.tone === "bad" ? " bad" : ""}`}>
                  {stateWord(state.word)}
                </span>
                <span role="cell" className="dv-txt c-en">{enabledWord(svc.enabled)}</span>
                <RowMenu name={svc.name} items={itemsFor(svc)} />
              </div>
            );
          })}
          {rows.length === 0 ? <div className="dv-empty"><b>{t("units.noMatch")}</b></div> : null}
          <FilterFoot shown={rows.length} total={services.length} onClear={() => { setFilter(""); setShow("all"); }} />
        </div>
      </ListState>
      <ConfirmDialog
        open={ask !== null}
        onClose={() => setAsk(null)}
        onConfirm={() => { const target = ask; setAsk(null); if (target) void run(target.name, "stop"); }}
        title={ask ? t("units.stopTitle", { name: ask.name }) : ""}
        message={ask ? t(`units.stopRisk.${ask.risk}`) : ""}
        confirmLabel={t("units.stop")}
        danger
      />
    </div>
  );
}
