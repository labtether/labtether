"use client";

import { useEffect } from "react";

type UseDesktopSessionPresenceArgs = {
  nodeId: string;
  nodeName: string;
  connectionState: string;
  activeSessionId: string;
  registerSession: (nodeId: string, nodeName: string, sessionId: string) => void;
  clearSession: () => void;
};

export function useDesktopSessionPresence({
  nodeId,
  nodeName,
  connectionState,
  activeSessionId,
  registerSession,
  clearSession,
}: UseDesktopSessionPresenceArgs) {
  useEffect(() => {
    if (connectionState === "connected" && activeSessionId) {
      registerSession(nodeId, nodeName, activeSessionId);
    }
  }, [activeSessionId, connectionState, nodeId, nodeName, registerSession]);

  useEffect(() => {
    if (connectionState !== "connected") {
      clearSession();
    }
  }, [clearSession, connectionState]);
}
