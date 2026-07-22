"use client";

import { useCallback, useMemo, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { usePBSDetails, pbsAction } from "./usePBSData";
import { PBSDatastoresCard } from "../PBSDatastoresCard";

type Props = {
  assetId: string;
};

type DatastoreAction = "gc" | "verify" | "maintenance-enable" | "maintenance-disable";

function datastoreActionLabel(action: DatastoreAction): string {
  switch (action) {
    case "gc":
      return "run garbage collection";
    case "verify":
      return "start verification";
    case "maintenance-enable":
      return "enter read-only maintenance";
    case "maintenance-disable":
      return "exit maintenance";
  }
}

export function PBSDatastoresTab({ assetId }: Props) {
  const { details, loading, error, refresh } = usePBSDetails(assetId);

  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<{
    store: string;
    action: DatastoreAction;
  } | null>(null);
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
    async (store: string, action: DatastoreAction) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionSuccess(null);
      setConfirmation(null);
      setActionInFlight(`${action}-${store}`);
      try {
        const result = await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/datastores/${encodeURIComponent(store)}/${action}`,
          "POST",
        );
        if (actionSeq.current === seq) {
          const upid =
            result && typeof result === "object" && "upid" in result
              ? String((result as { upid?: unknown }).upid ?? "").trim()
              : "";
          setActionSuccess(
            `${datastoreActionLabel(action)} requested for ${store}${upid ? ` (${upid})` : ""}.`,
          );
          refresh();
        }
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
        <p role="alert" className="text-xs text-[var(--bad)]">{actionError}</p>
      ) : null}
      {actionSuccess ? (
        <p role="status" className="text-xs text-[var(--ok)]">{actionSuccess}</p>
      ) : null}
      {confirmation ? (
        <div role="alert" className="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--warn)] px-3 py-2 text-xs text-[var(--text)]">
          <span>
            Confirm: {datastoreActionLabel(confirmation.action)} on {confirmation.store}?
          </span>
          <Button
            size="sm"
            variant="danger"
            onClick={() => void doAction(confirmation.store, confirmation.action)}
          >
            Confirm
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setConfirmation(null)}>
            Cancel
          </Button>
        </div>
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
                        onClick={() => setConfirmation({ store: ds.store, action: "gc" })}
                      >
                        Run GC
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!!actionInFlight}
                        loading={actionInFlight === `verify-${ds.store}`}
                        onClick={() => setConfirmation({ store: ds.store, action: "verify" })}
                      >
                        Verify
                      </Button>
                      {ds.maintenance_mode ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `maintenance-disable-${ds.store}`}
                          onClick={() => setConfirmation({ store: ds.store, action: "maintenance-disable" })}
                        >
                          Exit Maintenance
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === `maintenance-enable-${ds.store}`}
                          onClick={() => setConfirmation({ store: ds.store, action: "maintenance-enable" })}
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
