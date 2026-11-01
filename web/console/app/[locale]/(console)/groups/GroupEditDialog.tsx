"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { GroupParentSelect } from "../../../components/GroupParentSelect";
import { JumpChainEditor } from "../../../components/JumpChainEditor";
import { Input } from "../../../components/ui/Input";
import type { Group, HopConfig } from "../../../console/models";
import { Field, PeopleDialog, errorText } from "../users/PeopleDialog";
import { apiMoveGroup, computeGroupDepth, NESTING_DEPTH_WARN } from "./groupOperations";
import type { UpdateGroupInput } from "./useGroupMutationActions";

type GroupEditDialogProps = {
  group: Group | null;
  groups: Group[];
  onClose: () => void;
  onUpdate: (id: string, patch: UpdateGroupInput) => Promise<void>;
  onSaved: () => Promise<void>;
};

/** Edit a group: its name, the group it sits inside, and the hosts its devices are reached through. */
export function GroupEditDialog({ group, groups, onClose, onUpdate, onSaved }: GroupEditDialogProps) {
  const t = useTranslations("groups");
  const [name, setName] = useState("");
  const [parent, setParent] = useState("");
  const [hops, setHops] = useState<HopConfig[]>([]);
  const [error, setError] = useState<{ name?: string; hub?: string }>({});
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!group) return;
    setName(group.name);
    setParent(group.parent_group_id ?? "");
    setHops(group.jump_chain?.hops ?? []);
    setError({});
    setSaving(false);
  }, [group]);

  const deep = parent ? computeGroupDepth(groups, parent) + 1 >= NESTING_DEPTH_WARN : false;

  const submit = async () => {
    if (!group) return;
    const cleanName = name.trim();
    if (!cleanName) {
      setError({ name: t("errors.nameEmpty") });
      return;
    }
    setSaving(true);
    setError({});
    try {
      const before = group.jump_chain?.hops ?? [];
      const patch: UpdateGroupInput = {};
      if (cleanName !== group.name) patch.name = cleanName;
      if (JSON.stringify(hops) !== JSON.stringify(before)) patch.jump_chain = hops.length > 0 ? { hops } : null;
      if (Object.keys(patch).length > 0) await onUpdate(group.id, patch);
      if ((parent || null) !== (group.parent_group_id ?? null)) await apiMoveGroup(group.id, parent || null);
      await onSaved();
      onClose();
    } catch (err) {
      setError({ hub: errorText(err, t("errors.saveFailed")) });
      setSaving(false);
      await onSaved().catch(() => undefined);
    }
  };

  return (
    <PeopleDialog
      open={Boolean(group)}
      title={t("edit.title", { name: group?.name ?? "" })}
      onClose={onClose}
      onSubmit={() => { void submit(); }}
      submitLabel={t("edit.submit")}
      busy={saving}
      error={error.hub}
      wide
    >
      <div className="pp-grid2">
        <Field id="pp-ge-name" label={t("form.name")} error={error.name}>
          <Input
            id="pp-ge-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoComplete="off"
            disabled={saving}
            error={Boolean(error.name)}
            aria-invalid={error.name ? true : undefined}
            aria-describedby={error.name ? "pp-ge-name-err" : undefined}
          />
        </Field>
        <GroupParentSelect
          id="pp-ge-parent"
          groups={groups}
          value={parent}
          onChange={setParent}
          excludeGroupId={group?.id}
          disabled={saving}
          label={t("form.inside")}
          noneLabel={t("form.topLevel")}
        />
      </div>
      {deep ? <p className="pp-note">{t("form.deep", { levels: NESTING_DEPTH_WARN })}</p> : null}
      <JumpChainEditor value={hops} onChange={setHops} disabled={saving} />
    </PeopleDialog>
  );
}
