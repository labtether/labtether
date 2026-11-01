"use client";

import { useTranslations } from "next-intl";
import { useAuth } from "../../../../contexts/AuthContext";
import { hasAdminRole } from "../../../../lib/roles";
import { useHubSSHKeySetting } from "./useHubSSHKeySetting";

type HubSSHKeyCardProps = {
  nodeId: string;
  isAgentDevice: boolean;
};

/** Settings › Hub SSH key: whether the hub may sign in to this device over SSH as root, as one switch. */
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
    <section className="dv-sec" aria-labelledby="dst-key-t">
      <div className="dv-sh"><h2 id="dst-key-t">{t("title")}</h2></div>
      <div className="dv-sheet dst-key-sheet">
        <div className="dst-switch-row">
          <p className="dst-switch-txt">{t("description")}</p>
          <span className="dst-switch-word">{loading ? t("loading") : enabled ? t("on") : t("off")}</span>
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            aria-label={t("title")}
            disabled={loading || saving || !canChange || setting === null}
            onClick={() => void setEnabled(!enabled)}
            className={`dst-switch${enabled ? " on" : ""}`}
          >
            <i aria-hidden="true" />
          </button>
        </div>
        {status ? <p className="dv-ok-text dst-gap" role="status">{status}</p> : null}
        {!canChange ? <p className="dv-hint dst-gap">{t("adminOnly")}</p> : null}
        {error === "forbidden" ? <p className="dv-err dst-gap">{t("adminOnly")}</p> : null}
        {error === "load" ? <p className="dv-err dst-gap">{t("loadError")}</p> : null}
        {error === "save" ? <p className="dv-err dst-gap">{t("saveError")}</p> : null}
      </div>
    </section>
  );
}
