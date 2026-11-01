"use client";

import { SignInCard } from "../../../../components/remote-toolbar/SignInCard";
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

/** The device's VNC server asks for a login: the sign-in card over the screen. */
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
    <SignInCard
      needsUsername={needsUsername}
      username={credUsername}
      onUsernameChange={onCredUsernameChange}
      password={credPassword}
      onPasswordChange={onCredPasswordChange}
      onSubmit={onSubmit}
    />
  );
}
