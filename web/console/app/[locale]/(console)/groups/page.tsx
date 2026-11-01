"use client";

import { useCallback, useMemo, useState } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal, Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import type { Group } from "../../../console/models";
import { useFastStatus, useSlowStatus, useStatusControls } from "../../../contexts/StatusContext";
import { GroupCreateDialog } from "./GroupCreateDialog";
import { GroupDeleteDialog } from "./GroupDeleteDialog";
import { GroupEditDialog } from "./GroupEditDialog";
import { GroupList, type GroupAction } from "./GroupList";
import { deviceNamesByGroup, ungroupedCount } from "./groupListModel";
import { useGroupMutationActions } from "./useGroupMutationActions";
import { useGroupTree } from "./useGroupTree";

type Dialog = { type: "create"; parentId: string } | { type: "edit" | "delete"; group: Group } | null;

/**
 * Settings › Groups: the group tree as a nested list. New group (or "New group inside" on a row) asks for a name, slug
 * and parent; a row's ⋯ edits the name, parent and jump hosts, or deletes the group.
 */
export default function GroupsPage() {
  const t = useTranslations("groups");
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const groups = useMemo(() => slowStatus?.groups ?? [], [slowStatus?.groups]);
  const assets = useMemo(() => status?.assets ?? [], [status?.assets]);
  const { createGroup, updateGroup, deleteGroup } = useGroupMutationActions();
  const [dialog, setDialog] = useState<Dialog>(null);
  const close = useCallback(() => setDialog(null), []);

  const namesByGroup = useMemo(() => deviceNamesByGroup(assets), [assets]);
  const deviceCountByGroup = useMemo(
    () => new Map([...namesByGroup].map(([id, names]) => [id, names.length])),
    [namesByGroup],
  );
  const { tree, toggleExpand, expandAll, collapseAll } = useGroupTree({ groups, deviceCountByGroup });
  const ungrouped = useMemo(() => ungroupedCount(assets, groups), [assets, groups]);

  const onAction = (action: GroupAction, group: Group) =>
    setDialog(action === "child" ? { type: "create", parentId: group.id } : { type: action, group });
  const target = (type: "edit" | "delete") => (dialog && dialog.type === type ? dialog.group : null);
  const deleting = target("delete");

  return (
    <>
      <PageHeader
        title={t("title")}
        action={(
          <div className="pp-head-acts">
            {groups.length > 0 ? (
              <Menu.Root>
                <Menu.Trigger className="pp-more pp-shown pp-ctl" aria-label={t("list.more")}>
                  <MoreHorizontal aria-hidden="true" />
                </Menu.Trigger>
                <Menu.Portal>
                  <Menu.Content className="lt-menu" align="end" sideOffset={4}>
                    <Menu.Item className="lt-menu-item" onSelect={expandAll}>{t("list.expandAll")}</Menu.Item>
                    <Menu.Item className="lt-menu-item" onSelect={collapseAll}>{t("list.collapseAll")}</Menu.Item>
                  </Menu.Content>
                </Menu.Portal>
              </Menu.Root>
            ) : null}
            <Button variant="primary" onClick={() => setDialog({ type: "create", parentId: "" })}>
              <Plus size={14} aria-hidden="true" />
              {t("newGroup")}
            </Button>
          </div>
        )}
      />

      {groups.length === 0 ? (
        <div className="pp-list">
          <p className="pp-empty">{t("list.empty")}</p>
        </div>
      ) : (
        <GroupList
          tree={tree}
          namesByGroup={namesByGroup}
          ungrouped={ungrouped}
          onToggle={toggleExpand}
          onAction={onAction}
        />
      )}

      <GroupCreateDialog
        open={dialog?.type === "create"}
        parentId={dialog?.type === "create" ? dialog.parentId : ""}
        groups={groups}
        onClose={close}
        onCreate={async (input) => {
          await createGroup(input);
          await fetchStatus();
          close();
        }}
      />
      <GroupEditDialog
        group={target("edit")}
        groups={groups}
        onClose={close}
        onUpdate={updateGroup}
        onSaved={fetchStatus}
      />
      <GroupDeleteDialog
        group={deleting}
        groups={groups}
        ownDevices={deleting ? namesByGroup.get(deleting.id)?.length ?? 0 : 0}
        onClose={close}
        onDelete={async (group) => {
          await deleteGroup(group.id);
          await fetchStatus();
          close();
        }}
      />
    </>
  );
}
