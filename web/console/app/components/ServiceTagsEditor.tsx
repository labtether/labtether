"use client";

import { X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";

/** Tags as a comma-separated value: shown as small tags with a remove button, added with Enter or Add. */
export function TagsEditor({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const t = useTranslations("services.edit");
  const tags = value.split(",").map((tag) => tag.trim()).filter(Boolean);
  const [draft, setDraft] = useState("");

  function addTag() {
    const next = draft.trim().toLowerCase();
    if (next && !tags.includes(next)) onChange([...tags, next].join(", "));
    setDraft("");
  }

  return (
    <div className="sv-tagsed">
      {tags.map((tag) => (
        <span key={tag} className="sv-tag">
          {tag}
          <button type="button" onClick={() => onChange(tags.filter((x) => x !== tag).join(", "))} aria-label={t("tagRemove", { tag })}>
            <X aria-hidden="true" strokeWidth={2.4} />
          </button>
        </span>
      ))}
      <input
        className="sv-input"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault();
            addTag();
          }
        }}
        onBlur={addTag}
        placeholder={t("tagAdd")}
        aria-label={t("tagAdd")}
      />
    </div>
  );
}
