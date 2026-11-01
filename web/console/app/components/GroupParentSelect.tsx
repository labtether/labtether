"use client";

import { useId, useMemo } from "react";
import { useTranslations } from "next-intl";
import type { Group } from "../console/models";
import { Select } from "./ui/Input";

type GroupParentSelectProps = {
  groups: Group[];
  value: string; // current parent_group_id or ""
  onChange: (id: string) => void;
  excludeGroupId?: string; // exclude self + descendants
  disabled?: boolean;
  label?: string;
  /** The first option, for "no group"; defaults to "None". */
  noneLabel?: string;
  id?: string;
};

type FlatOption = {
  id: string;
  label: string;
};

/** Collect the full set of descendant IDs for a given root ID. */
function collectDescendantIds(rootId: string, groups: Group[]): Set<string> {
  const result = new Set<string>();
  const queue = [rootId];
  while (queue.length > 0) {
    const current = queue.pop()!;
    for (const g of groups) {
      if (g.parent_group_id === current && !result.has(g.id)) {
        result.add(g.id);
        queue.push(g.id);
      }
    }
  }
  return result;
}

/** Tree order (sort_order, then name), each option labelled with its path from the top: "Home / Rack A". */
function buildFlatOptions(groups: Group[], excludeGroupId: string | undefined): FlatOption[] {
  const excluded = new Set<string>();
  if (excludeGroupId) {
    excluded.add(excludeGroupId);
    collectDescendantIds(excludeGroupId, groups).forEach((id) => excluded.add(id));
  }

  const visible = groups.filter((g) => !excluded.has(g.id));
  const visibleIds = new Set(visible.map((g) => g.id));
  const sorted = [...visible].sort((a, b) => (a.sort_order !== b.sort_order ? a.sort_order - b.sort_order : a.name.localeCompare(b.name)));

  const options: FlatOption[] = [];
  const seen = new Set<string>();
  function walk(parentId: string | undefined, path: string) {
    const children = sorted.filter((g) =>
      parentId === undefined ? !g.parent_group_id || !visibleIds.has(g.parent_group_id) : g.parent_group_id === parentId,
    );
    for (const g of children) {
      if (seen.has(g.id)) continue;
      seen.add(g.id);
      const label = path ? `${path} / ${g.name}` : g.name;
      options.push({ id: g.id, label });
      walk(g.id, label);
    }
  }
  walk(undefined, "");
  return options;
}

/** A group picker in tree order. Used for a group's parent and for the group a device is in. */
export function GroupParentSelect({
  groups,
  value,
  onChange,
  excludeGroupId,
  disabled = false,
  label,
  noneLabel,
  id,
}: GroupParentSelectProps) {
  const t = useTranslations("groups");
  const autoId = useId();
  const selectId = id ?? autoId;
  const options = useMemo(() => buildFlatOptions(groups, excludeGroupId), [groups, excludeGroupId]);

  return (
    <div className="pp-field">
      {label ? <label htmlFor={selectId}>{label}</label> : null}
      <Select
        id={selectId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        className="w-full"
        aria-label={label ? undefined : t("form.inside")}
      >
        <option value="">{noneLabel ?? t("form.none")}</option>
        {options.map((opt) => (
          <option key={opt.id} value={opt.id}>
            {opt.label}
          </option>
        ))}
      </Select>
    </div>
  );
}
