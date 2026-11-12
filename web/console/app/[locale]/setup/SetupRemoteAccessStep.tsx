"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../components/ui/Button";
import type { useTailscaleServeStatus } from "../../hooks/useTailscaleServeStatus";
import { defaultRemoteAccessWay, type RemoteAccessWay } from "./remoteAccessWay";

type Status = ReturnType<typeof useTailscaleServeStatus>["status"];

export type SetupRemoteAccessStepProps = {
  tailscaleStatus: Status;
  tailscaleLoading: boolean;
  tailscaleError: ReturnType<typeof useTailscaleServeStatus>["error"];
  refreshTailscaleStatus: () => void;
  copied: string;
  copyToClipboard: (text: string, label: string) => void;
  managedActionLoading: "" | "apply" | "disable";
  managedActionMessage: string;
  managedActionError: string;
  runManagedTailscaleAction: (action: "apply" | "disable") => Promise<void>;
  continueToConsole: (mode: "serve" | "off") => Promise<void>;
};

/** What the host reports for Tailscale itself: its own state word, or ours. */
function tailscaleWord(t: ReturnType<typeof useTranslations>, s: Status, loading: boolean) {
  if (!s) return loading ? t("remoteAccess.checking") : t("remoteAccess.unavailable");
  if (!s.tailscale_installed) return t("remoteAccess.notInstalled");
  return s.backend_state || (s.logged_in ? t("remoteAccess.connected") : t("remoteAccess.loginRequired"));
}

function httpsWord(t: ReturnType<typeof useTranslations>, s: Status, loading: boolean) {
  if (!s) return loading ? t("remoteAccess.checking") : t("remoteAccess.unavailable");
  if (s.serve_configured) return t("remoteAccess.on");
  if (s.serve_status === "login_required") return t("remoteAccess.loginRequired");
  if (s.serve_status === "not_installed") return t("remoteAccess.notInstalled");
  return t("remoteAccess.notSetUp");
}

/** Remote access: Tailscale HTTPS (the host runs `tailscale serve`; Check reads its state) or the hub's own address. */
export function SetupRemoteAccessStep({
  tailscaleStatus: s,
  tailscaleLoading,
  tailscaleError,
  refreshTailscaleStatus,
  copied,
  copyToClipboard,
  managedActionLoading,
  managedActionMessage,
  managedActionError,
  runManagedTailscaleAction,
  continueToConsole,
}: SetupRemoteAccessStepProps) {
  const t = useTranslations("auth");
  // The person's pick, once made; until then the choice follows what the hub reports for Tailscale.
  const [picked, setPicked] = useState<RemoteAccessWay | null>(null);
  const way = picked ?? defaultRemoteAccessWay(s);
  const [finishing, setFinishing] = useState(false);
  const ready = way === "own" || Boolean(s?.serve_configured);
  const origin = typeof window === "undefined" ? "" : window.location.origin;

  const choice = (id: RemoteAccessWay, name: string, sub: string) => (
    <button type="button" role="radio" aria-checked={way === id} className={`si-way${way === id ? " on" : ""}`} onClick={() => setPicked(id)}>
      <span className="si-radio" aria-hidden="true" />
      <span className="si-wt">
        <span className="si-wn">{name}</span>
        <span className="si-ws">{sub}</span>
      </span>
    </button>
  );

  const finish = async () => {
    setFinishing(true);
    try {
      await continueToConsole(way === "tailscale" ? "serve" : "off");
    } finally {
      setFinishing(false);
    }
  };

  return (
    <div className="si-form">
      <div className="si-ways" role="radiogroup" aria-label={t("remoteAccess.title")}>
        {choice("tailscale", t("remoteAccess.tailscaleHttps"), t("remoteAccess.tailscaleHttpsSub"))}
        {choice("own", t("remoteAccess.ownAddress"), origin)}
      </div>

      {way === "tailscale" ? (
        <div className="si-ts">
          <dl className="si-kv">
            <div><dt>{t("remoteAccess.tailscale")}</dt><dd>{tailscaleWord(t, s, tailscaleLoading)}</dd></div>
            <div><dt>{t("remoteAccess.https")}</dt><dd>{httpsWord(t, s, tailscaleLoading)}</dd></div>
            {s?.tsnet_url ? (
              <div>
                <dt>{t("remoteAccess.address")}</dt>
                <dd className="si-addr">
                  <code>{s.tsnet_url}</code>
                  <Button variant="ghost" size="sm" onClick={() => copyToClipboard(s.tsnet_url!, "setup-url")}>
                    {copied === "setup-url" ? t("copied") : t("copy")}
                  </Button>
                </dd>
              </div>
            ) : null}
          </dl>
          {tailscaleError && !s ? <p className="si-err" role="alert">{tailscaleError}</p> : null}
          {s?.status_note ? <p className="si-hint">{s.status_note}</p> : null}

          {!s?.serve_configured && s?.suggested_command && !s.can_manage ? (
            <div className="si-hint">
              <span>{t("remoteAccess.onHost")}</span>
              <span className="si-cmd">
                <code>{s.suggested_command}</code>
                <Button variant="ghost" size="sm" onClick={() => copyToClipboard(s.suggested_command!, "setup-command")}>
                  {copied === "setup-command" ? t("copied") : t("copy")}
                </Button>
              </span>
            </div>
          ) : null}
          {!s?.serve_configured && !s?.suggested_command && s && !s.can_manage ? (
            <p className="si-hint">{t("remoteAccess.installFirst")}</p>
          ) : null}

          <div className="si-acts">
            {s?.can_manage ? (
              <Button loading={managedActionLoading === "apply"} onClick={() => void runManagedTailscaleAction("apply")}>
                {s.serve_configured ? t("remoteAccess.reapplyHttps") : t("remoteAccess.enableHttps")}
              </Button>
            ) : null}
            <Button variant={s?.can_manage ? "ghost" : "secondary"} loading={tailscaleLoading && Boolean(s)} onClick={refreshTailscaleStatus}>
              {t("remoteAccess.check")}
            </Button>
            {s?.can_manage && s.serve_configured ? (
              <Button variant="ghost" loading={managedActionLoading === "disable"} onClick={() => void runManagedTailscaleAction("disable")}>
                {t("remoteAccess.disableHttps")}
              </Button>
            ) : null}
          </div>
          {managedActionMessage ? <p className="si-hint" role="status">{managedActionMessage}</p> : null}
          {managedActionError ? <p className="si-err" role="alert">{managedActionError}</p> : null}
        </div>
      ) : (
        <p className="si-hint">{t("remoteAccess.ownNote")}</p>
      )}

      <Button variant="primary" className="si-go" disabled={!ready || finishing} onClick={() => void finish()}>
        {t("remoteAccess.finish")}
      </Button>
    </div>
  );
}
