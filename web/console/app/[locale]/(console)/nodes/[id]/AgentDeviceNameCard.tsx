"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";

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

/** The device's name as the agent keeps it: one field and Save. Heartbeats do not replace a saved name. */
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
  const t = useTranslations("devices.settingsTab.name");
  return (
    <section className="dv-sec" aria-labelledby="dst-name-t">
      <div className="dv-sh"><h2 id="dst-name-t">{t("title")}</h2></div>
      <div className="dv-sheet">
        <div className="dv-form-row dst-name-row">
          <div className="dv-field">
            <label htmlFor="dst-name">{t("label")}</label>
            <input
              id="dst-name"
              className="dv-input"
              value={deviceNameDraft}
              onChange={(event) => onDeviceNameDraftChange(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  onSaveDeviceName();
                }
              }}
              placeholder={t("placeholder")}
              disabled={savingDeviceName}
              aria-invalid={deviceNameError ? true : undefined}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <Button
            onClick={onSaveDeviceName}
            disabled={savingDeviceName || normalizedDeviceName === "" || !hasDeviceNameChange}
          >
            {savingDeviceName ? t("saving") : t("save")}
          </Button>
        </div>
        <p className="dv-hint dst-gap">{t("hint")}</p>
        {deviceNameError ? <p className="dv-err dst-gap" role="alert">{deviceNameError}</p> : null}
        {deviceNameMessage ? <p className="dv-ok-text dst-gap" role="status">{deviceNameMessage}</p> : null}
      </div>
    </section>
  );
}
