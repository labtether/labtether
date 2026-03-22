"use client";

import { useState } from "react";
import { Button } from "../../../../components/ui/Button";
import type { WebService } from "../../../../hooks/useWebServices";

const CATEGORIES = [
  "dashboard", "monitoring", "storage", "media", "network",
  "automation", "development", "security", "database", "other",
];

type WebServiceFormProps = {
  initial?: WebService;
  onSave: (data: { name: string; url: string; category: string }) => void | Promise<void>;
  onCancel: () => void;
};

export function WebServiceForm({ initial, onSave, onCancel }: WebServiceFormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [url, setUrl] = useState(initial?.url ?? "");
  const [category, setCategory] = useState(initial?.category ?? "other");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    setError(null);
    if (!name.trim()) { setError("Name is required."); return; }
    if (!url.trim()) { setError("URL is required."); return; }
    try {
      new URL(url);
    } catch {
      setError("URL must be a valid URL (e.g., https://10.0.0.5:9443).");
      return;
    }
    setSaving(true);
    try {
      await onSave({ name: name.trim(), url: url.trim(), category });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-3">
      {error && <p className="text-xs text-[var(--bad)]">{error}</p>}
      <div>
        <label className="text-xs text-[var(--muted)] block mb-1">Name</label>
        <input
          type="text"
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="e.g., Portainer"
          className="w-full rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)]"
        />
      </div>
      <div>
        <label className="text-xs text-[var(--muted)] block mb-1">URL</label>
        <input
          type="text"
          value={url}
          onChange={e => setUrl(e.target.value)}
          placeholder="e.g., https://10.0.0.5:9443"
          className="w-full rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)]"
        />
      </div>
      <div>
        <label className="text-xs text-[var(--muted)] block mb-1">Category</label>
        <select
          value={category}
          onChange={e => setCategory(e.target.value)}
          className="w-full rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-sm text-[var(--text)]"
        >
          {CATEGORIES.map(c => (
            <option key={c} value={c}>{c.charAt(0).toUpperCase() + c.slice(1)}</option>
          ))}
        </select>
      </div>
      <div className="flex justify-end gap-2 pt-1">
        <Button size="sm" variant="ghost" onClick={onCancel} disabled={saving}>Cancel</Button>
        <Button size="sm" onClick={() => void handleSubmit()} disabled={saving}>
          {saving ? "Saving..." : initial ? "Update" : "Add"}
        </Button>
      </div>
    </div>
  );
}
