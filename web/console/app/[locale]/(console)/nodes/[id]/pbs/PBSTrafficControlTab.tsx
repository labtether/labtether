"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { pbsAction, pbsFetch } from "./usePBSData";

type Props = {
  assetId: string;
};

type TrafficRule = {
  name: string;
  rate?: number;
  burst?: number;
  network?: string;
  comment?: string;
};

type TrafficControlResponse = {
  rules?: unknown[];
  error?: string;
};

function normalizeTrafficRules(value: unknown): TrafficRule[] {
  if (!value || typeof value !== "object") return [];
  const raw = value as Record<string, unknown>;
  const rules = raw.rules;
  if (!Array.isArray(rules)) return [];
  return rules.map((entry) => {
    const r = (entry && typeof entry === "object" ? entry : {}) as Record<string, unknown>;
    return {
      name: typeof r.name === "string" ? r.name : String(r.name ?? ""),
      rate: typeof r.rate === "number" ? r.rate : undefined,
      burst: typeof r.burst === "number" ? r.burst : undefined,
      network: typeof r.network === "string" ? r.network : undefined,
      comment: typeof r.comment === "string" ? r.comment : undefined,
    };
  });
}

function formatRate(bps?: number): string {
  if (bps == null) return "\u2014";
  if (bps >= 1_000_000_000) return `${(bps / 1_000_000_000).toFixed(1)} Gbps`;
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  if (bps >= 1_000) return `${(bps / 1_000).toFixed(1)} Kbps`;
  return `${bps} bps`;
}

export function PBSTrafficControlTab({ assetId }: Props) {
  const [rules, setRules] = useState<TrafficRule[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const seqRef = useRef(0);
  const latestRef = useRef(0);
  const actionSeq = useRef(0);

  const fetchRules = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await pbsFetch<TrafficControlResponse>(
        `/api/pbs/assets/${encodeURIComponent(assetId)}/traffic-control`,
      );
      if (latestRef.current !== id) return;
      setRules(normalizeTrafficRules(data));
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load traffic control rules");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void fetchRules();
  }, [fetchRules]);

  const doDelete = useCallback(
    async (name: string) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`delete-${name}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/traffic-control/${encodeURIComponent(name)}`,
          "DELETE",
        );
        if (actionSeq.current === seq) void fetchRules();
      } catch (err) {
        if (actionSeq.current === seq)
          setActionError(err instanceof Error ? err.message : "delete failed");
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, fetchRules],
  );

  return (
    <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Traffic Control</h2>
        <Button size="sm" variant="ghost" onClick={() => void fetchRules()} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </Button>
      </div>

      {actionError ? <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}

      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : rules.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">
          {loading ? "Loading traffic control rules..." : "No traffic control rules configured."}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Name</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Network</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Rate Limit</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Burst</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Comment</th>
                <th className="py-1 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr key={rule.name} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="py-2 px-2 text-[var(--text)] font-medium">{rule.name}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{rule.network || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{formatRate(rule.rate)}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{formatRate(rule.burst)}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{rule.comment || "\u2014"}</td>
                  <td className="py-2 px-2 text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={!!actionInFlight}
                      loading={actionInFlight === `delete-${rule.name}`}
                      onClick={() => void doDelete(rule.name)}
                    >
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
