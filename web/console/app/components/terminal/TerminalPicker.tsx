"use client";

import { ChevronRight, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import type { Asset } from "../../console/models";
import { buildTargetLists, looksLikeAddress, type TerminalTarget } from "./terminalTargets";
import { loadRecentTargets, saveRecentTarget } from "./terminalRecentTargets";
import { useTerminalWhen } from "./useTerminalWhen";

type TerminalPickerProps = {
  assets: Asset[];
  connectedAgentIds: ReadonlySet<string>;
  /** Opens a shell on the device or container. */
  onPick: (id: string) => void;
  /** "Connect to root@10.20.0.50": opens the address form with it. */
  onAddress: (address: string) => void;
  /** Listed first in All devices as "(this device)". */
  firstId?: string;
  /** Above Recent (running sessions), and after the devices (saved connections, ended sessions). */
  before?: ReactNode;
  after?: ReactNode;
  query?: string;
  onQueryChange?: (query: string) => void;
  autoFocus?: boolean;
  /** Inside an empty pane: smaller field, no section gap. */
  compact?: boolean;
};

const rowsIn = (root: HTMLElement | null) => Array.from(root?.querySelectorAll<HTMLElement>("[data-tm-row]") ?? []);

/**
 * The page Terminal opens on when nothing is open, and the list an empty pane shows: "Connect to…" filters, Enter opens
 * the first match, ↓ moves into the list. Recent sits above every device; offline devices and containers stay folded.
 */
export default function TerminalPicker({
  assets, connectedAgentIds, onPick, onAddress, firstId = "", before, after, query: controlled, onQueryChange, autoFocus, compact,
}: TerminalPickerProps) {
  const t = useTranslations("terminal.picker");
  const when = useTerminalWhen();
  const [own, setOwn] = useState("");
  const query = controlled ?? own;
  const setQuery = onQueryChange ?? setOwn;
  const [recentIds, setRecentIds] = useState<string[]>([]);
  const [openFolds, setOpenFolds] = useState<Record<string, boolean>>({});
  const rootRef = useRef<HTMLDivElement | null>(null);
  const fieldRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => setRecentIds(loadRecentTargets().map((r) => r.id)), []);
  useEffect(() => {
    if (!autoFocus) return undefined;
    const timer = window.setTimeout(() => {
      const active = document.activeElement;
      if (!active || active === document.body) fieldRef.current?.focus({ preventScroll: true });
    }, 0);
    return () => window.clearTimeout(timer);
  }, [autoFocus]);

  const lists = useMemo(
    () => buildTargetLists({ assets, connectedAgentIds, recentIds, query, firstId }),
    [assets, connectedAgentIds, recentIds, query, firstId],
  );
  const address = looksLikeAddress(query) ? query.trim() : "";

  const pick = (target: TerminalTarget) => {
    saveRecentTarget({
      id: target.asset.id,
      name: target.asset.name || target.asset.id,
      type: target.container ? "container" : "device",
      lastConnected: new Date().toISOString(),
    });
    onPick(target.asset.id);
  };

  const grey = (target: TerminalTarget) => {
    if (target.state === "offline") return { tone: "bad", text: t("offlineSince", { time: when(target.asset.last_seen_at) }) };
    if (target.state === "unresponsive") return { tone: "warn", text: t("notResponding", { time: when(target.asset.last_seen_at) }) };
    const where = target.host ? (target.vmid ? t("vmOn", { vmid: target.vmid, host: target.host }) : t("on", { host: target.host })) : "";
    return { tone: "", text: [target.os, where].filter(Boolean).join(" · ") };
  };
  const how = (target: TerminalTarget) => (target.via === "ssh" ? t("ssh") : target.via === "proxmox" ? t("proxmox") : "");

  const row = (target: TerminalTarget, section: string) => {
    const g = grey(target);
    const ip = target.asset.metadata?.ip_address || target.asset.metadata?.ip || "";
    return (
      <button
        type="button"
        key={`${section}-${target.asset.id}`}
        className="tm-row tm-prow"
        data-tm-row=""
        title={[target.asset.name || target.asset.id, ip].filter(Boolean).join(" · ")}
        onClick={() => pick(target)}
      >
        <span className="tm-lead" aria-hidden="true">{g.tone ? <i className={`kt-dot sm ${g.tone}`} /> : null}</span>
        <span className="tm-main">
          <span className="tm-nm">{target.asset.name || target.asset.id}</span>
          {target.asset.id === firstId ? <span className="tm-this">{t("thisDevice")}</span> : null}
          <span className={`tm-grey${g.tone ? ` ${g.tone}` : ""}`}>{g.text}</span>
        </span>
        <span className="tm-how">{how(target)}</span>
        <ChevronRight className="tm-chev" aria-hidden="true" />
      </button>
    );
  };

  const fold = (key: "containers" | "offline", items: TerminalTarget[]) => {
    if (items.length === 0) return null;
    const shown = openFolds[key] || query.trim() !== "";
    return (
      <section className="tm-sec">
        <button type="button" className="tm-h tm-fold" aria-expanded={shown} onClick={() => setOpenFolds((f) => ({ ...f, [key]: !f[key] }))}>
          <ChevronRight aria-hidden="true" className={shown ? "open" : undefined} />
          {t(key)}
          <span className="tm-n">{items.length}</span>
        </button>
        {shown ? <div className="tm-list">{items.map((item) => row(item, key))}</div> : null}
      </section>
    );
  };

  const onFieldKey = (event: KeyboardEvent<HTMLInputElement>) => {
    const first = rowsIn(rootRef.current)[0];
    if (event.key === "Enter" && first) {
      event.preventDefault();
      first.click();
    } else if (event.key === "ArrowDown" && first) {
      event.preventDefault();
      first.focus();
    } else if (event.key === "Escape" && query) {
      event.preventDefault();
      setQuery("");
    }
  };
  const onListKey = (event: KeyboardEvent<HTMLDivElement>) => {
    const rows = rowsIn(rootRef.current);
    const at = rows.indexOf(document.activeElement as HTMLElement);
    if (at < 0) return;
    const down = event.key === "ArrowDown" || event.key === "j";
    const up = event.key === "ArrowUp" || event.key === "k";
    if (!down && !up) return;
    event.preventDefault();
    if (up && at === 0) fieldRef.current?.focus();
    else rows[Math.max(0, Math.min(rows.length - 1, at + (down ? 1 : -1)))]?.focus();
  };

  const nothing = lists.devices.length + lists.containers.length + lists.offline.length === 0;
  return (
    <div ref={rootRef} className={`tm-pick${compact ? " compact" : ""}`} onKeyDown={onListKey}>
      <div className="tm-pick-bar">
        <label className="tm-q">
          <Search aria-hidden="true" />
          <span className="sr-only">{t("search")}</span>
          <input
            ref={fieldRef}
            type="search"
            value={query}
            placeholder={t("search")}
            autoComplete="off"
            spellCheck={false}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={onFieldKey}
          />
        </label>
      </div>
      {before}
      {lists.recent.length > 0 ? (
        <section className="tm-sec" aria-label={t("recent")}>
          <h2 className="tm-h">{t("recent")}</h2>
          <div className="tm-list">{lists.recent.map((item) => row(item, "r"))}</div>
        </section>
      ) : null}
      {lists.devices.length > 0 ? (
        <section className="tm-sec" aria-label={t("devices")}>
          {lists.recent.length > 0 ? <h2 className="tm-h">{t("allDevices")}</h2> : null}
          <div className="tm-list">{lists.devices.map((item) => row(item, "a"))}</div>
        </section>
      ) : null}
      {address ? (
        <section className="tm-sec">
          <div className="tm-list">
            <button type="button" className="tm-row tm-prow" data-tm-row="" onClick={() => onAddress(address)}>
              <span className="tm-lead" />
              <span className="tm-main">
                <span className="tm-nm">{t("connectTo", { address })}</span>
                <span className="tm-grey">{t("ssh")}</span>
              </span>
              <span className="tm-how" />
              <ChevronRight className="tm-chev" aria-hidden="true" />
            </button>
          </div>
        </section>
      ) : null}
      {nothing && !address ? <p className="tm-none">{query.trim() ? t("noMatch", { query: query.trim() }) : t("none")}</p> : null}
      {fold("containers", lists.containers)}
      {after}
      {fold("offline", lists.offline)}
    </div>
  );
}
