"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { Asset, Group } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { Modal } from "../../../../components/ui/Modal";
import { GroupParentSelect } from "../../../../components/GroupParentSelect";
import { draftOf, useDeviceEdit, type DeviceEditResult } from "./deviceEdit";

type NodeEditModalProps = {
  asset: Asset;
  groups: Group[];
  onClose: () => void;
  onSave: () => void;
};

/** The words for a failed save: an empty name, or the Hub's reason when it gave one. */
export function editErrorText(t: (key: string, values?: Record<string, string>) => string, result: DeviceEditResult): string | null {
  if (result.ok) return null;
  if (result.error === "nameRequired") return t("nameRequired");
  return result.reason ? t("failedWith", { reason: result.reason }) : t("failed");
}

/** Rename the device, set its group and its tags (Edit… in the device ⋯). */
export function NodeEditModal({ asset, groups, onClose, onSave }: NodeEditModalProps) {
  const t = useTranslations("devices.page.edit");
  const [draft, setDraft] = useState(() => draftOf(asset));
  const [error, setError] = useState<string | null>(null);
  const { save, saving } = useDeviceEdit(asset);

  const handleSave = async () => {
    setError(null);
    const result = await save(draft);
    if (!result.ok) {
      setError(editErrorText(t, result));
      return;
    }
    if (result.changed) onSave();
    onClose();
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
            value={draft.name}
            onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
            disabled={saving}
            autoComplete="off"
            spellCheck={false}
            autoFocus
            aria-invalid={error === t("nameRequired") ? true : undefined}
          />
        </div>
        <GroupParentSelect groups={groups} value={draft.groupId} onChange={(groupId) => setDraft((d) => ({ ...d, groupId }))} disabled={saving} label={t("group")} />
        <div className="dv-field">
          <label htmlFor="dv-edit-tags">{t("tags")}</label>
          <input
            id="dv-edit-tags"
            className="dv-input"
            value={draft.tags}
            onChange={(e) => setDraft((d) => ({ ...d, tags: e.target.value }))}
            disabled={saving}
            autoComplete="off"
            spellCheck={false}
            aria-describedby="dv-edit-tags-h"
          />
          <p className="dv-hint" id="dv-edit-tags-h">{t("tagsHint")}</p>
        </div>
        {error ? <p className="dv-err" role="alert">{error}</p> : null}
        <div className="kt-dlg-acts">
          <Button type="button" onClick={onClose} disabled={saving}>{t("cancel")}</Button>
          <Button type="submit" variant="primary" loading={saving}>{t("save")}</Button>
        </div>
      </form>
    </Modal>
  );
}
