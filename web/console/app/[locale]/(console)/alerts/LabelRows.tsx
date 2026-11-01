"use client";

import { Plus, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";
import { blankLabel, type LabelRow } from "./alertsPageTypes";

type Props = {
  legend: string;
  rows: LabelRow[];
  onRows: (rows: LabelRow[]) => void;
  hint?: string;
  error?: string;
};

/** label = value rows with Add and Remove, for silence matchers and rule labels. */
export function LabelRows({ legend, rows, onRows, hint, error }: Props) {
  const t = useTranslations("alerts.labelRows");
  const shown = rows.length ? rows : [blankLabel()];
  const edit = (i: number, part: Partial<LabelRow>) => onRows(shown.map((r, j) => (j === i ? { ...r, ...part } : r)));

  return (
    <fieldset className="al-timing al-wide">
      <legend className="al-lbl">{legend}</legend>
      <div className="al-matchers">
        {shown.map((r, i) => (
          <div key={i} className="al-matcher">
            <input className="al-in al-mono" aria-label={t("label")} placeholder="severity" value={r.key} autoComplete="off" spellCheck={false}
              aria-invalid={error && i === 0 ? true : undefined} onChange={(e) => edit(i, { key: e.target.value })} />
            <span className="al-unit">=</span>
            <input className="al-in al-mono" aria-label={t("value")} placeholder="low" value={r.value} autoComplete="off" spellCheck={false}
              onChange={(e) => edit(i, { value: e.target.value })} />
            {shown.length > 1 ? (
              <button type="button" className="al-btn icon quiet" aria-label={t("remove")} onClick={() => onRows(shown.filter((_, j) => j !== i))}>
                <X aria-hidden="true" strokeWidth={2} />
              </button>
            ) : null}
          </div>
        ))}
      </div>
      {error ? <p className="al-hint bad">{error}</p> : hint ? <p className="al-hint">{hint}</p> : null}
      <div className="al-addrow">
        <Button type="button" size="sm" variant="ghost" onClick={() => onRows([...shown, blankLabel()])}>
          <Plus aria-hidden="true" size={14} strokeWidth={2} />
          {t("add")}
        </Button>
      </div>
    </fieldset>
  );
}
