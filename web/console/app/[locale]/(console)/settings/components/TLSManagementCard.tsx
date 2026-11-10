"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { useHubCAFingerprint } from "../../../../hooks/useHubCAFingerprint";
import { useTailscaleServeStatus } from "../../../../hooks/useTailscaleServeStatus";
import { groupedFingerprint, plainFingerprint } from "../../../../lib/fingerprint";
import { CopyButton, Facts, Field, Message, Row, SettingsDialog } from "./SettingsKit";
import { formatWhen } from "./settingsModel";

type TLSCertificateMetadata = {
  subject_common_name?: string;
  subject_summary?: string;
  issuer_summary?: string;
  expires_at?: string;
  fingerprint_sha256?: string;
  dns_names?: string[];
};

type TLSSettingsResponse = {
  tls_enabled: boolean;
  tls_mode: string;
  tls_source: string;
  cert_type: string;
  ca_available: boolean;
  active_certificate?: TLSCertificateMetadata;
  uploaded_override_present: boolean;
  uploaded_certificate?: TLSCertificateMetadata;
  uploaded_updated_at?: string;
  can_upload: boolean;
  restart_required: boolean;
  restart_action_available: boolean;
  restart_action_note?: string;
  error?: string;
};

const SOURCE_KEYS: Record<string, string> = {
  built_in: "builtIn", tailscale: "tailscale", ui_uploaded: "uploaded", deployment_external: "external", disabled: "off",
};
const TYPE_KEYS: Record<string, string> = {
  "self-signed": "selfSigned", tailscale: "tailscale", uploaded: "uploaded", external: "external",
};

/** Days until a certificate expires, rounded down; null when unknown. */
function daysLeft(iso?: string): number | null {
  if (!iso) return null;
  const ms = new Date(iso).getTime() - Date.now();
  return Number.isFinite(ms) ? Math.floor(ms / 86_400_000) : null;
}

/** The HTTPS certificate row of Security, with its details, upload form and restart behind Show. */
export function TLSManagementCard() {
  const t = useTranslations("security");
  const locale = useLocale();
  const caFingerprint = useHubCAFingerprint();
  const [details, setDetails] = useState<TLSSettingsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [certPEM, setCertPEM] = useState("");
  const [keyPEM, setKeyPEM] = useState("");
  const [submitting, setSubmitting] = useState<"" | "upload" | "clear" | "restart">("");
  const { status: tailscaleStatus } = useTailscaleServeStatus();

  const request = useCallback(async (init: RequestInit | undefined, kind: "" | "upload" | "clear", fallback: string) => {
    if (kind) setSubmitting(kind);
    setError("");
    if (kind) setMessage("");
    try {
      const response = await fetch("/api/settings/tls", { cache: "no-store", ...init });
      const payload = await response.json().catch(() => null) as TLSSettingsResponse | null;
      if (!response.ok) throw new Error(payload?.error || fallback);
      setDetails(payload ?? null);
      return payload;
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : fallback);
      return null;
    } finally {
      if (kind) setSubmitting("");
      setLoading(false);
    }
  }, []);

  const loadFailed = t("tls.loadFailed");
  const loadFailedRef = useRef(loadFailed);
  loadFailedRef.current = loadFailed;
  useEffect(() => {
    void request(undefined, "", loadFailedRef.current);
  }, [request]);

  const applyUploadedCertificate = async () => {
    const payload = await request({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cert_pem: certPEM, key_pem: keyPEM }),
    }, "upload", t("tls.uploadFailed"));
    setKeyPEM("");
    if (!payload) return;
    setMessage(payload.restart_required ? t("tls.savedRestart") : t("tls.applied"));
    setCertPEM("");
    setUploadOpen(false);
  };

  const clearUploadedCertificate = async () => {
    const payload = await request({ method: "DELETE" }, "clear", t("tls.clearFailed"));
    if (!payload) return;
    setMessage(payload.restart_required ? t("tls.clearedRestart") : t("tls.cleared"));
  };

  const restartBackend = async () => {
    setSubmitting("restart");
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/restart", { method: "POST", cache: "no-store" });
      const payload = await response.json().catch(() => null) as { message?: string; error?: string } | null;
      if (!response.ok) throw new Error(payload?.error || t("tls.restartFailed"));
      setMessage(payload?.message || t("tls.restartRequested"));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("tls.restartFailed"));
    } finally {
      setSubmitting("");
      setRestartOpen(false);
    }
  };

  const active = details?.active_certificate;
  const uploaded = details?.uploaded_certificate;
  const days = daysLeft(active?.expires_at);
  const typeLabel = details ? t(`tls.type.${TYPE_KEYS[details.cert_type] ?? "none"}`) : "";
  const tailscaleURL = tailscaleStatus?.serve_configured ? tailscaleStatus.tsnet_url : undefined;

  const summary = !details ? (loading ? "" : t("tls.unavailable")) : (
    <>
      {typeLabel}
      {days !== null ? (
        <>
          {" · "}
          <span className={days < 14 ? "st-g warn" : undefined}>{days < 0 ? t("tls.expired") : t("tls.expiresIn", { days })}</span>
        </>
      ) : null}
    </>
  );

  return (
    <>
      <Row label={t("tls.certificate")} hint={summary}>
        <Button size="sm" onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls="st-tls-details" disabled={!details}>
          {open ? t("hide") : t("show")}
        </Button>
      </Row>

      {open && details ? (
        <div className="st-x" id="st-tls-details">
          <Facts
            items={[
              [t("tls.source"), t(`tls.sources.${SOURCE_KEYS[details.tls_source] ?? "unknown"}`)],
              active?.dns_names?.length ? [t("tls.names"), active.dns_names.join(", ")] : false,
              active?.subject_summary ? [t("tls.subject"), active.subject_summary] : false,
              active?.issuer_summary ? [t("tls.issuer"), active.issuer_summary] : false,
              active?.expires_at ? [t("tls.expires"), formatWhen(active.expires_at, new Date(), locale)] : false,
              active?.fingerprint_sha256 ? [t("tls.fingerprint"), (
                <>
                  <code className="st-mono">{groupedFingerprint(active.fingerprint_sha256) || active.fingerprint_sha256}</code>
                  <CopyButton text={plainFingerprint(active.fingerprint_sha256) || active.fingerprint_sha256} label={t("tls.copyFingerprint")} />
                </>
              )] : false,
              // The value Mac, Windows and iPhone setup show before they trust this Hub (also in Add device).
              details.ca_available && caFingerprint ? [t("tls.caFingerprint"), (
                <>
                  <code className="st-mono">{groupedFingerprint(caFingerprint)}</code>
                  <CopyButton text={caFingerprint} label={t("tls.copyCaFingerprint")} />
                </>
              )] : false,
              uploaded?.subject_summary ? [t("tls.uploadedCert"), details.uploaded_updated_at
                ? t("tls.uploadedAt", { subject: uploaded.subject_summary, when: formatWhen(details.uploaded_updated_at, new Date(), locale) })
                : uploaded.subject_summary] : false,
            ]}
          />
          {details.cert_type === "self-signed" ? (
            <p className="st-g">{tailscaleURL ? t("tls.selfSignedTailscale", { url: tailscaleURL }) : t("tls.selfSignedNote")}</p>
          ) : null}

          {uploadOpen ? (
            <form
              className="st-form"
              aria-label={t("tls.upload")}
              onSubmit={(event) => {
                event.preventDefault();
                void applyUploadedCertificate();
              }}
            >
              <Field label={t("tls.certChain")} htmlFor="st-tls-cert" wide>
                <textarea id="st-tls-cert" className="st-ta" value={certPEM} onChange={(event) => setCertPEM(event.target.value)} placeholder="-----BEGIN CERTIFICATE-----" spellCheck={false} />
              </Field>
              <Field label={t("tls.privateKey")} htmlFor="st-tls-key" hint={t("tls.keyStored")} wide>
                <textarea id="st-tls-key" className="st-ta" value={keyPEM} onChange={(event) => setKeyPEM(event.target.value)} placeholder="-----BEGIN PRIVATE KEY-----" spellCheck={false} autoComplete="off" />
              </Field>
              <div className="st-acts st-field wide">
                <Button type="submit" size="sm" loading={submitting === "upload"} disabled={!certPEM.trim() || !keyPEM.trim()}>
                  {t("tls.apply")}
                </Button>
                <Button type="button" size="sm" variant="ghost" onClick={() => { setUploadOpen(false); setKeyPEM(""); }}>
                  {t("cancel")}
                </Button>
              </div>
            </form>
          ) : (
            <div className="st-acts">
              {details.ca_available ? (
                <a className="st-btn-link" href="/api/v1/ca.crt" download="labtether-ca.crt">{t("tls.downloadCA")}</a>
              ) : null}
              {details.can_upload !== false ? (
                <Button size="sm" onClick={() => setUploadOpen(true)}>{t("tls.upload")}</Button>
              ) : null}
              {details.uploaded_override_present ? (
                <Button size="sm" variant="ghost" loading={submitting === "clear"} onClick={() => void clearUploadedCertificate()}>
                  {t("tls.removeUploaded")}
                </Button>
              ) : null}
            </div>
          )}

          {details.restart_required ? (
            <div className="st-acts">
              <span className="st-msg warn">{details.restart_action_note || t("tls.restartNeeded")}</span>
              {details.restart_action_available ? (
                <Button size="sm" loading={submitting === "restart"} onClick={() => setRestartOpen(true)}>{t("tls.restart")}</Button>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}

      {error ? <Message tone="bad">{error}</Message> : null}
      {message ? <Message>{message}</Message> : null}

      {restartOpen ? (
        <SettingsDialog labelledBy="st-restart-title" onClose={() => { if (!submitting) setRestartOpen(false); }}>
          <h3 id="st-restart-title">{t("tls.restartTitle")}</h3>
          <p>{t("tls.restartBody")}</p>
          <div className="st-dlg-acts">
            <Button onClick={() => setRestartOpen(false)} disabled={submitting === "restart"}>{t("cancel")}</Button>
            <Button variant="danger" className="st-danger-fill" loading={submitting === "restart"} onClick={() => void restartBackend()}>
              {t("tls.restartConfirm")}
            </Button>
          </div>
        </SettingsDialog>
      ) : null}
    </>
  );
}
