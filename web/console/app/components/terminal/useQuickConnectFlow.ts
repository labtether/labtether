"use client";

import { useCallback, useState } from "react";
import type { QuickConnectParams } from "../../hooks/useSession";
import type { HostKeyGate } from "./useHostKeyGate";

type Options = {
  /** The page asked for the address form (an address request is open). */
  requested: boolean;
  /** Closes the address request. */
  closeRequest: () => void;
  /** Opens the connection (the workspace's connectAddress). */
  connect: (params: QuickConnectParams) => void;
  gate: HostKeyGate;
};

/**
 * Connect to an address with the host key check in between. The form steps aside while the host key is checked;
 * when the person cancels there, it comes back with what they typed (address, login, password or key), and the
 * typed secret is dropped once the connection is opened or the form is closed.
 */
export function useQuickConnectFlow({ requested, closeRequest, connect, gate }: Options) {
  const [draft, setDraft] = useState<QuickConnectParams | null>(null);
  const [checking, setChecking] = useState(false);

  const onConnect = useCallback((params: QuickConnectParams) => {
    setDraft(params);
    setChecking(true);
    void gate({ host: params.host, port: params.port ?? 22, strict: params.strict_host_key !== false }).then((trusted) => {
      if (trusted) {
        setDraft(null);
        connect(params);
      }
      setChecking(false);
    });
  }, [gate, connect]);

  const onClose = useCallback(() => {
    setDraft(null);
    closeRequest();
  }, [closeRequest]);

  return { open: requested && !checking, draft, onConnect, onClose };
}
