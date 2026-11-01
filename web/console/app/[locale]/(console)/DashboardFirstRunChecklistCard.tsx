"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Check } from "lucide-react";
import { Link } from "../../../i18n/navigation";
import { useFastStatus } from "../../contexts/StatusContext";
import { useHubUsers } from "../../hooks/useHubUsers";
import { COMMUNITY_URL, GITHUB_REPO_URL, MCP_GATEWAY_DOCS_URL } from "../../lib/externalLinks";
import { checklistSteps, type ChecklistStep } from "../../components/AddDeviceModal/firstRunSteps";

const DISMISS_KEY = "labtether.dashboard.firstRun.dismissed";
// Set the first time this browser sees the hub with no devices: a lab that already had devices skips the checklist.
const STARTED_KEY = "labtether.dashboard.firstRun.started";

type DashboardFirstRunChecklistCardProps = {
  loading: boolean;
  hasError: boolean;
  deviceCount: number;
  connectorCount: number;
  recentCommandCount: number;
  recentLogCount: number;
  canWrite: boolean;
  canAdmin: boolean;
  onAddDevice: () => void;
};

/** Notification channels exist (admins only); null until known. */
function useHasChannels(enabled: boolean): boolean | null {
  const [has, setHas] = useState<boolean | null>(null);
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    fetch("/api/notifications/channels?limit=1&offset=0", { cache: "no-store", signal: controller.signal })
      .then((r) => (r.ok ? r.json() : null))
      .then((payload: { channels?: unknown[] } | null) => setHas(Array.isArray(payload?.channels) && payload.channels.length > 0))
      .catch(() => undefined);
    return () => controller.abort();
  }, [enabled]);
  return has;
}

/**
 * The Dashboard's first-run checklist: the mascot, then five steps that tick themselves from what the hub reports
 * (a device with the agent, a system connected through its API, a terminal command, a notification channel, a second
 * person). The next open step's button is the page's one rose button. It shows from a lab's first empty Dashboard
 * until Dismiss, which hides it for good on this browser.
 */
export function DashboardFirstRunChecklistCard({
  loading,
  hasError,
  deviceCount,
  connectorCount,
  recentCommandCount,
  canWrite,
  canAdmin,
  onAddDevice,
}: DashboardFirstRunChecklistCardProps) {
  const t = useTranslations("auth.firstRun");
  const [dismissed, setDismissed] = useState(true);
  const [started, setStarted] = useState(false);
  const fast = useFastStatus();

  useEffect(() => {
    try {
      setDismissed(window.localStorage.getItem(DISMISS_KEY) === "true");
      setStarted(window.localStorage.getItem(STARTED_KEY) === "true");
    } catch {
      setDismissed(false);
    }
  }, []);

  useEffect(() => {
    if (loading || hasError || deviceCount > 0 || !fast) return;
    setStarted(true);
    try {
      window.localStorage.setItem(STARTED_KEY, "true");
    } catch {
      // Without storage the checklist shows while the lab is empty.
    }
  }, [deviceCount, fast, hasError, loading]);

  const active = started && !dismissed && !loading && !hasError;
  const { users } = useHubUsers(active && canAdmin);
  const hasChannels = useHasChannels(active && canAdmin);
  const steps = useMemo(
    () => checklistSteps({ assets: fast?.assets ?? [], connectorCount, recentCommandCount, hasChannels: Boolean(hasChannels), users, canWrite, canAdmin }),
    [fast?.assets, connectorCount, recentCommandCount, hasChannels, users, canWrite, canAdmin],
  );

  if (!active || steps.length === 0) return null;

  const dismiss = () => {
    setDismissed(true);
    try {
      window.localStorage.setItem(DISMISS_KEY, "true");
    } catch {
      // Private windows can refuse storage; the checklist then hides until the page reloads.
    }
  };
  const next = steps.find((s) => !s.done && s.action);
  const allDone = steps.every((s) => s.done);

  const button = (s: ChecklistStep) => {
    const cls = `si-btn${s === next ? " primary" : ""}`;
    if (s.action === "add") return <button type="button" className={cls} onClick={onAddDevice}>{t(`steps.${s.id}.button`)}</button>;
    if (s.href) return <Link href={s.href} className={cls}>{t(`steps.${s.id}.button`)}</Link>;
    return null;
  };

  return (
    <section className="si-fr" aria-labelledby="si-fr-t">
      <img src="/logo.svg" alt="" width={64} height={64} aria-hidden="true" className="si-fr-logo" />
      <div className="si-fr-h">
        <h2 id="si-fr-t">
          {t("title")}
          {allDone ? <span className="si-fr-done"> · {t("done")}</span> : null}
        </h2>
        <button type="button" className="si-btn quiet" onClick={dismiss}>{t("dismiss")}</button>
      </div>
      <ol className="si-fr-list">
        {steps.map((s) => (
          <li key={s.id} className={`si-fr-step${s.done ? " done" : ""}`}>
            <span className="si-tick" aria-hidden="true">{s.done ? <Check strokeWidth={2.6} /> : null}</span>
            <div className="si-fr-tx">
              <span className="si-fr-nm">{t(`steps.${s.id}.label`)}</span>
              <span className="sr-only">{s.done ? t("isDone") : t("notDone")}</span>
              {s.sub ? <p className="si-fr-sub">{t(`steps.${s.id}.sub`, { name: s.sub })}</p> : null}
            </div>
            {s.done ? null : button(s)}
          </li>
        ))}
      </ol>
      {allDone ? (
        <p className="si-fr-more">
          <a href={MCP_GATEWAY_DOCS_URL} target="_blank" rel="noopener noreferrer">{t("mcp")}</a>
          <a href={COMMUNITY_URL} target="_blank" rel="noopener noreferrer">{t("community")}</a>
          <a href={GITHUB_REPO_URL} target="_blank" rel="noopener noreferrer">{t("github")}</a>
        </p>
      ) : null}
    </section>
  );
}
