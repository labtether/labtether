"use client";

import type { AgentSettingEntry } from "./agentSettingsModel";

type AgentSettingInputControlSetting = Pick<
  AgentSettingEntry,
  "key" | "label" | "type" | "min_int" | "max_int" | "allowed_values" | "sensitive" | "configured"
>;

type AgentSettingInputControlProps = {
  setting: AgentSettingInputControlSetting;
  currentValue: string;
  editable: boolean;
  onChange: (key: string, value: string) => void;
};

/** One agent setting's control, named by the setting's own label. Read-only settings show their value, secrets masked. */
export function AgentSettingInputControl({
  setting,
  currentValue,
  editable,
  onChange,
}: AgentSettingInputControlProps) {
  if (!editable) {
    return (
      <input
        className="dv-input"
        aria-label={setting.label}
        type={setting.sensitive ? "password" : "text"}
        value={currentValue}
        readOnly
      />
    );
  }

  if (setting.type === "bool" || setting.type === "enum") {
    const options = setting.type === "bool" ? ["true", "false"] : (setting.allowed_values ?? []);
    return (
      <select
        className="dv-input"
        aria-label={setting.label}
        value={currentValue}
        onChange={(event) => onChange(setting.key, event.target.value)}
      >
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    );
  }

  if (setting.type === "int") {
    return (
      <input
        className="dv-input"
        aria-label={setting.label}
        type="number"
        min={setting.min_int}
        max={setting.max_int}
        value={currentValue}
        onChange={(event) => onChange(setting.key, event.target.value)}
      />
    );
  }

  return (
    <input
      className={`dv-input${setting.sensitive ? " dv-mono" : ""}`}
      aria-label={setting.label}
      type={setting.sensitive ? "password" : "text"}
      autoComplete={setting.sensitive ? "new-password" : undefined}
      placeholder={setting.sensitive && setting.configured ? "••••••••" : undefined}
      value={currentValue}
      onChange={(event) => onChange(setting.key, event.target.value)}
      spellCheck={false}
    />
  );
}
