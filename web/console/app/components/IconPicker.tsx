"use client";

import { Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useMemo, useState } from "react";

interface IconPickerProps {
  selectedIcon: string;
  onSelect: (iconKey: string) => void;
  icons: string[];
}

const SHOWN = 200;

/** Built-in app icons with a search field. Picking the selected icon again clears it. */
export function IconPicker({ selectedIcon, onSelect, icons }: IconPickerProps) {
  const t = useTranslations("services.icons");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const q = search.toLowerCase().trim();
    return q ? icons.filter((key) => key.includes(q)) : icons;
  }, [icons, search]);

  const pick = useCallback((key: string) => onSelect(key === selectedIcon ? "" : key), [onSelect, selectedIcon]);

  return (
    <div className="sv-field">
      <label className="sv-filter sv-icsearch">
        <Search aria-hidden="true" strokeWidth={2} />
        <input type="search" value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t("search")} aria-label={t("search")} />
      </label>
      <div className="sv-icgrid tall">
        {filtered.slice(0, SHOWN).map((key) => (
          <button key={key} type="button" className={`sv-ictile${selectedIcon === key ? " on" : ""}`} onClick={() => pick(key)}
            aria-pressed={selectedIcon === key} aria-label={key} title={key}>
            <img src={`/service-icons/${key}.svg`} alt="" width={20} height={20} loading="lazy" decoding="async"
              onError={(e) => { (e.target as HTMLImageElement).style.visibility = "hidden"; }} />
          </button>
        ))}
      </div>
      {filtered.length > SHOWN && <p className="sv-hint">{t("more", { shown: SHOWN, total: filtered.length })}</p>}
      {filtered.length === 0 && <p className="sv-hint">{t("noMatch", { query: search })}</p>}
    </div>
  );
}
