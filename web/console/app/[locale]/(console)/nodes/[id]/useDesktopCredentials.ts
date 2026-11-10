"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";

import type { DesktopProtocol } from "../../../../components/SessionPanel";
import type { VNCViewerHandle, VNCCredentialRequest } from "../../../../components/VNCViewer";
import { useSessionPasswordAttempt } from "../../../../hooks/useSessionPasswordAttempt";

interface UseDesktopCredentialsOptions {
  protocol: DesktopProtocol;
  vncRef: RefObject<VNCViewerHandle | null>;
  autoPassword?: string | null;
}

export function useDesktopCredentials({ protocol, vncRef, autoPassword }: UseDesktopCredentialsOptions) {
  const [credPrompt, setCredPrompt] = useState<VNCCredentialRequest | null>(null);
  const [credUsername, setCredUsername] = useState("");
  const [credPassword, setCredPassword] = useState("");
  const autoAttemptedRef = useRef(false);
  const sessionPassword = useSessionPasswordAttempt();

  const needsUsername = credPrompt?.types.includes("username") ?? false;

  // Each attempt gets its own ticket, so a new one starts here.
  useEffect(() => {
    autoAttemptedRef.current = false;
    sessionPassword.settled();
  }, [autoPassword, protocol, sessionPassword]);

  const handleCredentialsRequired = useCallback(
    (request: VNCCredentialRequest) => {
      if (protocol !== "vnc") return;
      const supportsAutoPassword = request.types.includes("password") && !request.types.includes("username");
      const trimmedAutoPassword = autoPassword?.trim() ?? "";
      // The Hub sends a session password only for a VNC server the agent started with it; a device that refused it
      // once is not sent it again.
      if (!autoAttemptedRef.current && sessionPassword.allowed() && supportsAutoPassword && trimmedAutoPassword) {
        autoAttemptedRef.current = true;
        sessionPassword.sent();
        vncRef.current?.sendCredentials({ password: trimmedAutoPassword });
        return;
      }
      setCredPrompt(request);
    },
    [autoPassword, protocol, sessionPassword, vncRef],
  );

  const handleSubmitCredentials = useCallback(() => {
    if (!credPrompt) return;
    const creds: { username?: string; password?: string } = {};
    if (credPrompt.types.includes("username")) creds.username = credUsername;
    if (credPrompt.types.includes("password")) creds.password = credPassword;
    vncRef.current?.sendCredentials(creds);
    setCredPrompt(null);
    setCredPassword("");
  }, [credPassword, credPrompt, credUsername, vncRef]);

  const resetCredentialForm = useCallback(() => {
    autoAttemptedRef.current = false;
    setCredPrompt(null);
    setCredUsername("");
    setCredPassword("");
  }, []);

  const dismissCredentialPrompt = useCallback(() => {
    setCredPrompt(null);
  }, []);

  return {
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
    takeSessionPasswordRefusal: sessionPassword.takeRefusal,
  };
}
