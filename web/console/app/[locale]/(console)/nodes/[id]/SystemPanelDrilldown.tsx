"use client";

import type { ReactNode } from "react";
import { ArrowLeft } from "lucide-react";
import type { TelemetryWindow } from "../../../../console/models";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import { DataValue } from "../../../../components/ui/DataValue";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import type { AnalyzedSeries } from "./nodeMetricsModel";
import {
  DRILLDOWN_ORDER,
  drilldownLabel,
  toneColor,
  toneStyle,
  type DrilldownContent,
} from "./systemPanelModel";
import { SystemPanelHistoricalContext } from "./SystemPanelHistoricalContext";
import { SystemPanelNetworkInterfacePreview } from "./SystemPanelNetworkInterfacePreview";
import type { SystemDrilldownView } from "./systemPanelTypes";

export function SummaryMetricCard({
  icon,
  title,
  value,
  hint,
  onClick,
}: {
  icon: ReactNode;
  title: string;
  value: string;
  hint: string;
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3 text-left transition-colors hover:border-[var(--accent)]/50 hover:bg-[var(--surface)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
    >
      <div className="flex items-center gap-2 text-[var(--text)]">
        {icon}
        <span className="text-sm font-medium">{title}</span>
      </div>
      <p className="mt-2 text-lg font-semibold tabular-nums text-[var(--text)]">{value}</p>
      <p className="mt-1 text-xs text-[var(--muted)]">{hint}</p>
      <p className="mt-2 text-xs text-[var(--accent)]">Open deep detail</p>
    </button>
  );
}

function DrilldownNav({
  activeView,
  onOpenDrilldown,
}: {
  activeView: SystemDrilldownView;
  onOpenDrilldown?: (view: SystemDrilldownView) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {DRILLDOWN_ORDER.map((view) => {
        const isActive = view === activeView;
        return (
          <Button
            key={view}
            size="sm"
            variant={isActive ? "primary" : "ghost"}
            onClick={() => onOpenDrilldown?.(view)}
          >
            {drilldownLabel(view)}
          </Button>
        );
      })}
    </div>
  );
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
  return (
    <div className="space-y-4">
      <Card
        highlight={content.statusTone === "bad"}
        className="overflow-hidden"
        style={{
          backgroundImage: "radial-gradient(circle at top right, var(--accent-subtle), transparent 42%)",
        }}
      >
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-base font-semibold text-[var(--text)]">{content.title}</h3>
              <span
                className="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium"
                style={toneStyle(content.statusTone)}
              >
                {content.statusLabel}
              </span>
            </div>
            <p className="max-w-2xl text-xs text-[var(--muted)]">{content.subtitle}</p>
          </div>
          <Button size="sm" variant="ghost" onClick={onCloseDrilldown}>
            <ArrowLeft size={14} />
            Back to Overview
          </Button>
        </div>

        <div className="mt-4 grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1.85fr)]">
          <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-[var(--muted)]">
              {content.heroLabel}
            </p>
            <div className="mt-2 text-3xl font-semibold text-[var(--text)]">
              <DataValue value={content.heroValue} className="text-3xl" mono={false} />
            </div>
            <p className="mt-2 text-sm text-[var(--muted)]">{content.heroHint}</p>
          </div>

          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            {content.summaryStats.map((stat) => (
              <div
                key={stat.key}
                className="rounded-lg border bg-[var(--surface)] p-3"
                style={{
                  borderColor: stat.tone === "neutral" ? "var(--line)" : toneColor(stat.tone),
                }}
              >
                <p className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--muted)]">
                  {stat.label}
                </p>
                <p className="mt-2 text-lg font-semibold tabular-nums text-[var(--text)]">{stat.value}</p>
                <p
                  className="mt-1 text-xs"
                  style={{ color: stat.tone === "neutral" ? "var(--muted)" : toneColor(stat.tone) }}
                >
                  {stat.hint}
                </p>
              </div>
            ))}
          </div>
        </div>

        <div className="mt-4">
          <p className="mb-2 text-xs font-medium uppercase tracking-[0.18em] text-[var(--muted)]">
            Switch Detail
          </p>
          <DrilldownNav activeView={drilldown} onOpenDrilldown={onOpenDrilldown} />
        </div>
      </Card>

      <SystemPanelHistoricalContext
        series={historySeries}
        telemetryLoading={telemetryLoading}
        telemetryWindow={telemetryWindow}
      />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(300px,1fr)]">
        <div className="space-y-4">
          {content.sections.map((section) => (
            <Card key={section.key}>
              <div className="space-y-1">
                <h4 className="text-sm font-medium text-[var(--text)]">{section.title}</h4>
                <p className="text-xs text-[var(--muted)]">{section.description}</p>
              </div>
              <dl className="mt-4 grid grid-cols-1 gap-x-6 gap-y-2 md:grid-cols-2">
                {section.rows.map((row) => (
                  <div key={row.key} className="flex items-baseline justify-between gap-2 border-b border-[var(--line)]/40 pb-1">
                    <dt className="text-xs text-[var(--muted)]">{row.label}</dt>
                    <dd className="text-right text-xs font-medium text-[var(--text)]">{row.value}</dd>
                  </div>
                ))}
              </dl>
            </Card>
          ))}
        </div>

        <div className="space-y-4">
          <Card>
            <h4 className="text-sm font-medium text-[var(--text)]">Related Diagnostics</h4>
            <p className="mt-1 text-xs text-[var(--muted)]">
              Use these pivots when the detail view tells you where to investigate next.
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              {content.actions.map((action) => (
                <Button
                  key={action.key}
                  size="sm"
                  variant={action.variant ?? "ghost"}
                  onClick={() => onOpenPanel?.(action.panel)}
                >
                  {action.label}
                </Button>
              ))}
            </div>
          </Card>

          <Card>
            <h4 className="text-sm font-medium text-[var(--text)]">Investigation Flow</h4>
            <div className="mt-3 space-y-2">
              {content.tips.map((tip) => (
                <p key={tip} className="text-xs leading-5 text-[var(--muted)]">
                  {tip}
                </p>
              ))}
            </div>
          </Card>

          {drilldown === "network" ? (
            <SystemPanelNetworkInterfacePreview
              interfaces={interfaces}
              loading={interfacesLoading}
              error={interfacesError}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}
