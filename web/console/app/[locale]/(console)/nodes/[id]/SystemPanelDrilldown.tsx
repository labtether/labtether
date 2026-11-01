"use client";

import type { ReactNode } from "react";
import { ArrowLeft } from "lucide-react";
import { useTranslations } from "next-intl";
import type { TelemetryWindow } from "../../../../console/models";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import { Button } from "../../../../components/ui/Button";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import type { DetailRow } from "./systemPanelDrilldownModel";
import {
  DRILLDOWN_ORDER,
  toneTextClass,
  type DrilldownContent,
} from "./systemPanelModel";
import { SystemPanelHistoricalContext } from "./SystemPanelHistoricalContext";
import { SystemPanelNetworkInterfacePreview } from "./SystemPanelNetworkInterfacePreview";
import type { SystemDrilldownView } from "./systemPanelTypes";

/**
 * One cell of the Hardware view's usage sheet: the label, the value now and a grey hint. The whole cell is a button
 * that opens that drill-down. icon is accepted for older callers and not drawn.
 */
export function SummaryMetricCard({
  title,
  value,
  hint,
  hot = false,
  onClick,
}: {
  icon?: ReactNode;
  title: string;
  value: string;
  hint: string;
  hot?: boolean;
  onClick?: () => void;
}) {
  return (
    <button type="button" onClick={onClick} className="dmt-cell dmt-open">
      <span className="dmt-k">{title}</span>
      <b className={`dmt-readout${hot ? " hot" : ""}`}>{value}</b>
      <span className="dmt-hint">{hint}</span>
    </button>
  );
}

function RowValue({ row }: { row: DetailRow }) {
  const t = useTranslations("devices.hardware");
  const text = typeof row.value === "string" ? row.value : t(`values.${row.value.phrase}`, row.value.values ?? {});
  const cls = toneTextClass(row.tone);
  return <span className={cls || undefined}>{text}</span>;
}

export function SystemPanelDrilldown({
  content,
  drilldown,
  historySeries,
  telemetryLoading,
  telemetryWindow,
  onOpenDrilldown,
  onCloseDrilldown,
  onOpenPanel,
  interfaces,
  interfacesLoading,
  interfacesError,
}: {
  content: DrilldownContent;
  drilldown: SystemDrilldownView;
  historySeries: AnalyzedSeries[];
  telemetryLoading: boolean;
  telemetryWindow?: TelemetryWindow;
  onOpenDrilldown?: (view: SystemDrilldownView) => void;
  onCloseDrilldown?: () => void;
  onOpenPanel?: (panel: string) => void;
  interfaces: NetworkInterfaceInfo[];
  interfacesLoading: boolean;
  interfacesError: string | null;
}) {
  const t = useTranslations("devices.hardware");
  const quiet = [content.heroValue, content.heroHint].filter(Boolean).join(" · ");
  const word = content.statusWord ? t(`status.${drilldown}.${content.statusWord}`) : "";

  return (
    <div className="dmt">
      <div className="dv-bar">
        <Button size="sm" variant="ghost" onClick={onCloseDrilldown}>
          <ArrowLeft size={14} aria-hidden="true" />
          {t("back")}
        </Button>
        <div className="dv-seg" role="group" aria-label={t("detailSwitch")}>
          {DRILLDOWN_ORDER.map((view) => (
            <button
              key={view}
              type="button"
              aria-pressed={view === drilldown}
              onClick={() => onOpenDrilldown?.(view)}
            >
              {t(`views.${view}`)}
            </button>
          ))}
        </div>
        <span className="dv-sp" />
        {content.actions.map((action) => (
          <Button key={action.key} size="sm" variant="secondary" onClick={() => onOpenPanel?.(action.panel)}>
            {t(`actions.${action.key}`)}
          </Button>
        ))}
      </div>

      <section className="dv-sec" aria-labelledby="dmt-dd">
        <div className="dv-sh">
          <h2 id="dmt-dd">
            {t(`views.${drilldown}`)}
            {quiet ? <span className="dv-quiet">{quiet}</span> : null}
            {word ? <span className={`dmt-status ${content.statusTone}`}>{word}</span> : null}
          </h2>
          {telemetryWindow ? <span className="dv-hint">{t("window", { window: telemetryWindow })}</span> : null}
        </div>
        <SystemPanelHistoricalContext
          series={historySeries}
          telemetryLoading={telemetryLoading}
          telemetryWindow={telemetryWindow}
        />
      </section>

      <div className="dmt-pair">
        {content.sections.map((section) => (
          <section key={section.key} className="dv-sec" aria-labelledby={`dmt-${section.key}`}>
            <div className="dv-sh"><h2 id={`dmt-${section.key}`}>{t(`sections.${section.key}`)}</h2></div>
            <dl className="dv-sheet tight dv-kv">
              {section.rows.map((row) => (
                <div key={row.key}>
                  <dt>{t(`rows.${row.key}`)}</dt>
                  <dd><RowValue row={row} /></dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>

      {drilldown === "network" ? (
        <SystemPanelNetworkInterfacePreview
          interfaces={interfaces}
          loading={interfacesLoading}
          error={interfacesError}
        />
      ) : null}
    </div>
  );
}
