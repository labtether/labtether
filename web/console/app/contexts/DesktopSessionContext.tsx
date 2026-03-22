"use client";

import { createContext, useCallback, useContext, useState } from "react";
import type { ReactNode } from "react";

interface ActiveDesktopSession {
  nodeId: string;
  nodeName: string;
  sessionId: string;
}

interface DesktopSessionContextValue {
  activeSession: ActiveDesktopSession | null;
  registerSession: (nodeId: string, nodeName: string, sessionId: string) => void;
  clearSession: () => void;
}

const DesktopSessionContext = createContext<DesktopSessionContextValue>({
  activeSession: null,
  registerSession: () => {},
  clearSession: () => {},
});

export function DesktopSessionProvider({ children }: { children: ReactNode }) {
  const [activeSession, setActiveSession] = useState<ActiveDesktopSession | null>(null);

  const registerSession = useCallback((nodeId: string, nodeName: string, sessionId: string) => {
    setActiveSession({ nodeId, nodeName, sessionId });
  }, []);

  const clearSession = useCallback(() => {
    setActiveSession(null);
  }, []);

  return (
    <DesktopSessionContext.Provider value={{ activeSession, registerSession, clearSession }}>
      {children}
    </DesktopSessionContext.Provider>
  );
}

export function useDesktopSession() {
  return useContext(DesktopSessionContext);
}
