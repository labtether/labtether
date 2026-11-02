"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";
import { Modal } from "../../../components/ui/Modal";
import { createComposite } from "../../../hooks/useComposites";
import type { Asset, Proposal } from "../../../console/models";

type Action = "merge" | "link" | "dismiss";

type LinkReviewPanelProps = {
  proposals: Proposal[];
  assets: Asset[];
  onAccept: (id: string) => Promise<void>;
  onDismiss: (id: string) => Promise<void>;
  onClose: () => void;
};

/**
 * Discovery's suggested relationships in a dialog: each pair with its match score and signals, then Dismiss, Link
 * (one runs on the other) and Merge (the same machine seen twice). A refused change shows one line and stays.
 */
export function LinkReviewPanel({ proposals, assets, onAccept, onDismiss, onClose }: LinkReviewPanelProps) {
  const t = useTranslations("devices.proposals");
  const tc = useTranslations("common");
  const [busy, setBusy] = useState<{ id: string; action: Action } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const nameOf = useMemo(() => {
    const names = new Map(assets.map((asset) => [asset.id, asset.name]));
    return (id: string) => names.get(id) ?? id;
  }, [assets]);

  const run = async (proposal: Proposal, action: Action) => {
    setBusy({ id: proposal.id, action });
    setError(null);
    try {
      if (action === "merge") {
        await createComposite(proposal.source_asset_id, [proposal.target_asset_id]);
        await onDismiss(proposal.id);
      } else if (action === "link") {
        await onAccept(proposal.id);
      } else {
        await onDismiss(proposal.id);
      }
    } catch {
      setError(t("failed"));
    } finally {
      setBusy((current) => (current?.id === proposal.id ? null : current));
    }
  };

  return (
    <Modal open onClose={onClose} title={t("title")}>
      <div className="kt-dlg-body">
        {error ? <p className="ld-wait-err" role="alert">{error}</p> : null}
        {proposals.length === 0 ? <p>{t("noneLeft")}</p> : null}
        {proposals.map((proposal) => {
          const isBusy = busy?.id === proposal.id;
          const signals = Object.entries(proposal.match_signals ?? {})
            .map(([key, value]) => (value === null || value === undefined || value === "" ? key : `${key} ${String(value)}`));
          const facts = [t("confidence", { value: Math.round(proposal.confidence * 100) })];
          if (signals.length > 0) facts.push(`${t("matchSignals")}: ${signals.join(", ")}`);
          return (
            <div key={proposal.id} className="ld-sugg">
              <p className="ld-sugg-nm">{t("pair", { a: nameOf(proposal.source_asset_id), b: nameOf(proposal.target_asset_id) })}</p>
              <p className="ld-wait-sub">{facts.join(" · ")}</p>
              <div className="ld-wait-acts">
                <Button variant="ghost" size="sm" disabled={isBusy} onClick={() => void run(proposal, "dismiss")}>
                  {t("dismiss")}
                </Button>
                <Button size="sm" disabled={isBusy} loading={isBusy && busy?.action === "link"} onClick={() => void run(proposal, "link")}>
                  {t("linkParentChild")}
                </Button>
                <Button size="sm" disabled={isBusy} loading={isBusy && busy?.action === "merge"} onClick={() => void run(proposal, "merge")}>
                  {t("mergeComposite")}
                </Button>
              </div>
            </div>
          );
        })}
        <div className="kt-dlg-acts">
          <Button onClick={onClose}>{tc("close")}</Button>
        </div>
      </div>
    </Modal>
  );
}
