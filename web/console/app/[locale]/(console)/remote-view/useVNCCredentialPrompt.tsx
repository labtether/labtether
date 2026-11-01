"use client";

import { useCallback, useRef, useState, type RefObject } from "react";
import type { VNCViewerHandle, VNCCredentialRequest } from "../../../components/VNCViewer";
import { SignInCard } from "../../../components/remote-toolbar/SignInCard";

type CredentialPromptOptions = {
  vncRef: RefObject<VNCViewerHandle | null>;
  initialCredentials?: { username?: string; password?: string };
  vncPassword: string | null;
};

export function useVNCCredentialPrompt({ vncRef, initialCredentials, vncPassword }: CredentialPromptOptions) {
  // ── Credential state ──
  const [credentialRequest, setCredentialRequest] = useState<VNCCredentialRequest | null>(null);
  const [credPassword, setCredPassword] = useState("");
  const [credUsername, setCredUsername] = useState("");
  // Track whether initial credentials have been consumed so we only auto-submit once.
  const initialCredentialsUsedRef = useRef(false);

  const handleCredentialsRequired = useCallback((request: VNCCredentialRequest) => {
    // Priority 1: initial credentials from the pre-connect dialog (one-shot)
    if (initialCredentials && !initialCredentialsUsedRef.current) {
      const hasCreds =
        (request.types.includes("username") && !!initialCredentials.username) ||
        (request.types.includes("password") && !!initialCredentials.password);
      if (hasCreds) {
        initialCredentialsUsedRef.current = true;
        const creds: { username?: string; password?: string } = {};
        if (request.types.includes("username")) creds.username = initialCredentials.username;
        if (request.types.includes("password")) creds.password = initialCredentials.password;
        vncRef.current?.sendCredentials(creds);
        return;
      }
    }
    // Priority 2: VNC password from the stream ticket (password-only flows)
    const autoPassword = vncPassword;
    if (autoPassword && request.types.includes("password") && !request.types.includes("username")) {
      vncRef.current?.sendCredentials({ password: autoPassword });
      return;
    }
    // Priority 3: show the runtime credential overlay
    setCredentialRequest(request);
  }, [initialCredentials, vncPassword, vncRef]);

  const handleCredentialSubmit = useCallback(() => {
    if (!credentialRequest) return;
    const creds: { username?: string; password?: string } = {};
    if (credentialRequest.types.includes("username")) creds.username = credUsername;
    if (credentialRequest.types.includes("password")) creds.password = credPassword;
    vncRef.current?.sendCredentials(creds);
    setCredentialRequest(null);
    setCredPassword("");
    setCredUsername("");
  }, [credentialRequest, credPassword, credUsername, vncRef]);

  // ── Credential overlay ──
  const credentialOverlay = credentialRequest ? (
    <SignInCard
      needsUsername={credentialRequest.types?.includes("username") ?? false}
      username={credUsername}
      onUsernameChange={setCredUsername}
      password={credPassword}
      onPasswordChange={setCredPassword}
      onSubmit={handleCredentialSubmit}
      onCancel={() => {
        setCredentialRequest(null);
        setCredPassword("");
        setCredUsername("");
      }}
    />
  ) : null;

  return { credentialOverlay, handleCredentialsRequired };
}
