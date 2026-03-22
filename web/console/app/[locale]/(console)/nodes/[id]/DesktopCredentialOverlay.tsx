"use client";

import { Button } from "../../../../components/ui/Button";
import type { VNCCredentialRequest } from "../../../../components/VNCViewer";

interface DesktopCredentialOverlayProps {
  credPrompt: VNCCredentialRequest | null;
  needsUsername: boolean;
  credUsername: string;
  onCredUsernameChange: (value: string) => void;
  credPassword: string;
  onCredPasswordChange: (value: string) => void;
  onSubmit: () => void;
}

export function DesktopCredentialOverlay({
  credPrompt,
  needsUsername,
  credUsername,
  onCredUsernameChange,
  credPassword,
  onCredPasswordChange,
  onSubmit,
}: DesktopCredentialOverlayProps) {
  if (!credPrompt) return null;

  return (
    <div className="absolute inset-0 flex items-center justify-center z-20 bg-black/60 backdrop-blur-sm">
      <form
        className="bg-[var(--panel)] border border-[var(--line)] rounded-lg p-4 w-80 space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <p className="text-sm font-medium text-[var(--text)]">
          {needsUsername ? "Authentication Required" : "VNC Password Required"}
        </p>
        <p className="text-xs text-[var(--muted)] text-center max-w-sm">
          {needsUsername
            ? "Enter your username and password."
            : "Enter the VNC password for this device."}
        </p>
        {needsUsername && (
          <input
            type="text"
            placeholder="Username"
            value={credUsername}
            onChange={(event) => onCredUsernameChange(event.target.value)}
            className="w-full bg-transparent border rounded-lg px-3 py-2 text-sm text-[var(--text)] border-[var(--line)]"
            autoFocus
          />
        )}
        <input
          type="password"
          placeholder="Password"
          value={credPassword}
          onChange={(event) => onCredPasswordChange(event.target.value)}
          className="w-full bg-transparent border rounded-lg px-3 py-2 text-sm text-[var(--text)] border-[var(--line)]"
          autoFocus={!needsUsername}
        />
        <Button variant="primary" type="submit" className="w-full">
          Authenticate
        </Button>
      </form>
    </div>
  );
}
