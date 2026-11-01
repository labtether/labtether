"use client";

import { Search, X } from "lucide-react";
import type { MapWords } from "./useMapWords";

/** Find on the map (⋯ or ⌘F): matches stand out and the rest dims; Enter opens the first match, Esc closes. */
export function MapFind({ value, words, none, onChange, onClose, onPick }: {
  value: string;
  words: MapWords;
  /** Text is typed and nothing matches. */
  none: boolean;
  onChange: (v: string) => void;
  onClose: () => void;
  onPick: () => void;
}) {
  const { t } = words;
  return (
    <div className="mp-find" role="search">
      <Search aria-hidden="true" width={14} height={14} />
      <input
        autoFocus
        value={value}
        placeholder={t("find.placeholder")}
        aria-label={t("find.label")}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") { e.preventDefault(); onPick(); }
          if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onClose(); }
        }}
      />
      {none && <span className="mp-find-none" role="status">{t("find.none")}</span>}
      <button type="button" className="mp-x" onClick={onClose} aria-label={t("find.close")}>
        <X aria-hidden="true" width={14} height={14} strokeWidth={2} />
      </button>
    </div>
  );
}
