"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import type { CSSProperties } from "react";
import { Button } from "../../../components/ui/Button";
import type { WebService } from "../../../hooks/useWebServices";
import { isHidden } from "./servicesListModel";

const SKELETON_WIDTHS = [120, 148, 104, 136, 112, 156];

/** Placeholder rows while the first list loads. */
export function ServicesSkeleton() {
  const t = useTranslations("services.list");
  return (
    <div className="sv-list" aria-busy="true" aria-label={t("loading")}>
      {SKELETON_WIDTHS.map((w) => (
        <div key={w} className="sv-row skel" aria-hidden="true">
          <span />
          <i className="sv-skel" style={{ "--w": `${w}px` } as CSSProperties} />
        </div>
      ))}
    </div>
  );
}

/** No services at all yet: one line and Add service. */
export function ServicesEmpty({ adding, onAdd }: { adding: boolean; onAdd: () => void }) {
  const t = useTranslations("services.list");
  return (
    <div className="sv-sheet sv-empty">
      <b>{t("emptyTitle")}</b>
      <span>{t("emptyBody")}</span>
      {!adding && (
        <Button variant="primary" onClick={onAdd}>
          <Plus size={14} aria-hidden="true" />
          {t("emptyAdd")}
        </Button>
      )}
    </div>
  );
}

/** Floats at the bottom of the list while rows are picked: Edit…, Hide or Show, Clear. */
export function ServicesBulkBar({ picked, busy, onEdit, onHide, onClear }: {
  picked: WebService[];
  busy: boolean;
  onEdit: () => void;
  onHide: (hidden: boolean) => void;
  onClear: () => void;
}) {
  const t = useTranslations("services.bulk");
  if (picked.length === 0) return null;
  const allHidden = picked.every(isHidden);
  return (
    <div className="sv-bulk" role="region" aria-label={t("label")}>
      <span className="sv-bulk-n">{t.rich("selected", { count: picked.length, b: (chunks) => <b>{chunks}</b> })}</span>
      <Button size="sm" onClick={onEdit} disabled={busy}>{t("edit")}</Button>
      <Button size="sm" onClick={() => onHide(!allHidden)} disabled={busy}>{allHidden ? t("show") : t("hide")}</Button>
      <Button size="sm" variant="ghost" onClick={onClear}>{t("clear")}</Button>
    </div>
  );
}
