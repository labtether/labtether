"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

type ConnectedAgentsContextValue = {
  connectedAgentIds: Set<string>;
  agentTmuxStatus: Map<string, boolean>;
  refreshConnected: () => Promise<void>;
};

const ConnectedAgentsContext = createContext<ConnectedAgentsContextValue | null>(null);

function setsEqual(left: Set<string>, right: Set<string>): boolean {
  if (left.size !== right.size) {
    return false;
  }
  for (const value of left) {
    if (!right.has(value)) {
      return false;
    }
  }
  return true;
}

export function ConnectedAgentsProvider({ children }: { children: ReactNode }) {
  const [connectedAgentIds, setConnectedAgentIds] = useState<Set<string>>(new Set());
  const [agentTmuxStatus, setAgentTmuxStatus] = useState<Map<string, boolean>>(new Map());
  const refreshRequestSeqRef = useRef(0);
  const refreshAbortRef = useRef<AbortController | null>(null);

  const refreshConnected = useCallback(async () => {
    const requestID = ++refreshRequestSeqRef.current;
    refreshAbortRef.current?.abort();
    const controller = new AbortController();
    refreshAbortRef.current = controller;
    try {
      const res = await fetch("/api/agents/connected", { cache: "no-store", signal: controller.signal });
      if (!res.ok) {
        if (requestID === refreshRequestSeqRef.current) {
          setConnectedAgentIds((current) => (current.size === 0 ? current : new Set()));
        }
        return;
      }
      const data = (await res.json()) as {
        assets?: string[];
        assetsInfo?: Array<{ id: string; has_tmux: boolean; platform?: string }>;
      };
      if (requestID !== refreshRequestSeqRef.current) {
        return;
      }
      const next = new Set(data.assets ?? []);
      setConnectedAgentIds((current) => (setsEqual(current, next) ? current : next));
      if (data.assetsInfo) {
        const tmuxMap = new Map<string, boolean>();
        for (const info of data.assetsInfo) {
          tmuxMap.set(info.id, info.has_tmux);
        }
        setAgentTmuxStatus(tmuxMap);
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      if (requestID === refreshRequestSeqRef.current) {
        setConnectedAgentIds((current) => (current.size === 0 ? current : new Set()));
      }
    } finally {
      if (refreshAbortRef.current === controller) {
        refreshAbortRef.current = null;
      }
    }
  }, []);

  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const startInterval = useCallback(() => {
    if (intervalRef.current !== null) return;
    intervalRef.current = setInterval(() => void refreshConnected(), 30_000);
  }, [refreshConnected]);

  const stopInterval = useCallback(() => {
    if (intervalRef.current === null) return;
    clearInterval(intervalRef.current);
    intervalRef.current = null;
  }, []);

  useEffect(() => {
    void refreshConnected();
    startInterval();

    const handleVisibilityChange = () => {
      if (document.visibilityState === "hidden") {
        stopInterval();
      } else {
        void refreshConnected();
        startInterval();
      }
    };

    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      stopInterval();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      refreshAbortRef.current?.abort();
      refreshAbortRef.current = null;
    };
  }, [refreshConnected, startInterval, stopInterval]);

  const value = useMemo(
    () => ({ connectedAgentIds, agentTmuxStatus, refreshConnected }),
    [connectedAgentIds, agentTmuxStatus, refreshConnected]
  );

  return (
    <ConnectedAgentsContext.Provider value={value}>
      {children}
    </ConnectedAgentsContext.Provider>
  );
}

export function useConnectedAgentsContext(): ConnectedAgentsContextValue {
  const context = useContext(ConnectedAgentsContext);
  if (!context) {
    throw new Error("useConnectedAgentsContext must be used within a ConnectedAgentsProvider");
  }
  return context;
}
