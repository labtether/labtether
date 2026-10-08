"use client";

import { useState } from "react";
import { Plus, X } from "lucide-react";

export function TagsEditor({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const tags = value
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
  const [draft, setDraft] = useState("");

  function addTag() {
    const trimmed = draft.trim().toLowerCase();
    if (!trimmed) return;
    if (tags.includes(trimmed)) {
      setDraft("");
      return;
    }
    onChange([...tags, trimmed].join(", "));
    setDraft("");
  }

  function removeTag(tag: string) {
    onChange(tags.filter((t) => t !== tag).join(", "));
  }

  return (
    <div>
      {tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-2">
          {tags.map((tag) => (
            <span
              key={tag}
              className="inline-flex items-center gap-1 h-5 px-2 rounded-full bg-[var(--accent)]/15 border border-[var(--accent)]/25 text-[10px] font-medium text-[var(--accent)]"
            >
              {tag}
              <button
                type="button"
                onClick={() => removeTag(tag)}
                className="hover:text-[var(--bad)] transition-colors cursor-pointer"
              >
                <X size={10} />
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="flex items-center gap-1.5">
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addTag();
            }
          }}
          placeholder="Add tag..."
          className="flex-1 h-7 px-2.5 rounded border border-[var(--line)] bg-[var(--surface)] text-[12px] text-[var(--text)] focus:outline-none focus:border-[var(--accent)]"
        />
        <button
          type="button"
          onClick={addTag}
          className="h-7 w-7 rounded border border-[var(--line)] hover:bg-[var(--hover)] transition-colors cursor-pointer inline-flex items-center justify-center"
        >
          <Plus size={12} className="text-[var(--muted)]" />
        </button>
      </div>
    </div>
  );
}
