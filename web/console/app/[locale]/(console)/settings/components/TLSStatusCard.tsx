"use client";

import { useEffect, useState } from "react";
import { Copy, Download, ShieldOff } from "lucide-react";
import { Card } from "../../../../components/ui/Card";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { SkeletonRow } from "../../../../components/ui/Skeleton";

type TLSInfo = {
  tls_enabled: boolean;
  cert_type: string;
  ca_available: boolean;
  ca_fingerprint_sha256?: string;
  ca_expires?: string;
};

function certTypeLabel(certType: string): string {
  if (certType === "self-signed") return "Self-Signed (Built-in CA)";
  if (certType === "external") return "External";
  return "None";
}

type TLSStatusCardProps = {
  copyToClipboard: (text: string, label: string, toastMessage?: string) => void;
  copied: string;
};

export function TLSStatusCard({ copyToClipboard, copied }: TLSStatusCardProps) {
  const [tlsInfo, setTlsInfo] = useState<TLSInfo | null>(null);
  const [tlsLoading, setTlsLoading] = useState(true);
  const [tlsError, setTlsError] = useState("");

  useEffect(() => {
    setTlsLoading(true);
    fetch("/api/v1/tls/info")
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json() as Promise<TLSInfo>;
      })
      .then((data) => {
        setTlsInfo(data);
        setTlsError("");
      })
      .catch((err: unknown) => {
        setTlsError(err instanceof Error ? err.message : "Failed to load TLS info");
      })
      .finally(() => {
        setTlsLoading(false);
      });
  }, []);

  const downloadCA = () => {
    const anchor = document.createElement("a");
    anchor.href = "/api/v1/ca.crt";
    anchor.download = "labtether-ca.crt";
    anchor.click();
  };

  const isSelfSigned = tlsInfo?.cert_type === "self-signed";

  // Hide the card entirely if we got an error and have no data (e.g. endpoint doesn't exist)
  if (!tlsLoading && tlsError && !tlsInfo) return null;

  return (
    <Card className="mb-6">
      <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">// TLS / Certificate Status</p>
      {tlsLoading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}
      {tlsError ? <p className="text-xs text-[var(--bad)]">{tlsError}</p> : null}

      {!tlsLoading && !tlsError && tlsInfo ? (
        <div className="space-y-4">
          {isSelfSigned ? (
            <div className="flex items-start gap-2 rounded-lg border border-[var(--warn)]/20 bg-[var(--warn-glow)] px-3 py-2.5">
              <ShieldOff size={14} className="mt-0.5 shrink-0 text-[var(--warn)]" />
              <p className="text-xs text-[var(--warn)]">
                LabTether uses a self-signed certificate. Install the CA certificate on your devices to remove browser warnings.
              </p>
            </div>
          ) : null}

          <div className="divide-y divide-[var(--line)]">
            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">TLS Status</span>
                <span className="text-xs text-[var(--muted)]">Whether HTTPS is active on this hub</span>
              </div>
              <div className="flex items-center gap-2 sm:justify-end">
                <Badge status={tlsInfo.tls_enabled ? "enabled" : "disabled"} size="sm" />
              </div>
            </div>

            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Certificate Type</span>
                <span className="text-xs text-[var(--muted)]">How the TLS certificate was provisioned</span>
              </div>
              <div className="flex items-center gap-2 sm:justify-end">
                <span className="text-sm text-[var(--text)]">{certTypeLabel(tlsInfo.cert_type)}</span>
              </div>
            </div>

            {tlsInfo.ca_available && tlsInfo.ca_fingerprint_sha256 ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">CA Fingerprint</span>
                  <span className="text-xs text-[var(--muted)]">SHA-256 fingerprint of the built-in CA certificate</span>
                </div>
                <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">
                  <code className="block max-w-full break-all text-xs sm:text-sm">{tlsInfo.ca_fingerprint_sha256}</code>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      copyToClipboard(
                        tlsInfo.ca_fingerprint_sha256!,
                        "ca-fingerprint",
                        "CA fingerprint copied to clipboard"
                      )
                    }
                    aria-label="Copy CA fingerprint"
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "ca-fingerprint" ? "Copied" : "Copy"}
                  </Button>
                </div>
              </div>
            ) : null}

            {tlsInfo.ca_available && tlsInfo.ca_expires ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">CA Expiry</span>
                  <span className="text-xs text-[var(--muted)]">When the built-in CA certificate expires</span>
                </div>
                <div className="flex items-center gap-2 sm:justify-end">
                  <span className="text-sm text-[var(--text)]">
                    {new Date(tlsInfo.ca_expires).toLocaleDateString(undefined, {
                      year: "numeric",
                      month: "long",
                      day: "numeric",
                    })}
                  </span>
                </div>
              </div>
            ) : null}

            {tlsInfo.ca_available ? (
              <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">CA Certificate</span>
                  <span className="text-xs text-[var(--muted)]">
                    Install on your devices to trust LabTether&apos;s self-signed certificate
                  </span>
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

          {!tlsInfo.tls_enabled && (
            <p className="text-xs text-[var(--muted)]">
              TLS is not enabled on this hub. Enable it via runtime settings to serve traffic over HTTPS.
            </p>
          )}
        </div>
      ) : null}
    </Card>
  );
}
