"use client";

// Traffic control: each rule with the networks it covers and its in and out limits. A rule's ⋯ deletes it after a
// confirm.

import { Button } from "../../../../../components/ui/Button";
import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection, DiTime } from "../proxmox/DiList";
import { normalizeTrafficRules, type PBSTrafficRule } from "./pbsJobsModel";
import { pbsAction, pbsErrorDetail, usePBSActions, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY: PBSTrafficRule[] = [];
const COLS = "minmax(0, 1fr) 110px 110px 28px";
const COLS_S = "minmax(0, 1fr) 28px";

export function PBSTrafficView({ assetId }: { assetId: string }) {
  const { t, tk } = usePBSWords();
  const base = `/api/pbs/assets/${encodeURIComponent(assetId)}/traffic-control`;
  const rules = usePBSResource(base, normalizeTrafficRules, EMPTY);
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();

  const remove = (rule: PBSTrafficRule) => run(
    `delete-${rule.name}`,
    () => pbsAction(`${base}/${encodeURIComponent(rule.name)}`, "DELETE"),
    t("traffic.deleted", { name: rule.name }),
    t("traffic.deleteFailed", { name: rule.name }),
    rules.refresh,
  );

  return (
    <DiSection
      id="pbs-traffic"
      title={t("view.traffic")}
      right={<Button size="sm" variant="ghost" onClick={rules.refresh} disabled={rules.loading}>{tk("refresh")}</Button>}
    >
      {rules.error ? <DiNote tone="bad">{[t("traffic.loadFailed"), pbsErrorDetail(rules.error)].filter(Boolean).join(" ")}</DiNote> : null}
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={COLS} colsS={COLS_S} label={t("view.traffic")}>
        <DiCols>
          <span>{t("cols.rule")}</span>
          <span className="r di-s">{t("cols.in")}</span>
          <span className="r di-s">{t("cols.out")}</span>
          <span />
        </DiCols>
        {rules.data.length === 0 ? <DiEmpty title={rules.loaded ? t("traffic.none") : tk("loading")} /> : null}
        {rules.data.map((rule) => (
          <DiRow key={rule.name}>
            <DiMain name={rule.name} grey={<DiDots parts={[rule.network?.join(", "), rule.comment]} />} />
            <DiTime className="di-s">{rule.rate_in}</DiTime>
            <DiTime className="di-s">{rule.rate_out}</DiTime>
            <DiRowMenu
              label={tk("actionsFor", { name: rule.name })}
              items={[{
                label: t("traffic.delete"),
                danger: true,
                disabled: busy !== null,
                onSelect: () => ask({
                  title: t("traffic.askTitle", { name: rule.name }),
                  message: t("traffic.askBody"),
                  confirmLabel: t("traffic.askConfirm"),
                  onConfirm: () => remove(rule),
                }),
              }]}
            />
          </DiRow>
        ))}
      </DiList>
      {dialog}
    </DiSection>
  );
}
