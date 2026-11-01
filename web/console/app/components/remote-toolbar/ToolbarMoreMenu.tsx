"use client";

import { Ellipsis } from "lucide-react";
import { useTranslations } from "next-intl";
import type { RemoteViewToolbarProps, ScalingMode } from "./types";
import { ToolCheck, ToolChoice, ToolItem, ToolLabel, ToolMenu, ToolSep } from "./ToolbarButtons";

type MoreMenuProps = RemoteViewToolbarProps & {
  onOpenChange: (open: boolean) => void;
  overlay: boolean;
  autoHide: boolean;
  onAutoHideToggle: () => void;
  position: "top" | "bottom";
  onPositionToggle: () => void;
};

const QUALITIES = ["low", "medium", "high"] as const;
const SCALINGS: ScalingMode[] = ["fit", "native", "fill"];

/**
 * More: the screen's settings. Display (when there are several), scaling, quality, sound, the performance numbers,
 * recording and view only; in full screen also where the toolbar sits and whether it hides.
 */
export function ToolbarMoreMenu(props: MoreMenuProps) {
  const t = useTranslations("desktop.tools");
  const displays = props.displays && props.displays.length > 1 && props.onDisplayChange ? props.displays : null;
  const sound = props.onAudioToggle || props.audioUnavailable;
  return (
    <ToolMenu icon={<Ellipsis aria-hidden="true" />} label={t("more")} onOpenChange={props.onOpenChange}>
      {displays ? (
        <>
          <ToolChoice
            label={t("display")}
            value={props.selectedDisplay || displays.find((d) => d.primary)?.name || displays[0].name}
            options={displays.map((d) => ({
              value: d.name,
              label: t(d.primary ? "displayPrimary" : "displayItem", { name: d.name, width: d.width, height: d.height }),
            }))}
            onChange={(name) => props.onDisplayChange?.(name)}
          />
          <ToolSep />
        </>
      ) : null}
      <ToolChoice
        label={t("scaling")}
        value={props.scalingMode}
        options={SCALINGS.map((mode) => ({ value: mode, label: t(`scale.${mode}`) }))}
        onChange={props.onScalingModeChange}
      />
      <ToolSep />
      <ToolChoice
        label={t("quality")}
        value={props.quality}
        options={QUALITIES.map((q) => ({ value: q, label: t(`q.${q}`) }))}
        onChange={props.onQualityChange}
      />
      {props.onAdaptiveQualityToggle ? (
        <ToolCheck checked={!!props.adaptiveQualityEnabled} onChange={props.onAdaptiveQualityToggle}>
          {t("autoQuality")}
        </ToolCheck>
      ) : null}
      {props.qualitySuggestion && props.onQualitySuggestionApply ? (
        <ToolItem onSelect={props.onQualitySuggestionApply}>
          {t(props.qualitySuggestion === "upgrade" ? "raiseQuality" : "lowerQuality")}
        </ToolItem>
      ) : null}
      <ToolSep />
      {sound ? (
        <ToolCheck
          checked={!props.audioMuted && !props.audioUnavailable}
          onChange={props.onAudioToggle ?? (() => {})}
          disabled={props.audioUnavailable}
        >
          {t(props.audioUnavailable ? "soundUnavailable" : "sound")}
        </ToolCheck>
      ) : null}
      {sound && props.onVolumeChange && !props.audioUnavailable ? (
        <div className="rv-vol">
          <input
            type="range"
            aria-label={t("volume")}
            min={0}
            max={1}
            step={0.05}
            value={Math.max(0, Math.min(1, props.volume ?? 1))}
            onChange={(event) => props.onVolumeChange?.(Number(event.target.value))}
          />
        </div>
      ) : null}
      {props.onPerformanceOverlayToggle ? (
        <ToolCheck checked={!!props.showPerformanceOverlay} onChange={props.onPerformanceOverlayToggle} hint="Ctrl+F1">
          {t("performance")}
        </ToolCheck>
      ) : null}
      {props.onToggleRecording ? (
        <ToolItem onSelect={props.onToggleRecording}>{t(props.recording ? "stopRecording" : "record")}</ToolItem>
      ) : null}
      <ToolCheck checked={props.viewOnly} onChange={props.onViewOnlyToggle}>
        {t("viewOnly")}
      </ToolCheck>
      {props.overlay ? (
        <>
          <ToolSep />
          <ToolLabel>{t("toolbar")}</ToolLabel>
          <ToolCheck checked={props.autoHide} onChange={props.onAutoHideToggle}>
            {t("autoHide")}
          </ToolCheck>
          <ToolItem onSelect={props.onPositionToggle}>{t(props.position === "top" ? "moveBottom" : "moveTop")}</ToolItem>
        </>
      ) : null}
    </ToolMenu>
  );
}
