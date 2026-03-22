"use client";

import { Card } from "../../../../../components/ui/Card";
import { useProxmoxList } from "./useProxmoxData";

type Certificate = {
  filename?: string;
  fingerprint?: string;
  issuer?: string;
  subject?: string;
  notbefore?: number;
  notafter?: number;
  public_key_bits?: number;
  public_key_type?: string;
  san?: string[];
};

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

function certExpiryClass(notafter: number | undefined): string {
  if (notafter == null) return "text-[var(--muted)]";
  const nowMs = Date.now();
  const expiresMs = notafter * 1000;
  const daysLeft = (expiresMs - nowMs) / (1000 * 60 * 60 * 24);
  if (daysLeft < 0) return "text-[var(--bad)]";
  if (daysLeft < 7) return "text-[var(--bad)]";
  if (daysLeft < 30) return "text-[var(--warn)]";
  return "text-[var(--ok)]";
}

function certExpiryLabel(notafter: number | undefined): string {
  if (notafter == null) return "-";
  const nowMs = Date.now();
  const expiresMs = notafter * 1000;
  const daysLeft = Math.floor((expiresMs - nowMs) / (1000 * 60 * 60 * 24));
  const dateStr = new Date(expiresMs).toLocaleDateString();
  if (daysLeft < 0) return `Expired ${dateStr}`;
  return `${dateStr} (${daysLeft}d)`;
}

export function ProxmoxCertificatesTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const collectorSuffix = proxmoxCollectorID
    ? `?collector_id=${encodeURIComponent(proxmoxCollectorID)}`
    : "";

  const path = proxmoxNode
    ? `/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/certificates${collectorSuffix}`
    : null;

  const { data: certs, loading, error, refresh } = useProxmoxList<Certificate>(path);

  return (
    <Card>
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">
          Certificates{certs.length > 0 ? ` (${certs.length})` : ""}
        </h2>
        <button
          className="ml-auto text-xs text-[var(--accent)] hover:underline"
          onClick={refresh}
          disabled={loading}
        >
          {loading ? "Loading..." : "Refresh"}
        </button>
      </div>
      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : loading && certs.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Loading certificates...</p>
      ) : certs.length > 0 ? (
        <div className="space-y-4">
          {certs.map((cert, idx) => (
            <div
              key={cert.fingerprint ?? idx}
              className="rounded-lg border border-[var(--line)] p-3 space-y-2"
            >
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs font-medium text-[var(--text)]">
                  {cert.filename || `Certificate ${idx + 1}`}
                </span>
                <span className={`text-xs font-medium ${certExpiryClass(cert.notafter)}`}>
                  Expires: {certExpiryLabel(cert.notafter)}
                </span>
              </div>
              <dl className="grid grid-cols-2 gap-x-4 gap-y-1">
                {cert.subject ? (
                  <div className="col-span-2">
                    <dt className="text-[10px] text-[var(--muted)]">Subject</dt>
                    <dd className="text-xs text-[var(--text)] break-all">{cert.subject}</dd>
                  </div>
                ) : null}
                {cert.issuer ? (
                  <div className="col-span-2">
                    <dt className="text-[10px] text-[var(--muted)]">Issuer</dt>
                    <dd className="text-xs text-[var(--muted)] break-all">{cert.issuer}</dd>
                  </div>
                ) : null}
                {cert.notbefore != null ? (
                  <div>
                    <dt className="text-[10px] text-[var(--muted)]">Valid From</dt>
                    <dd className="text-xs text-[var(--muted)]">
                      {new Date(cert.notbefore * 1000).toLocaleDateString()}
                    </dd>
                  </div>
                ) : null}
                {cert.public_key_type ? (
                  <div>
                    <dt className="text-[10px] text-[var(--muted)]">Key</dt>
                    <dd className="text-xs text-[var(--muted)]">
                      {cert.public_key_type}
                      {cert.public_key_bits != null ? ` ${cert.public_key_bits}-bit` : ""}
                    </dd>
                  </div>
                ) : null}
                {cert.fingerprint ? (
                  <div className="col-span-2">
                    <dt className="text-[10px] text-[var(--muted)]">Fingerprint</dt>
                    <dd className="font-mono text-[10px] text-[var(--muted)] break-all">
                      {cert.fingerprint}
                    </dd>
                  </div>
                ) : null}
                {cert.san && cert.san.length > 0 ? (
                  <div className="col-span-2">
                    <dt className="text-[10px] text-[var(--muted)]">SANs</dt>
                    <dd className="text-xs text-[var(--muted)]">{cert.san.join(", ")}</dd>
                  </div>
                ) : null}
              </dl>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-xs text-[var(--muted)]">No certificates returned.</p>
      )}
    </Card>
  );
}
