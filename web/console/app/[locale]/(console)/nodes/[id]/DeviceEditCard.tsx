"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import type { Asset, Group } from "../../../../console/models";
import { Button } from "../../../../components/ui/Button";
import { GroupParentSelect } from "../../../../components/GroupParentSelect";
import { draftOf, deviceEditPatch, mergeDraft, useDeviceEdit } from "./deviceEdit";
import { editErrorText } from "./NodeEditModal";

/**
 * Settings › Device for a device added by hand: name, group and tags, saved together. An agent device keeps its
 * name in the agent's own card; every device can also be edited from its ⋯ › Edit….
 */
export function DeviceEditCard({ asset, groups, onSaved }: { asset: Asset; groups: Group[]; onSaved: () => void }) {
  const t = useTranslations("devices.page.edit");
  const tc = useTranslations("devices.settingsTab.device");
  const [draft, setDraft] = useState(() => draftOf(asset));
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const { save, saving } = useDeviceEdit(asset);

  // A change from the Hub (a save, or another person's edit) arrives with a later status. Take it in every field
  // the person has not changed; keep the ones they are typing. A status refresh from the previous save can land
  // while they type the next change.
  const base = useRef(draftOf(asset));
  const key = `${asset.name}|${asset.group_id ?? ""}|${(asset.tags ?? []).join(",")}`;
  useEffect(() => {
    const next = draftOf(asset);
    setDraft((current) => mergeDraft(current, base.current, next));
    base.current = next;
  }, [key]); // eslint-disable-line react-hooks/exhaustive-deps

  const changed = deviceEditPatch(asset, draft) !== null;
  const submit = async () => {
    setError(null);
    setSaved(false);
    const result = await save(draft);
    if (!result.ok) {
      setError(editErrorText(t, result));
      return;
    }
    if (result.changed) {
      setSaved(true);
      onSaved();
    }
  };

  return (
    <section className="dv-sec" aria-labelledby="dst-dev-t">
      <div className="dv-sh"><h2 id="dst-dev-t">{tc("title")}</h2></div>
      <form
        className="dv-sheet dv-form"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <div className="dv-field">
          <label htmlFor="dst-dev-name">{t("name")}</label>
          <input
            id="dst-dev-name"
            className="dv-input"
            value={draft.name}
            onChange={(e) => { setSaved(false); setDraft((d) => ({ ...d, name: e.target.value })); }}
            disabled={saving}
            autoComplete="off"
            spellCheck={false}
            aria-invalid={error === t("nameRequired") ? true : undefined}
          />
        </div>
        <GroupParentSelect
          groups={groups}
          value={draft.groupId}
          onChange={(groupId) => { setSaved(false); setDraft((d) => ({ ...d, groupId })); }}
          disabled={saving}
          label={t("group")}
        />
        <div className="dv-field">
          <label htmlFor="dst-dev-tags">{t("tags")}</label>
          <input
            id="dst-dev-tags"
            className="dv-input"
            value={draft.tags}
            onChange={(e) => { setSaved(false); setDraft((d) => ({ ...d, tags: e.target.value })); }}
            disabled={saving}
            autoComplete="off"
            spellCheck={false}
            aria-describedby="dst-dev-tags-h"
          />
          <p className="dv-hint" id="dst-dev-tags-h">{t("tagsHint")}</p>
        </div>
        {error ? <p className="dv-err" role="alert">{error}</p> : null}
        {saved ? <p className="dv-ok-text" role="status">{tc("saved")}</p> : null}
        <div>
          <Button type="submit" disabled={saving || !changed}>{saving ? tc("saving") : t("save")}</Button>
        </div>
      </form>
    </section>
  );
}
