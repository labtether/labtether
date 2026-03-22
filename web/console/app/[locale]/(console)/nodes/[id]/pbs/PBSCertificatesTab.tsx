"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { pbsFetch } from "./usePBSData";

type Props = {
  assetId: string;
};

type Certificate = {
  filename: string;
  subject?: string;
  issuer?: string;
  san?: string[];
  not_after?: number;
  fingerprint?: string;
};

type CertificatesResponse = {
  certificates?: unknown[];
  error?: string;
};

function normalizeCertificates(value: unknown): Certificate[] {
  if (!value || typeof value !== "object") return [];
  const raw = value as Record<string, unknown>;
  const certs = raw.certificates;
  if (!Array.isArray(certs)) return [];
  return certs.map((entry) => {
    const c = (entry && typeof entry === "object" ? entry : {}) as Record<string, unknown>;
    return {
      filename: typeof c.filename === "string" ? c.filename : String(c.filename ?? ""),
      subject: typeof c.subject === "string" ? c.subject : undefined,
      issuer: typeof c.issuer === "string" ? c.issuer : undefined,
      san: Array.isArray(c.san)
        ? c.san.filter((s): s is string => typeof s === "string")
        : undefined,
      not_after: typeof c.not_after === "number" ? c.not_after : undefined,
      fingerprint: typeof c.fingerprint === "string" ? c.fingerprint : undefined,
    };
  });
}

function expiryStatus(notAfterEpoch?: number): {
  label: string;
  color: string;
} {
  if (!notAfterEpoch) return { label: "unknown", color: "var(--muted)" };
  const msLeft = notAfterEpoch * 1000 - Date.now();
  const daysLeft = msLeft / 86_400_000;
  if (daysLeft < 0) return { label: "expired", color: "var(--bad)" };
  if (daysLeft < 7) return { label: `${Math.floor(daysLeft)}d`, color: "var(--bad)" };
  if (daysLeft < 30) return { label: `${Math.floor(daysLeft)}d`, color: "var(--warn)" };
  return { label: `${Math.floor(daysLeft)}d`, color: "var(--ok)" };
}

function formatEpochDate(epoch?: number): string {
  if (!epoch) return "\u2014";
  return new Date(epoch * 1000).toLocaleDateString();
}

export function PBSCertificatesTab({ assetId }: Props) {
  const [certs, setCerts] = useState<Certificate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const fetchCerts = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await pbsFetch<CertificatesResponse>(
        `/api/pbs/assets/${encodeURIComponent(assetId)}/certificates`,
      );
      if (latestRef.current !== id) return;
      setCerts(normalizeCertificates(data));
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load certificates");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void fetchCerts();
  }, [fetchCerts]);

  return (
    <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Certificates</h2>
        <Button size="sm" variant="ghost" onClick={() => void fetchCerts()} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </Button>
      </div>

      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : certs.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">
          {loading ? "Loading certificates..." : "No certificates found."}
        </p>
      ) : (
        <div className="space-y-4">
          {certs.map((cert) => {
            const { label: expiryLabel, color: expiryColor } = expiryStatus(cert.not_after);
            return (
              <div
                key={cert.filename}
                className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4"
              >
                <div className="flex items-center justify-between gap-2 flex-wrap mb-2">
                  <span className="text-xs font-medium text-[var(--text)]">{cert.filename}</span>
                  <span
                    className="text-xs font-medium"
                    style={{ color: expiryColor }}
                  >
                    Expires: {expiryLabel} ({formatEpochDate(cert.not_after)})
                  </span>
                </div>
                <div className="grid grid-cols-1 gap-1 text-xs text-[var(--muted)] sm:grid-cols-2">
                  {cert.subject ? (
                    <div>
                      <span className="text-[var(--muted)]">Subject: </span>
                      <span className="text-[var(--text)] break-all">{cert.subject}</span>
                    </div>
                  ) : null}
                  {cert.issuer ? (
                    <div>
                      <span className="text-[var(--muted)]">Issuer: </span>
                      <span className="text-[var(--text)] break-all">{cert.issuer}</span>
                    </div>
                  ) : null}
                  {cert.fingerprint ? (
                    <div className="sm:col-span-2">
                      <span className="text-[var(--muted)]">Fingerprint: </span>
                      <span className="text-[var(--text)] font-mono break-all text-[10px]">
                        {cert.fingerprint}
                      </span>
                    </div>
                  ) : null}
                  {cert.san && cert.san.length > 0 ? (
                    <div className="sm:col-span-2">
                      <span className="text-[var(--muted)]">SANs: </span>
                      <span className="text-[var(--text)]">{cert.san.join(", ")}</span>
                    </div>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
