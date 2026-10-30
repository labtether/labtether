"use client";

import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { useAuth } from "../../../../contexts/AuthContext";
import { hasAdminRole } from "../../../../lib/roles";
import { useHubSSHKeySetting } from "./useHubSSHKeySetting";

type HubSSHKeyCardProps = {
  nodeId: string;
  isAgentDevice: boolean;
};

export function HubSSHKeyCard({ nodeId, isAgentDevice }: HubSSHKeyCardProps) {
  const t = useTranslations("devices.hubSshKey");
  const { user } = useAuth();
  const canChange = hasAdminRole(user?.role);
  const { setting, loading, saving, error, lastChange, setEnabled } = useHubSSHKeySetting(nodeId);
  const enabled = setting?.enabled ?? false;

  let status: string | null = null;
  if (setting?.removal_pending) {
    status = t("removalPending");
  } else if (lastChange === "off" && setting?.removal_sent) {
    status = t("removalSent");
  } else if (lastChange === "off" && !isAgentDevice) {
    status = t("noAgent");
  }

  return (
    <Card className="mt-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-sm font-medium text-[var(--text)]">{t("title")}</h2>
          <p className="mt-1 text-xs text-[var(--muted)]">{t("description")}</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs text-[var(--text)]">{loading ? t("loading") : enabled ? t("on") : t("off")}</span>
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            aria-label={t("title")}
            disabled={loading || saving || !canChange || setting === null}
            onClick={() => void setEnabled(!enabled)}
            className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] disabled:cursor-not-allowed disabled:opacity-60 ${
              enabled ? "bg-[var(--accent)]" : "bg-[var(--line)]"
            }`}
          >
            <span
              className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm transition-transform duration-200 ${
                enabled ? "translate-x-4" : "translate-x-0"
              }`}
            />
          </button>
        </div>
      </div>
      {status ? <p className="mt-2 text-xs text-[var(--text)]">{status}</p> : null}
      {!canChange ? <p className="mt-2 text-xs text-[var(--muted)]">{t("adminOnly")}</p> : null}
      {error === "forbidden" ? <p className="mt-2 text-xs text-[var(--bad)]">{t("adminOnly")}</p> : null}
      {error === "load" ? <p className="mt-2 text-xs text-[var(--bad)]">{t("loadError")}</p> : null}
      {error === "save" ? <p className="mt-2 text-xs text-[var(--bad)]">{t("saveError")}</p> : null}
    </Card>
  );
}
