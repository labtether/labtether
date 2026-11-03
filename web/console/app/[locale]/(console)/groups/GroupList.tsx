"use client";

import type { CSSProperties } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronRight, MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import type { Group } from "../../../console/models";
import { firstNames, hopsLine, visibleRows } from "./groupListModel";
import type { GroupTreeItem } from "./useGroupTree";
import { useCanWrite } from "../../../hooks/useCanWrite";

export type GroupAction = "edit" | "child" | "delete";

type GroupListProps = {
  tree: GroupTreeItem[];
  namesByGroup: Map<string, string[]>;
  ungrouped: number;
  onToggle: (id: string) => void;
  onAction: (action: GroupAction, group: Group) => void;
};

/**
 * The group tree as one 2.0 list: children indent under their parent, a group with children folds with its chevron.
 * The grey line names the group's own devices, its place and the hosts its devices are reached through.
 */
export function GroupList({ tree, namesByGroup, ungrouped, onToggle, onAction }: GroupListProps) {
  const t = useTranslations("groups");
  const canWrite = useCanWrite();
  const rows = visibleRows(tree);

  const line = (item: GroupTreeItem) => {
    const { shown, more } = firstNames(namesByGroup.get(item.group.id) ?? []);
    const devices = shown.length ? (more ? t("list.namesMore", { names: shown.join(", "), count: more }) : shown.join(", ")) : t("list.noDevices");
    const hops = hopsLine(item.group);
    return [devices, item.group.location ?? "", hops ? t("list.through", { hosts: hops }) : ""].filter(Boolean).join(" · ");
  };

  return (
    <div className="pp-list pp-hang" role="treegrid" aria-label={t("title")}>
      <div className="pp-cols pp-gr" role="row">
        <span role="columnheader">{t("list.group")}</span>
        <span role="columnheader" className="pp-r">{t("list.devices")}</span>
        <span role="columnheader"><span className="sr-only">{t("list.actions")}</span></span>
      </div>
      {rows.map((item) => {
        const { group } = item;
        const parent = item.children.length > 0;
        return (
          <div
            key={item.id}
            className="pp-row pp-gr"
            role="row"
            aria-level={item.depth + 1}
            aria-expanded={parent ? item.expanded : undefined}
            style={{ "--d": item.depth } as CSSProperties}
          >
            <span className="pp-gn" role="gridcell">
              {parent ? (
                <button
                  type="button"
                  className={`pp-fold${item.expanded ? " open" : ""}`}
                  onClick={() => onToggle(item.id)}
                  aria-label={item.expanded ? t("list.fold", { name: group.name }) : t("list.unfold", { name: group.name })}
                >
                  <ChevronRight aria-hidden="true" />
                </button>
              ) : null}
              <span className="pp-in">
                <span className="pp-nm">{group.name}</span>
                <span className="pp-grey">{line(item)}</span>
              </span>
            </span>
            <span className="pp-c pp-r pp-num" role="gridcell">{item.deviceCount}</span>
            <span className="pp-end" role="gridcell">
              {canWrite ? <Menu.Root>
                <Menu.Trigger className="pp-more" aria-label={t("list.actionsFor", { name: group.name })}>
                  <MoreHorizontal aria-hidden="true" />
                </Menu.Trigger>
                <Menu.Portal>
                  <Menu.Content className="lt-menu" align="end" sideOffset={4}>
                    <Menu.Item className="lt-menu-item" onSelect={() => onAction("edit", group)}>{t("menu.edit")}</Menu.Item>
                    <Menu.Item className="lt-menu-item" onSelect={() => onAction("child", group)}>{t("menu.child")}</Menu.Item>
                    <Menu.Separator className="lt-menu-sep" />
                    <Menu.Item className="lt-menu-item pp-danger" onSelect={() => onAction("delete", group)}>{t("menu.delete")}</Menu.Item>
                  </Menu.Content>
                </Menu.Portal>
              </Menu.Root> : null}
            </span>
          </div>
        );
      })}
      {ungrouped > 0 ? (
        <div className="pp-row pp-gr pp-loose" role="row">
          <span className="pp-gn" role="gridcell">
            <span className="pp-grey">{t("list.ungrouped")}</span>
          </span>
          <span className="pp-c pp-r pp-num" role="gridcell">{ungrouped}</span>
          <span role="gridcell" />
        </div>
      ) : null}
    </div>
  );
}
