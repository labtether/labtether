"use client";

import { useCallback, useRef, useState } from "react";

interface ClipboardSyncState {
  syncing: boolean;
  lastSync: "idle" | "success" | "error";
  error: string | null;
}

interface UseClipboardSyncOptions {
  nodeId: string;
  enabled: boolean;
  readRemoteText?: () => Promise<string>;
  writeRemoteText?: (text: string) => Promise<void>;
}

export function useClipboardSync({
  nodeId,
  enabled,
  readRemoteText,
  writeRemoteText,
}: UseClipboardSyncOptions) {
  const [state, setState] = useState<ClipboardSyncState>({
    syncing: false,
    lastSync: "idle",
    error: null,
  });
  const abortRef = useRef<AbortController | null>(null);

  // Read remote clipboard → write to local
  const pullFromRemote = useCallback(async () => {
    if (!enabled || !nodeId) return;
    setState((s) => ({ ...s, syncing: true, error: null }));
    try {
      let text = "";
      if (readRemoteText) {
        text = await readRemoteText();
      } else {
        abortRef.current = new AbortController();
        const res = await fetch(`/api/v1/nodes/${nodeId}/clipboard/get`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ format: "text" }),
          signal: abortRef.current.signal,
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (data.error) throw new Error(data.error);
        text = data.text ?? "";
      }
      if (text) {
        await navigator.clipboard.writeText(text);
      }
      setState({ syncing: false, lastSync: "success", error: null });
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        setState({
          syncing: false,
          lastSync: "error",
          error: (err as Error).message,
        });
      }
    }
  }, [enabled, nodeId, readRemoteText]);

  // Read local clipboard → write to remote
  const pushToRemote = useCallback(async () => {
    if (!enabled || !nodeId) return;
    setState((s) => ({ ...s, syncing: true, error: null }));
    try {
      const text = await navigator.clipboard.readText();
      if (!text) {
        setState({ syncing: false, lastSync: "success", error: null });
        return;
      }
      if (writeRemoteText) {
        await writeRemoteText(text);
      } else {
        abortRef.current = new AbortController();
        const res = await fetch(`/api/v1/nodes/${nodeId}/clipboard/set`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ format: "text", text }),
          signal: abortRef.current.signal,
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (data.error) throw new Error(data.error);
      }
      setState({ syncing: false, lastSync: "success", error: null });
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        setState({
          syncing: false,
          lastSync: "error",
          error: (err as Error).message,
        });
      }
    }
  }, [enabled, nodeId, writeRemoteText]);

  return { ...state, pullFromRemote, pushToRemote };
}
