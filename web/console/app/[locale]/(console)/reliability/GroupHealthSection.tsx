"use client";

import { useTranslations } from "next-intl";
import type { Group, GroupReliability } from "../../../console/models";
import { groupPath } from "../groups/groupListModel";
import { Section, Status } from "./HealthParts";
import { groupTone } from "./healthModel";

type GroupHealthSectionProps = {
  rows: GroupReliability[];
  groups: Group[];
  selected: string;
  onPick: (groupId: string) => void;
};

/** Where a group is: its location, else its coordinates. */
function place(group: Group): string {
  if (group.location?.trim()) return group.location.trim();
  if (typeof group.latitude === "number" && typeof group.longitude === "number") {
    return `${group.latitude.toFixed(3)}, ${group.longitude.toFixed(3)}`;
  }
  return "";
}

/**
 * Groups: each group's health score and grade from the hub, its devices online, and where it is. Picking a group shows
 * its events below.
 */
export function GroupHealthSection({ rows, groups, selected, onPick }: GroupHealthSectionProps) {
  const t = useTranslations("reliability.groups");
  const sorted = [...rows].sort((a, b) => a.score - b.score || a.group.name.localeCompare(b.group.name));

  return (
    <Section id="pp-groups" title={t("title")}>
      <div className="pp-list" role="table" aria-labelledby="pp-groups">
        {sorted.map((row) => {
          const { tone, word } = groupTone(row);
          const facts = [
            t("online", { online: row.assets_online, total: row.assets_total }),
            row.assets_offline > 0 ? t("offline", { count: row.assets_offline }) : "",
            row.assets_stale > 0 ? t("stale", { count: row.assets_stale }) : "",
            place(row.group),
          ].filter(Boolean).join(" · ");
          const on = row.group.id === selected;
          return (
            <div key={row.group.id} className={`pp-row pp-gh${on ? " sel" : ""}`} role="row" onClick={() => onPick(row.group.id)}>
              <span className="pp-in" role="cell">
                <button type="button" className="pp-nm pp-pick" aria-pressed={on} onClick={(e) => { e.stopPropagation(); onPick(row.group.id); }}>
                  {groupPath(groups, row.group.id) || row.group.name}
                </button>
                <span className="pp-grey">{facts}</span>
              </span>
              <span role="cell"><Status tone={tone}>{t(`words.${word}`)}</Status></span>
              <span className="pp-c pp-r pp-num" role="cell">{t("score", { score: row.score, grade: row.grade })}</span>
            </div>
          );
        })}
        {rows.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{t("empty")}</p></div> : null}
      </div>
    </Section>
  );
}
