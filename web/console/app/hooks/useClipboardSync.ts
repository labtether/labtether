"use client";

import { useCallback, useEffect, useRef, useState } from "react";

interface ClipboardSyncState {
  syncing: boolean;
  lastSync: "idle" | "success" | "error";
  error: string | null;
}

interface UseClipboardSyncOptions {
  nodeId: string;
  enabled: boolean;
  sessionKey?: string | null;
  readRemoteText?: () => Promise<string>;
  writeRemoteText?: (text: string) => Promise<void>;
}

const idleState: ClipboardSyncState = { syncing: false, lastSync: "idle", error: null };

export function useClipboardSync({ nodeId, enabled, sessionKey, readRemoteText, writeRemoteText }: UseClipboardSyncOptions) {
  const [state, setState] = useState<ClipboardSyncState>(idleState);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    setState(idleState);
    return () => {
      abortRef.current?.abort();
      abortRef.current = null;
    };
  }, [nodeId, enabled, sessionKey]);

  const runSync = useCallback(async (direction: "pull" | "push") => {
    if (!enabled || !nodeId || abortRef.current) return;
    const controller = new AbortController();
    abortRef.current = controller;
    const assertActive = () => controller.signal.throwIfAborted();
    setState({ syncing: true, lastSync: "idle", error: null });

    const remoteRequest = async (action: "get" | "set", text?: string) => {
      const res = await fetch(`/api/v1/nodes/${encodeURIComponent(nodeId)}/clipboard/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ format: "text", ...(action === "set" ? { text } : {}) }),
        signal: controller.signal,
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      if (data.error) throw new Error(data.error);
      return typeof data.text === "string" ? data.text : "";
    };

    try {
      if (direction === "pull") {
        const textPromise = (readRemoteText ? readRemoteText() : remoteRequest("get")).then((text) => {
          assertActive();
          return text;
        });
        // Safari requires clipboard.write during the click, before the remote read finishes.
        if (typeof ClipboardItem !== "undefined" && navigator.clipboard?.write) {
          const blobPromise = textPromise.then((text) => new Blob([text], { type: "text/plain" }));
          // The browser can reject permission before it consumes the promised item.
          void blobPromise.catch(() => {});
          await navigator.clipboard.write([new ClipboardItem({ "text/plain": blobPromise })]);
        } else {
          const text = await textPromise;
          assertActive();
          await navigator.clipboard.writeText(text);
        }
      } else {
        const text = await navigator.clipboard.readText();
        assertActive();
        if (writeRemoteText) await writeRemoteText(text);
        else await remoteRequest("set", text);
      }
      assertActive();
      setState({ syncing: false, lastSync: "success", error: null });
    } catch (err) {
      if (!controller.signal.aborted) {
        setState({ syncing: false, lastSync: "error", error: (err as Error).message });
      }
    } finally {
      controller.abort();
      if (abortRef.current === controller) abortRef.current = null;
    }
  }, [enabled, nodeId, readRemoteText, writeRemoteText]);

  const pullFromRemote = useCallback(() => runSync("pull"), [runSync]);
  const pushToRemote = useCallback(() => runSync("push"), [runSync]);
  return { ...state, pullFromRemote, pushToRemote };
}
