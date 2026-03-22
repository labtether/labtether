"use client";

import { Terminal, Globe } from "lucide-react";
import { Button } from "../../../../components/ui/Button";

type ConnectionTypePickerProps = {
  onSelectProtocol: () => void;
  onSelectWebService: () => void;
  onCancel: () => void;
};

export function ConnectionTypePicker({ onSelectProtocol, onSelectWebService, onCancel }: ConnectionTypePickerProps) {
  return (
    <div className="mb-4 rounded-lg border border-[var(--line)] p-4">
      <p className="text-xs font-medium text-[var(--text)] mb-3">What type of connection?</p>
      <div className="grid grid-cols-2 gap-3 mb-3">
        <button
          type="button"
          onClick={onSelectProtocol}
          className="flex flex-col items-center gap-2 rounded-lg border border-[var(--line)] p-4
            hover:border-purple-400/40 hover:bg-purple-500/5 transition-colors text-center"
        >
          <span className="flex items-center justify-center h-9 w-9 rounded-md bg-purple-500/10">
            <Terminal size={16} className="text-purple-400" />
          </span>
          <span className="text-sm font-medium text-[var(--text)]">Protocol</span>
          <span className="text-[11px] text-[var(--muted)]">SSH, VNC, RDP, Telnet, ARD</span>
        </button>
        <button
          type="button"
          onClick={onSelectWebService}
          className="flex flex-col items-center gap-2 rounded-lg border border-[var(--line)] p-4
            hover:border-sky-400/40 hover:bg-sky-500/5 transition-colors text-center"
        >
          <span className="flex items-center justify-center h-9 w-9 rounded-md bg-sky-500/10">
            <Globe size={16} className="text-sky-400" />
          </span>
          <span className="text-sm font-medium text-[var(--text)]">Web Service</span>
          <span className="text-[11px] text-[var(--muted)]">HTTP/HTTPS URL</span>
        </button>
      </div>
      <div className="flex justify-end">
        <Button size="sm" variant="ghost" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  );
}
