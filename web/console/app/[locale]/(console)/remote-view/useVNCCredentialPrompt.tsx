"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { VNCViewerHandle, VNCCredentialRequest } from "../../../components/VNCViewer";
import { SignInCard } from "../../../components/remote-toolbar/SignInCard";
import { useSessionPasswordAttempt } from "../../../hooks/useSessionPasswordAttempt";

type CredentialPromptOptions = {
  vncRef: RefObject<VNCViewerHandle | null>;
  initialCredentials?: { username?: string; password?: string };
  vncPassword: string | null;
  /** Cancel on the sign-in card: ends the attempt (the page disconnects and closes the session). */
  onCancel: () => void;
  /** The device's name when it is a Mac: the card then asks for an account on it. */
  macName?: string;
};

export function useVNCCredentialPrompt({ vncRef, initialCredentials, vncPassword, onCancel, macName }: CredentialPromptOptions) {
  // ── Credential state ──
  const [credentialRequest, setCredentialRequest] = useState<VNCCredentialRequest | null>(null);
  const [credPassword, setCredPassword] = useState("");
  const [credUsername, setCredUsername] = useState("");
  // Track whether initial credentials have been consumed so we only auto-submit once.
  const initialCredentialsUsedRef = useRef(false);
  const sessionPassword = useSessionPasswordAttempt();

  // Each attempt gets its own ticket, so a new one starts here.
  useEffect(() => {
    sessionPassword.settled();
  }, [sessionPassword, vncPassword]);

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
    // Priority 2: VNC password from the stream ticket (password-only flows). The Hub sends one only for a VNC server
    // the agent started with it; a device that refused it once is not sent it again.
    const autoPassword = vncPassword;
    if (
      autoPassword &&
      sessionPassword.allowed() &&
      request.types.includes("password") &&
      !request.types.includes("username")
    ) {
      sessionPassword.sent();
      vncRef.current?.sendCredentials({ password: autoPassword });
      return;
    }
    // Priority 3: show the runtime credential overlay
    setCredentialRequest(request);
  }, [initialCredentials, sessionPassword, vncPassword, vncRef]);

  const clearPrompt = useCallback(() => {
    setCredentialRequest(null);
    setCredPassword("");
    setCredUsername("");
  }, []);

  const handleCredentialSubmit = useCallback(() => {
    if (!credentialRequest) return;
    const creds: { username?: string; password?: string } = {};
    if (credentialRequest.types.includes("username")) creds.username = credUsername;
    if (credentialRequest.types.includes("password")) creds.password = credPassword;
    vncRef.current?.sendCredentials(creds);
    clearPrompt();
  }, [clearPrompt, credentialRequest, credPassword, credUsername, vncRef]);

  // Cancel used to close only the card, which left the page on "Opening stream" with the session still open.
  const handleCredentialCancel = useCallback(() => {
    clearPrompt();
    onCancel();
  }, [clearPrompt, onCancel]);

  // ── Credential overlay ──
  const credentialOverlay = credentialRequest ? (
    <SignInCard
      needsUsername={credentialRequest.types?.includes("username") ?? false}
      username={credUsername}
      onUsernameChange={setCredUsername}
      password={credPassword}
      onPasswordChange={setCredPassword}
      onSubmit={handleCredentialSubmit}
      onCancel={handleCredentialCancel}
      macName={macName}
    />
  ) : null;

  return {
    credentialOverlay,
    handleCredentialsRequired,
    takeSessionPasswordRefusal: sessionPassword.takeRefusal,
  };
}
