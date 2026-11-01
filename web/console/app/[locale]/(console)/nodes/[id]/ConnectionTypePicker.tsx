"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";

type ConnectionTypePickerProps = {
  onSelectProtocol: () => void;
  onSelectWebService: () => void;
  onCancel: () => void;
};

/** The first step of Add connection: a remote protocol (SSH, Telnet, VNC, RDP, ARD) or a web address. */
export function ConnectionTypePicker({ onSelectProtocol, onSelectWebService, onCancel }: ConnectionTypePickerProps) {
  const t = useTranslations("devices.settingsTab.connections.picker");
  return (
    <div className="dv-sheet dst-form-sheet">
      <h3 className="dst-form-t">{t("title")}</h3>
      <div className="dst-choices">
        <button type="button" className="dst-choice" onClick={onSelectProtocol}>
          <span className="dv-nm">{t("protocol")}</span>
          <span className="dv-hint">{t("protocolHint")}</span>
        </button>
        <button type="button" className="dst-choice" onClick={onSelectWebService}>
          <span className="dv-nm">{t("web")}</span>
          <span className="dv-hint">{t("webHint")}</span>
        </button>
      </div>
      <div className="dv-form-acts dst-gap">
        <Button size="sm" variant="ghost" onClick={onCancel}>{t("cancel")}</Button>
      </div>
    </div>
  );
}
