"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check } from "lucide-react";
import { useTranslations } from "next-intl";
import type { LogLevel, LogSource, TelemetryWindow } from "../../../console/models";
import { friendlySourceLabel } from "../../../console/taxonomy";
import { FilterField, PickMenu, type PickOption } from "./LogControls";

const LEVELS: LogLevel[] = ["all", "error", "warn", "info", "debug"];

/** Filter field, then Device, Level, Range and Source menus; Live on the right. */
export function LogsToolbar({
  query,
  onQuery,
  devices,
  device,
  onDevice,
  level,
  onLevel,
  windows,
  window: range,
  onWindow,
  sources,
  source,
  onSource,
  heartbeats,
  onHeartbeats,
  live,
  onLive,
}: {
  query: string;
  onQuery: (value: string) => void;
  devices: PickOption[];
  device: string;
  onDevice: (value: string) => void;
  level: LogLevel;
  onLevel: (value: LogLevel) => void;
  windows: readonly TelemetryWindow[];
  window: TelemetryWindow;
  onWindow: (value: TelemetryWindow) => void;
  sources: LogSource[];
  source: string;
  onSource: (value: string) => void;
  heartbeats: boolean;
  onHeartbeats: (value: boolean) => void;
  live: boolean;
  onLive: (value: boolean) => void;
}) {
  const t = useTranslations("logs");
  const deviceOptions = [{ value: "all", label: t("device.all") }, ...devices];
  const sourceOptions = [
    { value: "all", label: t("source.all") },
    ...sources.map((s) => ({ value: s.source, label: friendlySourceLabel(s.source) })),
  ];
  return (
    <div className="lg-tools">
      <FilterField value={query} onChange={onQuery} placeholder={t("filter")} />
      <div className="lg-ctl">
        <PickMenu label={t("cols.device")} value={device} options={deviceOptions} onChange={onDevice} />
        <PickMenu
          label={t("cols.level")}
          value={level}
          options={LEVELS.map((l) => ({ value: l, label: t(`level.${l}`) }))}
          onChange={(v) => onLevel(v as LogLevel)}
        />
        <PickMenu
          label={t("range.label")}
          value={range}
          options={[...windows].map((w) => ({ value: w, label: t.has(`range.${w}`) ? t(`range.${w}`) : w }))}
          onChange={(v) => onWindow(v as TelemetryWindow)}
          current={t.has(`range.${range}`) ? t(`range.${range}`) : range}
        />
        <PickMenu
          label={t("cols.source")}
          value={source}
          options={sourceOptions}
          onChange={onSource}
          current={source === "all" && heartbeats ? t("source.allWithHeartbeats") : undefined}
        >
          <div className="lt-menu-sep" />
          <Menu.CheckboxItem className="lt-menu-item" checked={heartbeats} onCheckedChange={(v) => onHeartbeats(v === true)}>
            <span className="lg-menu-t">{t("source.heartbeats")}</span>
            <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
          </Menu.CheckboxItem>
        </PickMenu>
      </div>
      <span className="lt-sp" />
      <button type="button" className="lg-live" role="switch" aria-checked={live} onClick={() => onLive(!live)}>
        <span className={`lg-switch${live ? " on" : ""}`} aria-hidden="true"><i /></span>
        {live ? <i className="lg-on" aria-hidden="true" /> : null}
        {t("live")}
      </button>
    </div>
  );
}
