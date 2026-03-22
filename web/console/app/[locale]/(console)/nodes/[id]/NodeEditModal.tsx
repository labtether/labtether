"use client";

import { useEffect, useState } from "react";
import type { Asset, Group } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input } from "../../../../components/ui/Input";
import { GroupParentSelect } from "../../../../components/GroupParentSelect";

type NodeEditModalProps = {
  asset: Asset;
  groups: Group[];
  onClose: () => void;
  onSave: () => void;
};

export function NodeEditModal({ asset, groups, onClose, onSave }: NodeEditModalProps) {
  const [name, setName] = useState(asset.name);
  const [groupId, setGroupId] = useState(asset.group_id ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !saving) {
        e.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [saving, onClose]);

  const handleSave = async () => {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Device name cannot be empty.");
      return;
    }
    const patch: Record<string, string | null> = {};
    if (trimmedName !== asset.name) patch.name = trimmedName;
    if (groupId !== (asset.group_id ?? "")) patch.group_id = groupId || null;

    if (Object.keys(patch).length === 0) {
      onClose();
      return;
    }

    setSaving(true);
    setError(null);

    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(asset.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
      });

      if (!res.ok) {
        let msg = `Request failed (${res.status})`;
        const text = await res.text().catch(() => "");
        if (text) {
          try {
            const body = JSON.parse(text) as { error?: string };
            if (body.error) msg = body.error;
            else msg = text;
          } catch {
            msg = text;
          }
        }
        setError(msg);
        return;
      }

      onSave();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unexpected error");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={saving ? undefined : onClose}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[34rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">Edit Device</h3>

          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">Device Name</span>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Device name"
              disabled={saving}
              autoFocus
            />
          </label>

          <GroupParentSelect
            groups={groups}
            value={groupId}
            onChange={setGroupId}
            disabled={saving}
            label="Group"
          />

          {error && <p className="text-xs text-[var(--bad)]">{error}</p>}

          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={saving}>
              Cancel
            </Button>
            <Button variant="primary" onClick={() => { void handleSave(); }} disabled={saving} loading={saving}>
              Save
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
