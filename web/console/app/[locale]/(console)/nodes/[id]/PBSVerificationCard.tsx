"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import {
  formatRelativeEpoch,
  normalizePBSVerificationResponse,
  type PBSVerificationResponse,
} from "./pbsTabModel";

type Props = {
  assetId: string;
};

function mapStatus(status: string): "ok" | "pending" | "bad" {
  if (status === "bad") return "bad";
  if (status === "warn") return "pending";
  return "ok";
}

function computeOverallStatus(data: PBSVerificationResponse): "ok" | "pending" | "bad" {
  let worst: "ok" | "pending" | "bad" = "ok";
  for (const ds of data.datastores) {
    const mapped = mapStatus(ds.status);
    if (mapped === "bad") return "bad";
    if (mapped === "pending") worst = "pending";
  }
  return worst;
}

export function PBSVerificationCard({ assetId }: Props) {
  const [data, setData] = useState<PBSVerificationResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const requestSeqRef = useRef(0);
  const latestRequestRef = useRef(0);

  const fetchVerification = useCallback(async () => {
    const requestID = ++requestSeqRef.current;
    latestRequestRef.current = requestID;
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(
        `/api/pbs/assets/${encodeURIComponent(assetId)}/verification`,
        { cache: "no-store" },
      );
      const payload = normalizePBSVerificationResponse(await response.json().catch(() => null));
      if (!response.ok) {
        throw new Error(payload.error || `failed to load verification data (${response.status})`);
      }
      if (latestRequestRef.current !== requestID) {
        return;
      }
      setData(payload);
    } catch (err) {
      if (latestRequestRef.current !== requestID) {
        return;
      }
      setError(err instanceof Error ? err.message : "failed to load verification data");
      setData(null);
    } finally {
      if (latestRequestRef.current === requestID) {
        setLoading(false);
      }
    }
  }, [assetId]);

  useEffect(() => {
    void fetchVerification();
  }, [fetchVerification]);

  const overallStatus = data ? computeOverallStatus(data) : null;

  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-medium text-[var(--text)]">Verification Status</h2>
          {overallStatus ? <Badge status={overallStatus} size="sm" /> : null}
        </div>
        <Button
          size="sm"
          onClick={() => {
            void fetchVerification();
          }}
          disabled={loading}
        >
          {loading ? "Refreshing..." : "Refresh"}
        </Button>
      </div>

      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : !data ? (
        <p className="text-xs text-[var(--muted)]">
          {loading ? "Loading verification data..." : "No verification data available."}
        </p>
      ) : data.datastores.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">No datastore verification data returned.</p>
      ) : (
        <div className="space-y-3">
          {data.warnings && data.warnings.length > 0 ? (
            <ul className="space-y-1">
              {data.warnings.map((warning) => (
                <li key={warning} className="text-xs text-[var(--warn)]">
                  {warning}
                </li>
              ))}
            </ul>
          ) : null}

          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Datastore</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Status</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Verified</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Unverified</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Failed</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Last Verify</th>
                </tr>
              </thead>
              <tbody>
                {data.datastores.map((ds) => (
                  <tr key={ds.store} className="border-b border-[var(--line)] border-opacity-30">
                    <td className="py-1 px-2 text-[var(--text)] font-medium">{ds.store}</td>
                    <td className="py-1 px-2">
                      <Badge status={mapStatus(ds.status)} size="sm" />
                    </td>
                    <td className="py-1 px-2 text-[var(--ok)]">
                      {ds.verified_count}
                    </td>
                    <td
                      className="py-1 px-2"
                      style={{ color: ds.unverified_count > 0 ? "var(--warn)" : "var(--muted)" }}
                    >
                      {ds.unverified_count}
                    </td>
                    <td
                      className="py-1 px-2"
                      style={{ color: ds.failed_count > 0 ? "var(--bad)" : "var(--muted)" }}
                    >
                      {ds.failed_count}
                    </td>
                    <td className="py-1 px-2 text-[var(--muted)]">
                      {ds.last_verify_time ? formatRelativeEpoch(ds.last_verify_time) : "never"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </Card>
  );
}
