"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { Button } from "../../../../components/ui/Button";
import { useCollectorSettings } from "../../../../hooks/useCollectorSettings";
import { CollectorForm } from "./settingsViewCollectorForm";

/**
 * Settings › Proxmox API: how the hub's collector reaches this device, as key-value rows with Edit…, Test and
 * Run now; Edit… opens the form in place (a collector that is not set up opens on the form).
 */
export function SettingsTab({ collectorId, loading }: { collectorId: string | null; loading: boolean }) {
  const t = useTranslations("devices.settingsTab.collector");
  const s = useCollectorSettings(collectorId);
  const [editing, setEditing] = useState(false);
  const wasSaving = useRef(false);

  // Close the form once a save finishes without an error.
  useEffect(() => {
    if (wasSaving.current && !s.saving && !s.error) setEditing(false);
    wasSaving.current = s.saving;
  }, [s.saving, s.error]);

  const busy = loading || s.loading;
  const formOpen = editing || (!busy && !!collectorId && !s.configured);
  const tls = s.skipVerify ? t("tlsSkip") : s.caPEM.trim() ? t("tlsCa") : t("tlsVerify");
  const who = s.authMethod === "api_token" ? s.tokenID : s.username;

  return (
    <section className="dv-sec" aria-labelledby="dst-col-t">
      <div className="dv-sh"><h2 id="dst-col-t">{t("title")}</h2></div>
      {busy ? (
        <div className="dv-sheet"><p className="dv-hint">{t("loading")}</p></div>
      ) : !collectorId ? (
        <div className="dv-sheet">
          <div className="dv-empty">
            <b>{t("noneTitle")}</b>
            <span>{t("noneHint")}</span>
            <Link href="/settings" className="dv-link dv-empty-act">{t("openSettings")}</Link>
          </div>
        </div>
      ) : (
        <>
          {s.error ? <p className="dv-err dst-msg-line" role="alert">{s.error}</p> : null}
          {formOpen ? (
            <CollectorForm s={s} onCancel={() => { setEditing(false); void s.load(); }} />
          ) : (
            <div className="dv-sheet tight">
              <dl className="dv-kv">
                <div><dt>{t("url")}</dt><dd><span>{s.baseURL || "—"}</span></dd></div>
                <div>
                  <dt>{s.authMethod === "api_token" ? t("tokenId") : t("username")}</dt>
                  <dd><span className={s.authMethod === "api_token" ? "dv-mono" : undefined}>{who || "—"}</span></dd>
                </div>
                {s.credentialName ? <div><dt>{t("credential")}</dt><dd><span>{s.credentialName}</span></dd></div> : null}
                {s.clusterName ? <div><dt>{t("clusterName")}</dt><dd><span>{s.clusterName}</span></dd></div> : null}
                {s.baseURL.startsWith("https") ? <div><dt>{t("tls")}</dt><dd><span>{tls}</span></dd></div> : null}
                <div><dt>{t("pollShort")}</dt><dd><span>{t("every", { seconds: s.intervalSeconds })}</span></dd></div>
              </dl>
              <div className="dv-sheet-foot">
                {s.message ? <span className="dv-ok-text dst-grow-l" role="status">{s.message}</span> : null}
                <Button size="sm" onClick={() => setEditing(true)}>{t("edit")}</Button>
                <Button size="sm" disabled={s.testing || s.saving} onClick={() => void s.testConnection()}>
                  {s.testing ? t("testing") : t("test")}
                </Button>
                <Button size="sm" disabled={s.running || s.testing || s.saving} onClick={() => void s.runNow()}>
                  {s.running ? t("running") : t("runNow")}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </section>
  );
}
