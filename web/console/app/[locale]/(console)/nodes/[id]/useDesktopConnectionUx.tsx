"use client";

import type { RefObject } from "react";

import type { VNCViewerHandle } from "../../../../components/VNCViewer";
import type { DesktopProtocol } from "../../../../hooks/useSession";

import { DesktopCredentialOverlay } from "./DesktopCredentialOverlay";
import { useDesktopCredentials } from "./useDesktopCredentials";

type UseDesktopConnectionUxOptions = {
  protocol: DesktopProtocol;
  vncRef: RefObject<VNCViewerHandle | null>;
  autoPassword?: string | null;
};

export function useDesktopConnectionUx({
  protocol,
  vncRef,
  autoPassword,
}: UseDesktopConnectionUxOptions) {
  const {
    credPrompt,
    credUsername,
    setCredUsername,
    credPassword,
    setCredPassword,
    needsUsername,
    handleCredentialsRequired,
    handleSubmitCredentials,
    resetCredentialForm,
    dismissCredentialPrompt,
    takeSessionPasswordRefusal,
  } = useDesktopCredentials({
    protocol,
    vncRef,
    autoPassword,
  });

  // The sign-in card had no way out. onCancel is the page's Disconnect, which exists only after this hook has run.
  const renderCredentialOverlay = (onCancel: () => void) =>
    protocol === "vnc" && credPrompt ? (
      <DesktopCredentialOverlay
        credPrompt={credPrompt}
        needsUsername={needsUsername}
        credUsername={credUsername}
        onCredUsernameChange={setCredUsername}
        credPassword={credPassword}
        onCredPasswordChange={setCredPassword}
        onSubmit={handleSubmitCredentials}
        onCancel={onCancel}
      />
    ) : undefined;

  return {
    handleCredentialsRequired,
    resetCredentialForm,
    dismissCredentialPrompt,
    renderCredentialOverlay,
    takeSessionPasswordRefusal,
  };
}
