"use client";

import { useCallback, useMemo, useState } from "react";
import { ArrowLeftRight, Check, GitMerge, Link2, X } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { createComposite } from "../../../hooks/useComposites";
import type { Asset, Proposal } from "../../../console/models";

type BusyState = { id: string; action: "merge" | "link" | "dismiss" } | null;

type LinkReviewPanelProps = {
  proposals: Proposal[];
  assets: Asset[];
  onAccept: (id: string) => Promise<void>;
  onDismiss: (id: string) => Promise<void>;
  onClose: () => void;
};

export function LinkReviewPanel({
  proposals,
  assets,
  onAccept,
  onDismiss,
  onClose,
}: LinkReviewPanelProps) {
  const [busy, setBusy] = useState<BusyState>(null);

  const assetNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const asset of assets) {
      map.set(asset.id, asset.name);
    }
    return map;
  }, [assets]);

  const resolveAssetName = useCallback(
    (id: string) => assetNameById.get(id) ?? id,
    [assetNameById],
  );

  const handleMerge = useCallback(
    async (proposal: Proposal) => {
      setBusy({ id: proposal.id, action: "merge" });
      try {
        await createComposite(proposal.source_asset_id, [proposal.target_asset_id]);
        // After merging, dismiss the proposal so it leaves the list
        await onDismiss(proposal.id);
      } catch {
        // Silently ignore — user can retry
      } finally {
        setBusy((current) =>
          current?.id === proposal.id ? null : current,
        );
      }
    },
    [onDismiss],
  );

  const handleLink = useCallback(
    async (id: string) => {
      setBusy({ id, action: "link" });
      try {
        await onAccept(id);
      } finally {
        setBusy((current) => (current?.id === id ? null : current));
      }
    },
    [onAccept],
  );

  const handleDismiss = useCallback(
    async (id: string) => {
      setBusy({ id, action: "dismiss" });
      try {
        await onDismiss(id);
      } finally {
        setBusy((current) => (current?.id === id ? null : current));
      }
    },
    [onDismiss],
  );

  const confidenceColor = (confidence: number) => {
    if (confidence >= 0.8)
      return "text-[var(--ok)] border-[var(--ok)]/40 bg-[var(--ok-glow)]";
    if (confidence >= 0.5)
      return "text-[var(--warn)] border-[var(--warn)]/40 bg-[var(--warn-glow)]";
    return "text-[var(--muted)] border-[var(--line)] bg-[var(--surface)]";
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-end"
      onClick={onClose}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50" />

      {/* Panel */}
      <div
        className="relative z-10 h-full w-full max-w-lg overflow-y-auto border-l border-[var(--panel-border)] bg-[var(--bg)] p-6"
        style={{
          backdropFilter: "blur(var(--blur-md)) saturate(1.5)",
          WebkitBackdropFilter: "blur(var(--blur-md)) saturate(1.5)",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <Link2 size={18} className="text-[var(--accent)]" />
            <h2 className="text-lg font-semibold text-[var(--text)]">
              Review Suggested Relationships
            </h2>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-[var(--muted)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--text)] cursor-pointer"
            aria-label="Close review panel"
          >
            <X size={18} />
          </button>
        </div>

        {proposals.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <div
              className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-[var(--ok)]/10"
              style={{ boxShadow: "0 0 20px var(--ok-glow)" }}
            >
              <Check size={24} className="text-[var(--ok)]" />
            </div>
            <h3 className="text-sm font-semibold text-[var(--text)]">
              All done!
            </h3>
            <p className="mt-1 text-xs text-[var(--muted)]">
              No pending relationship proposals to review.
            </p>
          </div>
        ) : (
          <div className="space-y-3">
            {proposals.map((proposal) => {
              const isBusy = busy?.id === proposal.id;
              const confidencePct = Math.round(proposal.confidence * 100);
              const signals = proposal.match_signals
                ? Object.entries(proposal.match_signals)
                : [];

              return (
                <Card key={proposal.id}>
                  <div className="space-y-3">
                    {/* Header row */}
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-medium text-[var(--muted)]">
                        Suggested Relationship
                      </span>
                      <span
                        className={`rounded-lg border px-2 py-0.5 text-xs font-semibold ${confidenceColor(proposal.confidence)}`}
                      >
                        {confidencePct}%
                      </span>
                    </div>

                    {/* Assets */}
                    <div className="flex items-center gap-2 text-sm">
                      <span className="font-medium text-[var(--text)]">
                        {resolveAssetName(proposal.source_asset_id)}
                      </span>
                      <ArrowLeftRight
                        size={14}
                        className="shrink-0 text-[var(--accent)]"
                      />
                      <span className="font-medium text-[var(--text)]">
                        {resolveAssetName(proposal.target_asset_id)}
                      </span>
                    </div>

                    {/* Match signals */}
                    {signals.length > 0 && (
                      <div className="space-y-1">
                        <span className="text-xs font-medium text-[var(--muted)]">
                          Match signals
                        </span>
                        <div className="flex flex-wrap gap-1.5">
                          {signals.map(([key, value]) => (
                            <span
                              key={key}
                              className="rounded border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-xs text-[var(--muted)]"
                            >
                              <span className="text-[var(--text)]">{key}</span>
                              {value !== null && value !== undefined && value !== "" && (
                                <>
                                  <span className="mx-0.5 opacity-40">=</span>
                                  <span>{String(value)}</span>
                                </>
                              )}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}

                    {/* Actions */}
                    <div className="flex items-center justify-end gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={isBusy}
                        onClick={() => void handleDismiss(proposal.id)}
                      >
                        Dismiss
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={isBusy}
                        loading={isBusy && busy?.action === "link"}
                        onClick={() => void handleLink(proposal.id)}
                      >
                        <Link2 size={13} />
                        Link
                      </Button>
                      <Button
                        variant="primary"
                        size="sm"
                        disabled={isBusy}
                        loading={isBusy && busy?.action === "merge"}
                        onClick={() => void handleMerge(proposal)}
                      >
                        <GitMerge size={13} />
                        Merge
                      </Button>
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
