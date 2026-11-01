"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import type { Group } from "../../../../console/models";

type GroupPathBreadcrumbProps = {
  groupId: string | undefined;
  groups: Group[];
};

/** Walks the parent_group_id chain and returns groups root-first. */
function buildGroupPath(groups: Group[], leafId: string): Group[] {
  const byId = new Map(groups.map((g) => [g.id, g]));
  const path: Group[] = [];
  let current = byId.get(leafId);
  let guard = 0;
  while (current && guard < 20) {
    path.unshift(current);
    current = current.parent_group_id ? byId.get(current.parent_group_id) : undefined;
    guard++;
  }
  return path;
}

/** A Details row: the device's group as its path ("Lab / Rack 2"), linked to Groups. Nothing without a group. */
export function GroupPathBreadcrumb({ groupId, groups }: GroupPathBreadcrumbProps) {
  const t = useTranslations("devices.page.details");
  const path = useMemo(() => (groupId ? buildGroupPath(groups, groupId) : []), [groupId, groups]);
  if (path.length === 0) return null;

  return (
    <div>
      <dt>{t("group")}</dt>
      <dd>
        <Link className="dv-link" href="/groups">{path.map((g) => g.name).join(" / ")}</Link>
      </dd>
    </div>
  );
}
