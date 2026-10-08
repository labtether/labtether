"use client";

import { useCallback, useRef, useState, type RefObject } from "react";
import type { VNCViewerHandle, VNCCredentialRequest } from "../../../components/VNCViewer";

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
    <div className="absolute inset-0 flex items-center justify-center bg-black/60 z-20">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="remote-view-auth-title"
        className="bg-[var(--surface)] border border-[var(--line)] rounded-lg p-6 w-80 shadow-lg"
      >
        <h3
          id="remote-view-auth-title"
          className="text-sm font-semibold text-[var(--text)] mb-3"
        >
          Authentication Required
        </h3>
        {credentialRequest.types?.includes("username") && (
          <input
            type="text"
            aria-label="Username"
            placeholder="Username"
            value={credUsername}
            onChange={(e) => setCredUsername(e.target.value)}
            className="w-full mb-2 px-3 py-1.5 rounded border border-[var(--line)] bg-[var(--input)] text-[var(--text)] text-sm"
            autoFocus
          />
        )}
        {credentialRequest.types?.includes("password") && (
          <input
            type="password"
            aria-label="Password"
            placeholder="Password"
            value={credPassword}
            onChange={(e) => setCredPassword(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleCredentialSubmit();
            }}
            className="w-full mb-3 px-3 py-1.5 rounded border border-[var(--line)] bg-[var(--input)] text-[var(--text)] text-sm"
            autoFocus={!credentialRequest.types?.includes("username")}
          />
        )}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={() => {
              setCredentialRequest(null);
              setCredPassword("");
              setCredUsername("");
            }}
            className="px-3 py-1.5 text-xs rounded border border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)]"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleCredentialSubmit}
            className="px-3 py-1.5 text-xs rounded bg-[var(--accent)] text-white"
          >
            Connect
          </button>
        </div>
      </div>
    </div>
  ) : null;

  return { credentialOverlay, handleCredentialsRequired };
}
