"use client";

import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";

type AgentDeviceNameCardProps = {
  deviceNameDraft: string;
  onDeviceNameDraftChange: (value: string) => void;
  onSaveDeviceName: () => void;
  savingDeviceName: boolean;
  normalizedDeviceName: string;
  hasDeviceNameChange: boolean;
  deviceNameError: string | null;
  deviceNameMessage: string | null;
};

export function AgentDeviceNameCard({
  deviceNameDraft,
  onDeviceNameDraftChange,
  onSaveDeviceName,
  savingDeviceName,
  normalizedDeviceName,
  hasDeviceNameChange,
  deviceNameError,
  deviceNameMessage,
}: AgentDeviceNameCardProps) {
  return (
    <div className="mb-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-3">
      <p className="text-sm font-medium text-[var(--text)]">Device Name</p>
      <p className="mt-1 text-xs text-[var(--muted)]">
        Rename this device for all views. This name is persisted and will not be overwritten by future heartbeats.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Input
          className="min-w-[220px] flex-1"
          value={deviceNameDraft}
          onChange={(event) => onDeviceNameDraftChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onSaveDeviceName();
            }
          }}
          placeholder="Device name"
          disabled={savingDeviceName}
        />
        <Button
          variant="primary"
          onClick={onSaveDeviceName}
          disabled={savingDeviceName || normalizedDeviceName === "" || !hasDeviceNameChange}
        >
          {savingDeviceName ? "Saving..." : "Save Name"}
        </Button>
      </div>
      {deviceNameError ? <p className="mt-2 text-xs text-[var(--bad)]">{deviceNameError}</p> : null}
      {deviceNameMessage ? <p className="mt-2 text-xs text-[var(--ok)]">{deviceNameMessage}</p> : null}
    </div>
  );
}
