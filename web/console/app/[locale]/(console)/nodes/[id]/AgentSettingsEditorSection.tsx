"use client";

import { useTranslations } from "next-intl";
import type { CSSProperties, ReactNode } from "react";
import { AgentSettingInputControl } from "./AgentSettingInputControl";
import type { AgentSettingEntry, AgentSettingsHistoryEvent } from "./agentSettingsModel";

type AgentSettingsEditorSectionProps = {
  serviceDiscoverySettings: AgentSettingEntry[];
  generalSettings: AgentSettingEntry[];
  draft: Record<string, string>;
  settingKeyFilesRootMode: string;
  onUpdateDraftValue: (key: string, value: string) => void;
  history: AgentSettingsHistoryEvent[];
  formatTime: (value?: string) => string;
  /** Save and Reset for every setting below, shown in the section's title row. */
  saveBar?: ReactNode;
};

const HISTORY_COLS = { "--cols": "minmax(0, 1fr) minmax(0, 160px) minmax(0, 160px)" } as CSSProperties;

function SettingRow({
  setting,
  value,
  filesRootMode,
  onChange,
}: {
  setting: AgentSettingEntry;
  value: string;
  filesRootMode: string;
  onChange: (key: string, value: string) => void;
}) {
  const t = useTranslations("devices.settingsTab.settings");
  const source = setting.source === "hub-override" ? "override" : setting.source === "hub-default" ? "hub" : "device";
  const values = setting.sensitive
    ? []
    : [
        [t("values.default"), setting.default_value],
        [t("values.global"), setting.global_value],
        [t("values.override"), setting.override_value],
        [t("values.reported"), setting.state_value],
      ].filter(([, v]) => v);
  return (
    <div className="dst-set">
      <div className="dst-set-txt">
        <span className="dv-nm">{setting.label}</span>
        {setting.description ? <p className="dv-hint">{setting.description}</p> : null}
        {setting.key === filesRootMode && value === "full" ? <p className="dst-warn dst-line">{t("filesFull")}</p> : null}
        <p className="dst-meta">
          <code className="dv-mono">{setting.key}</code>
          <span>{t(`source.${source}`)}</span>
          {setting.restart_required ? <span>{t("restarts")}</span> : null}
          {setting.drift ? <span className="dst-warn">{t("drift")}</span> : null}
          {setting.sensitive && setting.configured ? <span>{t("secretSaved")}</span> : null}
        </p>
        {values.length ? (
          <p className="dst-meta">
            {values.map(([label, v]) => <span key={label}>{label} <span className="dst-val">{v}</span></span>)}
          </p>
        ) : null}
      </div>
      <div className="dst-set-ctl">
        <AgentSettingInputControl
          setting={setting}
          currentValue={value}
          editable={setting.hub_managed && !setting.local_only}
          onChange={onChange}
        />
      </div>
    </div>
  );
}

/** The agent's settings (general, then service discovery) and the last applies the agent reported. */
export function AgentSettingsEditorSection({
  serviceDiscoverySettings,
  generalSettings,
  draft,
  settingKeyFilesRootMode,
  onUpdateDraftValue,
  history,
  formatTime,
  saveBar,
}: AgentSettingsEditorSectionProps) {
  const t = useTranslations("devices.settingsTab.settings");
  const row = (setting: AgentSettingEntry) => (
    <SettingRow
      key={setting.key}
      setting={setting}
      value={draft[setting.key] ?? setting.effective_value}
      filesRootMode={settingKeyFilesRootMode}
      onChange={onUpdateDraftValue}
    />
  );

  return (
    <>
      <section className="dv-sec" aria-labelledby="dst-set-t">
        <div className="dv-sh">
          <h2 id="dst-set-t">{t("title")}</h2>
          {saveBar ? <div className="dst-sh-acts">{saveBar}</div> : null}
        </div>
        {generalSettings.length > 0 ? (
          <div className="dv-sheet dst-sets">{generalSettings.map(row)}</div>
        ) : (
          <div className="dv-sheet"><p className="dv-hint">{t("none")}</p></div>
        )}
        {serviceDiscoverySettings.length > 0 ? (
          <>
            <h3 className="dst-sub" id="dst-disc-t">{t("discovery")}</h3>
            <div className="dv-sheet dst-sets" aria-labelledby="dst-disc-t">
              <p className="dv-hint">{t("discoveryHint")}</p>
              {serviceDiscoverySettings.map(row)}
            </div>
          </>
        ) : null}
      </section>

      <section className="dv-sec" aria-labelledby="dst-hist-t">
        <div className="dv-sh"><h2 id="dst-hist-t">{t("history")}</h2></div>
        <div className="dv-list" style={HISTORY_COLS}>
          {history.length === 0 ? (
            <div className="dv-empty"><b>{t("historyEmpty")}</b></div>
          ) : (
            <>
              <div className="dv-cols">
                <span>{t("historyCols.status")}</span>
                <span>{t("historyCols.changed")}</span>
                <span>{t("historyCols.applied")}</span>
              </div>
              {history.slice(0, 5).map((event, index) => (
                <div key={`${event.revision || "rev"}-${index}`} className={`dv-row${event.last_error ? " two" : ""}`}>
                  <div className="dst-hist-main">
                    <span className="dv-ln">
                      <span className={event.last_error ? "dst-bad" : "dv-nm"}>{event.status || t("historyUnknown")}</span>
                      {event.revision ? <span className="dv-grey mono">{event.revision}</span> : null}
                    </span>
                    {event.last_error ? <span className="dst-bad dst-small">{event.last_error}</span> : null}
                  </div>
                  <span className="dv-txt">{formatTime(event.updated_at)}</span>
                  <span className="dv-txt">{formatTime(event.applied_at)}</span>
                </div>
              ))}
            </>
          )}
        </div>
      </section>
    </>
  );
}
