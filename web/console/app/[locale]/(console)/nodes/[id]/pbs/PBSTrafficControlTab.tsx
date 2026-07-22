"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { PBSActionConfirmation } from "./PBSActionConfirmation";
import { pbsAction, pbsFetch } from "./usePBSData";

type Props = {
  assetId: string;
};

type TrafficRule = {
  name: string;
  rate_in?: string;
  rate_out?: string;
  network?: string[];
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
      rate_in:
        typeof r.rate_in === "string"
          ? r.rate_in
          : typeof r["rate-in"] === "string"
            ? r["rate-in"]
            : undefined,
      rate_out:
        typeof r.rate_out === "string"
          ? r.rate_out
          : typeof r["rate-out"] === "string"
            ? r["rate-out"]
            : undefined,
      network: Array.isArray(r.network)
        ? r.network.filter((entry): entry is string => typeof entry === "string")
        : typeof r.network === "string"
          ? [r.network]
          : undefined,
      comment: typeof r.comment === "string" ? r.comment : undefined,
    };
  });
}

export function PBSTrafficControlTab({ assetId }: Props) {
  const [rules, setRules] = useState<TrafficRule[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<string | null>(null);

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
      setActionSuccess(null);
      setConfirmation(null);
      setActionInFlight(`delete-${name}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/traffic-control/${encodeURIComponent(name)}`,
          "DELETE",
        );
        if (actionSeq.current === seq) {
          setActionSuccess(`Traffic control rule ${name} deleted.`);
          void fetchRules();
        }
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
    <div className="space-y-4">
      {confirmation ? (
        <PBSActionConfirmation
          message={`Confirm delete traffic control rule ${confirmation}?`}
          confirmLabel="Confirm Delete"
          busy={actionInFlight !== null}
          onConfirm={() => void doDelete(confirmation)}
          onCancel={() => setConfirmation(null)}
        />
      ) : null}
      <Card>
      <div className="flex items-center justify-between mb-3 flex-wrap gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">Traffic Control</h2>
        <Button size="sm" variant="ghost" onClick={() => void fetchRules()} disabled={loading}>
          {loading ? "Loading..." : "Refresh"}
        </Button>
      </div>

      {actionError ? <p role="alert" className="mb-3 text-xs text-[var(--bad)]">{actionError}</p> : null}
      {actionSuccess ? <p role="status" className="mb-3 text-xs text-[var(--ok)]">{actionSuccess}</p> : null}

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
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Inbound Limit</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Outbound Limit</th>
                <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Comment</th>
                <th className="py-1 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr key={rule.name} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="py-2 px-2 text-[var(--text)] font-medium">{rule.name}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{rule.network?.join(", ") || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{rule.rate_in || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{rule.rate_out || "\u2014"}</td>
                  <td className="py-2 px-2 text-[var(--muted)]">{rule.comment || "\u2014"}</td>
                  <td className="py-2 px-2 text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={!!actionInFlight}
                      loading={actionInFlight === `delete-${rule.name}`}
                      onClick={() => setConfirmation(rule.name)}
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
    </div>
  );
}
