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
  } = useDesktopCredentials({
    protocol,
    vncRef,
    autoPassword,
  });

  const credentialOverlay =
    protocol === "vnc" && credPrompt ? (
      <DesktopCredentialOverlay
        credPrompt={credPrompt}
        needsUsername={needsUsername}
        credUsername={credUsername}
        onCredUsernameChange={setCredUsername}
        credPassword={credPassword}
        onCredPasswordChange={setCredPassword}
        onSubmit={handleSubmitCredentials}
      />
    ) : undefined;

  return {
    handleCredentialsRequired,
    resetCredentialForm,
    dismissCredentialPrompt,
    credentialOverlay,
  };
}
