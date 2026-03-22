"use client";

import Image from "next/image";
import { useState, useMemo, useCallback } from "react";
import { Search, Check } from "lucide-react";

interface IconPickerProps {
  selectedIcon: string;
  onSelect: (iconKey: string) => void;
  icons: string[];
}

export function IconPicker({ selectedIcon, onSelect, icons }: IconPickerProps) {
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!search.trim()) return icons;
    const q = search.toLowerCase().trim();
    return icons.filter((key) => key.includes(q));
  }, [icons, search]);

  const handleSelect = useCallback(
    (key: string) => {
      onSelect(key === selectedIcon ? "" : key);
    },
    [onSelect, selectedIcon]
  );

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search
          size={14}
          className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--muted)]"
        />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search icons..."
          className="w-full h-8 pl-8 pr-3 rounded border border-[var(--line)] bg-[var(--surface)] text-[13px] text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:border-[var(--accent)]"
        />
      </div>
      <div className="grid grid-cols-8 gap-1 max-h-[240px] overflow-y-auto p-1">
        {filtered.slice(0, 200).map((key) => (
          <button
            key={key}
            type="button"
            onClick={() => handleSelect(key)}
            title={key}
            className={`relative w-9 h-9 rounded flex items-center justify-center transition-colors duration-[var(--dur-fast)] cursor-pointer ${
              selectedIcon === key
                ? "bg-[var(--accent)]/20 border border-[var(--accent)]"
                : "hover:bg-[var(--hover)] border border-transparent"
            }`}
          >
            <Image
              src={`/service-icons/${key}.svg`}
              alt={key}
              width={24}
              height={24}
              unoptimized
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = "none";
              }}
            />
            {selectedIcon === key && (
              <Check
                size={10}
                className="absolute top-0.5 right-0.5 text-[var(--accent)]"
              />
            )}
          </button>
        ))}
      </div>
      {filtered.length > 200 && (
        <p className="text-xs text-[var(--muted)] text-center">
          Showing 200 of {filtered.length} — refine your search
        </p>
      )}
      {filtered.length === 0 && (
        <p className="text-xs text-[var(--muted)] text-center py-4">
          No icons match &ldquo;{search}&rdquo;
        </p>
      )}
    </div>
  );
}
