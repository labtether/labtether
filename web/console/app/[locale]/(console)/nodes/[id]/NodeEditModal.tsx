"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { Asset, Group } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Modal } from "../../../../components/ui/Modal";
import { GroupParentSelect } from "../../../../components/GroupParentSelect";

type NodeEditModalProps = {
  asset: Asset;
  groups: Group[];
  onClose: () => void;
  onSave: () => void;
};

/** The hub's error text from a failed response, without status codes; "" when it sent none. */
async function errorText(res: Response): Promise<string> {
  const text = await res.text().catch(() => "");
  if (!text) return "";
  try {
    const body = JSON.parse(text) as { error?: string };
    return body.error ?? "";
  } catch {
    return text.length < 200 ? text : "";
  }
}

/** Rename the device and set its group (Edit… in the device ⋯). */
export function NodeEditModal({ asset, groups, onClose, onSave }: NodeEditModalProps) {
  const t = useTranslations("devices.page.edit");
  const [name, setName] = useState(asset.name);
  const [groupId, setGroupId] = useState(asset.group_id ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = async () => {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError(t("nameRequired"));
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
        const reason = await errorText(res);
        setError(reason ? t("failedWith", { reason }) : t("failed"));
        return;
      }
      onSave();
      onClose();
    } catch {
      setError(t("failed"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open onClose={saving ? () => {} : onClose} title={t("title", { name: asset.name })}>
      <form
        className="kt-dlg-body dv-form"
        onSubmit={(event) => {
          event.preventDefault();
          void handleSave();
        }}
      >
        <div className="dv-field">
          <label htmlFor="dv-edit-name">{t("name")}</label>
          <input
            id="dv-edit-name"
            className="dv-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={saving}
            autoComplete="off"
            spellCheck={false}
            autoFocus
            aria-invalid={error === t("nameRequired") ? true : undefined}
          />
        </div>
        <GroupParentSelect groups={groups} value={groupId} onChange={setGroupId} disabled={saving} label={t("group")} />
        {error ? <p className="dv-err" role="alert">{error}</p> : null}
        <div className="kt-dlg-acts">
          <Button type="button" onClick={onClose} disabled={saving}>{t("cancel")}</Button>
          <Button type="submit" variant="primary" loading={saving}>{t("save")}</Button>
        </div>
      </form>
    </Modal>
  );
}
