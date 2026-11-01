"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import type { Group } from "../../../console/models";
import { PeopleDialog, errorText } from "../users/PeopleDialog";
import { groupPath } from "./groupListModel";

type GroupDeleteDialogProps = {
  group: Group | null;
  groups: Group[];
  ownDevices: number;
  onClose: () => void;
  onDelete: (group: Group) => Promise<void>;
};

/**
 * Delete a group and say what changes, as the hub does it: its devices become ungrouped and the groups inside it
 * move up into its parent.
 */
export function GroupDeleteDialog({ group, groups, ownDevices, onClose, onDelete }: GroupDeleteDialogProps) {
  const t = useTranslations("groups");
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    setError("");
    setDeleting(false);
  }, [group]);

  const children = group ? groups.filter((g) => g.parent_group_id === group.id).map((g) => g.name) : [];
  const parent = group?.parent_group_id ? groupPath(groups, group.parent_group_id) : "";

  const submit = async () => {
    if (!group) return;
    setDeleting(true);
    setError("");
    try {
      await onDelete(group);
    } catch (err) {
      setError(errorText(err, t("errors.deleteFailed")));
      setDeleting(false);
    }
  };

  return (
    <PeopleDialog
      open={Boolean(group)}
      title={t("delete.title", { name: group?.name ?? "" })}
      onClose={onClose}
      onSubmit={() => { void submit(); }}
      submitLabel={t("delete.submit")}
      tone="danger"
      busy={deleting}
      error={error}
    >
      <p className="pp-say">
        {ownDevices > 0 ? t("delete.devices", { count: ownDevices }) : t("delete.noDevices")}
        {children.length > 0 ? ` ${parent
          ? t("delete.childrenInto", { names: children.join(", "), count: children.length, parent })
          : t("delete.childrenTop", { names: children.join(", "), count: children.length })}` : ""}
      </p>
    </PeopleDialog>
  );
}
