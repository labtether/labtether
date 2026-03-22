"use client";

import { useCallback, useMemo, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { usePBSDetails, pbsAction } from "./usePBSData";
import { PBSDatastoresCard } from "../PBSDatastoresCard";

type Props = {
  assetId: string;
};

export function PBSDatastoresTab({ assetId }: Props) {
  const { details, loading, error, refresh } = usePBSDetails(assetId);

  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const sortedDatastores = useMemo(() => {
    if (!details) return [];
    const list =
      details.kind === "server"
        ? (details.datastores ?? [])
        : details.datastore
        ? [details.datastore]
        : [];
    return [...list].sort((a, b) => a.store.localeCompare(b.store));
  }, [details]);

  const doAction = useCallback(
    async (store: string, action: "gc" | "verify" | "maintenance-enable" | "maintenance-disable") => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`${action}-${store}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/datastores/${encodeURIComponent(store)}/${action}`,
          "POST",
        );
        if (actionSeq.current === seq) refresh();
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : `${action} failed`);
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refresh],
  );

  if (loading && !details) {
    return <div className="p-4 text-sm text-[var(--muted)]">Loading datastores...</div>;
  }

  if (error && !details) {
    return <div className="p-4 text-sm text-[var(--bad)]">{error}</div>;
  }

  return (
    <div className="space-y-4">
      {actionError ? (
        <p className="text-xs text-[var(--bad)]">{actionError}</p>
      ) : null}

      <PBSDatastoresCard datastores={sortedDatastores} assetId={assetId} />

      {sortedDatastores.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--line)]">
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Datastore</th>
                <th className="py-1.5 px-2 text-left text-[var(--muted)] font-medium">Maintenance</th>
                <th className="py-1.5 px-2 text-right text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {sortedDatastores.map((ds) => (
                <tr key={ds.store} className="border-b border-[var(--line)] border-opacity-30">
                  <td className="py-2 px-2 text-[var(--text)] font-medium">{ds.store}</td>
                  <td className="py-2 px-2">
                    {ds.maintenance_mode ? (
                      <span className="text-xs text-[var(--warn)]">{ds.maintenance_mode}</span>
                    ) : (
                      <span className="text-xs text-[var(--muted)]">off</span>
                    )}
                  </td>
                  <td className="py-2 px-2">
                    <div className="flex items-center justify-end gap-1 flex-wrap">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `gc-${ds.store}`}
                        onClick={() => { void doAction(ds.store, "gc"); }}
                      >
                        Run GC
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `verify-${ds.store}`}
                        onClick={() => { void doAction(ds.store, "verify"); }}
                      >
                        Verify
                      </Button>
                      {ds.maintenance_mode ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `maintenance-disable-${ds.store}`}
                          onClick={() => { void doAction(ds.store, "maintenance-disable"); }}
                        >
                          Exit Maintenance
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `maintenance-enable-${ds.store}`}
                          onClick={() => { void doAction(ds.store, "maintenance-enable"); }}
                        >
                          Enter Maintenance
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
