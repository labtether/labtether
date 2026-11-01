"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import type { Group, MaintenanceWindow } from "../../../console/models";
import { groupPath } from "../groups/groupListModel";
import { Section } from "./HealthParts";
import { sortWindows, windowEffects, windowState } from "./healthModel";
import { span, when } from "./healthTime";

type DowntimeSectionProps = {
  windows: MaintenanceWindow[];
  groups: Group[];
  loading: boolean;
  partial: boolean;
  onEnd: (window: MaintenanceWindow) => void;
};

/**
 * Downtime: every group's windows, running ones first. A window mutes alerts, pauses actions and pauses updates for
 * its group's devices, as set when it was scheduled. ⋯ ends a running window now or deletes another.
 */
export function DowntimeSection({ windows, groups, loading, partial, onEnd }: DowntimeSectionProps) {
  const t = useTranslations("reliability.downtime");
  const locale = useLocale();
  const now = Date.now();
  const rows = sortWindows(windows, now);

  return (
    <Section id="pp-downtime" title={t("title")}>
      {partial ? <p className="pp-err pp-load" role="status">{t("partial")}</p> : null}
      <div className="pp-list pp-hang" role="table" aria-labelledby="pp-downtime">
        {rows.map((w) => {
          const state = windowState(w, now);
          const effects = windowEffects(w).map((k) => t(`effects.${k}`)).join(", ");
          const group = groupPath(groups, w.group_id) || w.group_id;
          const status = state === "active"
            ? t("state.activeUntil", { time: when(w.end_at, now, locale) })
            : state === "ended" ? t("state.ended", { time: when(w.end_at, now, locale) }) : span(w.start_at, w.end_at, now, locale);
          return (
            <div key={`${w.group_id}:${w.id}`} className={`pp-row pp-dt${state === "ended" ? " pp-dim" : ""}`} role="row">
              <span className="pp-lead" aria-hidden="true">{state === "active" ? <i className="pp-dot busy" /> : null}</span>
              <span className="pp-in" role="cell">
                <span className="pp-nm">{group}</span>
                <span className="pp-grey">{[w.name, effects ? capitalize(effects, locale) : t("effects.none")].filter(Boolean).join(" · ")}</span>
              </span>
              <span className="pp-r" role="cell">
                <span className="pp-when pp-num">{status}</span>
              </span>
              <span className="pp-end" role="cell">
                <Menu.Root>
                  <Menu.Trigger className="pp-more" aria-label={t("actionsFor", { name: group })}>
                    <MoreHorizontal aria-hidden="true" />
                  </Menu.Trigger>
                  <Menu.Portal>
                    <Menu.Content className="lt-menu" align="end" sideOffset={4}>
                      <Menu.Item className="lt-menu-item pp-danger" onSelect={() => onEnd(w)}>
                        {state === "active" ? t("endNow") : t("delete")}
                      </Menu.Item>
                    </Menu.Content>
                  </Menu.Portal>
                </Menu.Root>
              </span>
            </div>
          );
        })}
        {rows.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{loading ? t("loading") : t("empty")}</p></div> : null}
      </div>
    </Section>
  );
}

const capitalize = (text: string, locale: string) => text.charAt(0).toLocaleUpperCase(locale) + text.slice(1);
