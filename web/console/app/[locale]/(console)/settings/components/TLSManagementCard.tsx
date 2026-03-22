"use client";

import { useEffect, useState } from "react";
import { Copy, Download, Info, RotateCcw, ShieldOff, Upload } from "lucide-react";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { SkeletonRow } from "../../../../components/ui/Skeleton";
import { useTailscaleServeStatus } from "../../../../hooks/useTailscaleServeStatus";

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
  default_tls_mode?: string;
  default_tls_source?: string;
  uploaded_override_present: boolean;
  uploaded_certificate?: TLSCertificateMetadata;
  uploaded_updated_at?: string;
  can_upload: boolean;
  can_apply_live: boolean;
  restart_required: boolean;
  restart_action_available: boolean;
  restart_action_note?: string;
  error?: string;
};

type RestartSettingsResponse = {
  accepted?: boolean;
  message?: string;
  error?: string;
};

type TLSManagementCardProps = {
  copyToClipboard: (text: string, label: string, toastMessage?: string) => void;
  copied: string;
};

function sourceLabel(source: string): string {
  switch (source) {
    case "built_in":
      return "Built-in LabTether TLS";
    case "tailscale":
      return "Tailscale (Publicly Trusted)";
    case "ui_uploaded":
      return "Uploaded Certificate";
    case "deployment_external":
      return "Deployment-managed External TLS";
    case "disabled":
      return "Disabled";
    default:
      return source || "Unknown";
  }
}

function certTypeLabel(certType: string): string {
  switch (certType) {
    case "self-signed":
      return "Self-Signed (Built-in CA)";
    case "tailscale":
      return "Tailscale (Publicly Trusted)";
    case "uploaded":
      return "Uploaded";
    case "external":
      return "External";
    default:
      return "None";
  }
}

export function TLSManagementCard({ copyToClipboard, copied }: TLSManagementCardProps) {
  const [details, setDetails] = useState<TLSSettingsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [certPEM, setCertPEM] = useState("");
  const [keyPEM, setKeyPEM] = useState("");
  const [submitting, setSubmitting] = useState<"" | "upload" | "clear" | "restart">("");

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/settings/tls", { cache: "no-store" });
      const payload = await response.json().catch(() => null) as TLSSettingsResponse | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setDetails(payload ?? null);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to load TLS settings");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const downloadCA = () => {
    const anchor = document.createElement("a");
    anchor.href = "/api/v1/ca.crt";
    anchor.download = "labtether-ca.crt";
    anchor.click();
  };

  const applyUploadedCertificate = async () => {
    setSubmitting("upload");
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/tls", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cert_pem: certPEM, key_pem: keyPEM }),
      });
      const payload = await response.json().catch(() => null) as TLSSettingsResponse | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setDetails(payload ?? null);
      setMessage(payload?.restart_required
        ? "Certificate saved. Restart the backend to finish applying it."
        : "Uploaded certificate applied.");
      setKeyPEM("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "TLS certificate upload failed");
    } finally {
      setSubmitting("");
    }
  };

  const clearUploadedCertificate = async () => {
    setSubmitting("clear");
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/tls", { method: "DELETE" });
      const payload = await response.json().catch(() => null) as TLSSettingsResponse | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setDetails(payload ?? null);
      setMessage(payload?.restart_required
        ? "Uploaded override cleared. Restart the backend to return to the startup TLS source."
        : "Restored the startup TLS source.");
      setCertPEM("");
      setKeyPEM("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to clear uploaded TLS certificate");
    } finally {
      setSubmitting("");
    }
  };

  const restartBackend = async () => {
    if (!details?.restart_action_available) {
      return;
    }
    const confirmed = window.confirm(
      "Restart the backend now? The current hub process will exit cleanly. It only comes back automatically if Docker or another process supervisor restarts it.",
    );
    if (!confirmed) {
      return;
    }

    setSubmitting("restart");
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/restart", {
        method: "POST",
        cache: "no-store",
      });
      const payload = await response.json().catch(() => null) as RestartSettingsResponse | null;
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`);
      }
      setMessage(payload?.message || "Backend restart requested. Wait for the hub to come back up.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to restart backend");
    } finally {
      setSubmitting("");
    }
  };

  const { status: tailscaleStatus } = useTailscaleServeStatus();
  const tailscaleServeActive = Boolean(tailscaleStatus?.serve_configured && tailscaleStatus?.tsnet_url);

  const activeCertificate = details?.active_certificate;
  const uploadedCertificate = details?.uploaded_certificate;

  return (
    <Card className="mb-6">
      <div className="mb-3 flex items-center justify-between gap-3">
        <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">// TLS Management</p>
        <Button variant="ghost" size="sm" onClick={() => void load()} loading={loading && Boolean(details)}>
          Refresh
        </Button>
      </div>

      {loading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}
      {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
      {message ? <p className="text-xs text-[var(--ok)]">{message}</p> : null}

      {!loading && details ? (
        <div className="space-y-4">
          {details.cert_type === "self-signed" && tailscaleServeActive ? (
            <div className="flex items-start gap-2 rounded-lg border border-[var(--accent)]/20 bg-[var(--accent-glow)]/30 px-3 py-2.5">
              <Info size={14} className="mt-0.5 shrink-0 text-[var(--accent)]" />
              <div className="space-y-1">
                <p className="text-xs text-[var(--text)]/90">
                  Tailscale HTTPS is active — agents enrolled via <code className="text-[10px]">{tailscaleStatus!.tsnet_url}</code> use a publicly trusted certificate and do not need this built-in CA.
                </p>
                <p className="text-xs text-[var(--muted)]">
                  This built-in certificate is only used when agents connect directly to the hub (e.g. by LAN IP). Those agents need the pinned CA installer.
                </p>
              </div>
            </div>
          ) : details.cert_type === "self-signed" ? (
            <div className="flex items-start gap-2 rounded-lg border border-[var(--warn)]/20 bg-[var(--warn-glow)] px-3 py-2.5">
              <ShieldOff size={14} className="mt-0.5 shrink-0 text-[var(--warn)]" />
              <p className="text-xs text-[var(--warn)]">
                LabTether is serving its built-in self-signed certificate. Agents connecting directly need the pinned CA installer to trust this hub. Enable Tailscale HTTPS above for a publicly trusted alternative.
              </p>
            </div>
          ) : null}

          <div className="divide-y divide-[var(--line)]">
            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Active TLS Source</span>
                <span className="text-xs text-[var(--muted)]">What currently owns the certificate served by the hub</span>
              </div>
              <div className="flex items-center gap-2 sm:justify-end">
                <Badge status={details.tls_enabled ? "enabled" : "disabled"} size="sm" />
                <span className="text-sm text-[var(--text)]">{sourceLabel(details.tls_source)}</span>
              </div>
            </div>

            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Certificate Type</span>
                <span className="text-xs text-[var(--muted)]">How the active TLS certificate was provisioned</span>
              </div>
              <div className="flex items-center gap-2 sm:justify-end">
                <span className="text-sm text-[var(--text)]">{certTypeLabel(details.cert_type)}</span>
              </div>
            </div>

            {activeCertificate?.subject_summary ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">Subject</span>
                  <span className="text-xs text-[var(--muted)]">Presented certificate subject</span>
                </div>
                <span className="text-sm text-[var(--text)] sm:text-right">{activeCertificate.subject_summary}</span>
              </div>
            ) : null}

            {activeCertificate?.issuer_summary ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">Issuer</span>
                  <span className="text-xs text-[var(--muted)]">Who signed the active certificate</span>
                </div>
                <span className="text-sm text-[var(--text)] sm:text-right">{activeCertificate.issuer_summary}</span>
              </div>
            ) : null}

            {activeCertificate?.expires_at ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">Certificate Expiry</span>
                  <span className="text-xs text-[var(--muted)]">When the active certificate expires</span>
                </div>
                <span className="text-sm text-[var(--text)]">
                  {new Date(activeCertificate.expires_at).toLocaleDateString(undefined, {
                    year: "numeric",
                    month: "long",
                    day: "numeric",
                  })}
                </span>
              </div>
            ) : null}

            {activeCertificate?.fingerprint_sha256 ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">Certificate Fingerprint</span>
                  <span className="text-xs text-[var(--muted)]">SHA-256 fingerprint of the active certificate leaf</span>
                </div>
                <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">
                  <code className="block max-w-full break-all text-xs sm:text-sm">{activeCertificate.fingerprint_sha256}</code>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => copyToClipboard(activeCertificate.fingerprint_sha256!, "tls-fingerprint", "TLS fingerprint copied")}
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "tls-fingerprint" ? "Copied" : "Copy"}
                  </Button>
                </div>
              </div>
            ) : null}

            {details.ca_available ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">Built-in CA</span>
                  <span className="text-xs text-[var(--muted)]">Available while the active source is LabTether&apos;s built-in TLS</span>
                </div>
                <div className="flex items-center gap-2 sm:justify-end">
                  <Button variant="ghost" size="sm" onClick={downloadCA}>
                    <Download size={13} className="shrink-0" />
                    Download CA Certificate
                  </Button>
                </div>
              </div>
            ) : null}
          </div>

          <div className="rounded-lg border border-[var(--line)] p-3">
            <div className="mb-3">
              <p className="text-sm font-medium text-[var(--text)]">Upload Signed Certificate</p>
              <p className="text-xs text-[var(--muted)]">
                Upload a PEM certificate chain and matching PEM private key. The private key is encrypted before it is stored.
              </p>
            </div>

            <div className="grid gap-3">
              <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                Certificate PEM / Chain
                <textarea
                  className="min-h-32 rounded-lg border border-[var(--line)] bg-[var(--panel)] px-3 py-2 font-mono text-xs text-[var(--text)]"
                  value={certPEM}
                  onChange={(event) => setCertPEM(event.target.value)}
                  placeholder="-----BEGIN CERTIFICATE-----"
                />
              </label>

              <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
                Private Key PEM
                <textarea
                  className="min-h-32 rounded-lg border border-[var(--line)] bg-[var(--panel)] px-3 py-2 font-mono text-xs text-[var(--text)]"
                  value={keyPEM}
                  onChange={(event) => setKeyPEM(event.target.value)}
                  placeholder="-----BEGIN PRIVATE KEY-----"
                />
              </label>

              <div className="flex flex-wrap items-center gap-2">
                <Button variant="primary" onClick={() => void applyUploadedCertificate()} loading={submitting === "upload"}>
                  <Upload size={13} className="shrink-0" />
                  Apply Uploaded Certificate
                </Button>
                {details.uploaded_override_present ? (
                  <Button variant="ghost" onClick={() => void clearUploadedCertificate()} loading={submitting === "clear"}>
                    <RotateCcw size={13} className="shrink-0" />
                    Restore Startup TLS Source
                  </Button>
                ) : null}
              </div>

              {details.restart_required ? (
                <div className="rounded-lg border border-[var(--warn)]/20 bg-[var(--warn-glow)] px-3 py-2.5">
                  <p className="text-xs text-[var(--warn)]">
                    A backend restart is required before this TLS change can fully take effect.
                    {!details.tls_enabled
                      ? " This hub is currently serving without a live-swappable TLS listener, so the new certificate will not be served until the process starts again."
                      : " The change has been saved, but the current process cannot finish this transition without restarting."}
                  </p>
                  {details.restart_action_note ? (
                    <p className="mt-2 text-xs text-[var(--warn)]">{details.restart_action_note}</p>
                  ) : null}
                  {details.restart_action_available ? (
                    <div className="mt-3">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => void restartBackend()}
                        loading={submitting === "restart"}
                      >
                        Restart Backend
                      </Button>
                    </div>
                  ) : null}
                </div>
              ) : null}
            </div>
          </div>

          {uploadedCertificate?.subject_summary ? (
            <div className="rounded-lg border border-[var(--line)] p-3">
              <p className="text-sm font-medium text-[var(--text)]">Uploaded Override</p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                {uploadedCertificate.subject_summary}
                {details.uploaded_updated_at ? ` · saved ${new Date(details.uploaded_updated_at).toLocaleString()}` : ""}
              </p>
            </div>
          ) : null}
        </div>
      ) : null}
    </Card>
  );
}
