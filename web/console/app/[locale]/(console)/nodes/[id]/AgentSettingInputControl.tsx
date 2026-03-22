"use client";

import { Input, Select } from "../../../../components/ui/Input";
import type { AgentSettingEntry } from "./agentSettingsModel";

type AgentSettingInputControlSetting = Pick<
  AgentSettingEntry,
  "key" | "type" | "min_int" | "max_int" | "allowed_values"
>;

type AgentSettingInputControlProps = {
  setting: AgentSettingInputControlSetting;
  currentValue: string;
  editable: boolean;
  onChange: (key: string, value: string) => void;
};

export function AgentSettingInputControl({
  setting,
  currentValue,
  editable,
  onChange,
}: AgentSettingInputControlProps) {
  if (!editable) {
    return <Input value={currentValue} readOnly />;
  }

  if (setting.type === "bool") {
    return (
      <Select
        value={currentValue}
        onChange={(event) => onChange(setting.key, event.target.value)}
        className="w-full"
      >
        <option value="true">true</option>
        <option value="false">false</option>
      </Select>
    );
  }

  if (setting.type === "enum") {
    return (
      <Select
        value={currentValue}
        onChange={(event) => onChange(setting.key, event.target.value)}
        className="w-full"
      >
        {(setting.allowed_values ?? []).map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </Select>
    );
  }

  if (setting.type === "int") {
    return (
      <Input
        type="number"
        min={setting.min_int}
        max={setting.max_int}
        value={currentValue}
        onChange={(event) => onChange(setting.key, event.target.value)}
      />
    );
  }

  return (
    <Input
      value={currentValue}
      onChange={(event) => onChange(setting.key, event.target.value)}
    />
  );
}
