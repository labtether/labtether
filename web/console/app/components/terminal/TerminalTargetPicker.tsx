"use client";

import { useTranslations } from "next-intl";
import type { Asset } from "../../console/models";
import { Modal } from "../ui/Modal";
import TerminalPicker from "./TerminalPicker";

interface TargetPickerProps {
  assets: Asset[];
  /** The pane's current device, listed first as "(this device)". */
  selectedTargetID: string;
  connectedAgentIDs: ReadonlySet<string>;
  onSelect: (id: string) => void;
  /** "Connect to <address>": the address form, for this pane. */
  onAddress: (address: string) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Change a pane's device: the Terminal picker in a dialog. Choosing a row closes it and opens that device. */
export function TerminalTargetPicker({ assets, selectedTargetID, connectedAgentIDs, onSelect, onAddress, open, onOpenChange }: TargetPickerProps) {
  const t = useTranslations("terminal.pane");
  const close = () => onOpenChange(false);
  return (
    <Modal open={open} onClose={close} title={t("chooseDevice")} className="tm-dlg-wide">
      <div className="tm-dlg tm-dlg-scroll">
        <TerminalPicker
          assets={assets}
          connectedAgentIds={connectedAgentIDs}
          firstId={selectedTargetID}
          autoFocus
          compact
          onPick={(id) => {
            close();
            if (id !== selectedTargetID) onSelect(id);
          }}
          onAddress={(address) => {
            close();
            onAddress(address);
          }}
        />
      </div>
    </Modal>
  );
}
