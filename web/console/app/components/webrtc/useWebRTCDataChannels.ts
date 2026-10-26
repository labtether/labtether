"use client";

import { useCallback, useRef } from "react";

interface ClipboardChannelMessage {
  type?: string;
  text?: string;
  error?: string;
}

interface FileTransferChannelMessage {
  type?: string;
  request_id?: string;
  done?: boolean;
  bytes_written?: number;
  error?: string;
}

type PendingRequest<T> = {
  resolve: (value: T) => void;
  reject: (reason: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};

type PendingClipboardRequest = PendingRequest<string> & { mode: "get" | "set" };
const RESPONSE_TIMEOUT_MS = 15_000;

function createRequestID(): string {
  return typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `req-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

function requireOpenChannel(channel: RTCDataChannel | null, label: string): RTCDataChannel {
  if (!channel || channel.readyState !== "open") throw new Error(`${label} channel unavailable`);
  return channel;
}

export function useWebRTCDataChannels() {
  const clipboardDCRef = useRef<RTCDataChannel | null>(null);
  const fileDCRef = useRef<RTCDataChannel | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const pendingClipboardRef = useRef<PendingClipboardRequest | null>(null);
  const fileTransferWaitersRef = useRef(new Map<string, PendingRequest<FileTransferChannelMessage>>());

  const rejectClipboard = useCallback((reason: string) => {
    const pending = pendingClipboardRef.current;
    pendingClipboardRef.current = null;
    if (pending) {
      clearTimeout(pending.timer);
      pending.reject(new Error(reason));
    }
  }, []);

  const rejectFileTransfers = useCallback((reason: string) => {
    fileTransferWaitersRef.current.forEach(({ reject, timer }) => {
      clearTimeout(timer);
      reject(new Error(reason));
    });
    fileTransferWaitersRef.current.clear();
  }, []);

  const clipboardRequest = useCallback((mode: "get" | "set", text?: string) => {
    const channel = requireOpenChannel(clipboardDCRef.current, "clipboard");
    if (pendingClipboardRef.current) throw new Error("clipboard request already in progress");
    return new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => {
        rejectClipboard("clipboard request timed out");
        // Replies have no request IDs: a late reply must never complete a later request.
        if (clipboardDCRef.current === channel) clipboardDCRef.current = null;
        channel.close();
      }, RESPONSE_TIMEOUT_MS);
      pendingClipboardRef.current = { resolve, reject, mode, timer };
      try {
        channel.send(JSON.stringify({ type: mode, format: "text", ...(mode === "set" ? { text } : {}) }));
      } catch (error) {
        clearTimeout(timer);
        pendingClipboardRef.current = null;
        reject(error);
      }
    });
  }, [rejectClipboard]);

  const requestClipboardText = useCallback(async () => clipboardRequest("get"), [clipboardRequest]);
  const writeClipboardText = useCallback(async (text: string) => {
    await clipboardRequest("set", text);
  }, [clipboardRequest]);

  const fileRequest = useCallback((channel: RTCDataChannel, requestID: string, payload: Record<string, unknown>) => {
    return new Promise<FileTransferChannelMessage>((resolve, reject) => {
      const timer = setTimeout(() => {
        fileTransferWaitersRef.current.delete(requestID);
        reject(new Error("file transfer timed out"));
      }, RESPONSE_TIMEOUT_MS);
      fileTransferWaitersRef.current.set(requestID, { resolve, reject, timer });
      try {
        requireOpenChannel(channel, "file transfer");
        channel.send(JSON.stringify(payload));
      } catch (error) {
        clearTimeout(timer);
        fileTransferWaitersRef.current.delete(requestID);
        reject(error);
      }
    });
  }, []);

  const uploadFile = useCallback(async (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => {
    const channel = requireOpenChannel(fileDCRef.current, "file transfer");
    const requestID = createRequestID();
    const envelope = { type: "chunk", request_id: requestID, path: targetPath, offset: file.size, done: true, data: "" };
    const envelopeBytes = new TextEncoder().encode(JSON.stringify(envelope)).length;
    const negotiatedLimit = peerRef.current?.sctp?.maxMessageSize;
    const messageLimit = negotiatedLimit && Number.isFinite(negotiatedLimit)
      ? Math.min(64 * 1024, negotiatedLimit) : 64 * 1024;
    // Base64 expands each three bytes to four. Leave room for JSON and UTF-8 paths.
    const chunkSize = Math.min(32 * 1024, Math.floor((messageLimit - envelopeBytes - 16) / 4) * 3);
    if (chunkSize <= 0) throw new Error("file transfer path exceeds channel message limit");
    const ready = await fileRequest(channel, requestID, {
      type: "start", request_id: requestID, name: file.name, path: targetPath,
    });
    if (ready.type !== "ready") throw new Error("invalid file transfer response");

    let offset = 0;
    do {
      const drainDeadline = Date.now() + RESPONSE_TIMEOUT_MS;
      while (channel.bufferedAmount > 256 * 1024) {
        requireOpenChannel(channel, "file transfer");
        if (fileDCRef.current !== channel) throw new Error("file transfer session ended");
        if (Date.now() >= drainDeadline) throw new Error("file transfer timed out");
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      const chunk = await file.slice(offset, offset + chunkSize).arrayBuffer();
      if (fileDCRef.current !== channel) throw new Error("file transfer session ended");
      if (chunk.byteLength === 0 && offset < file.size) throw new Error("file read ended before upload completed");
      const loaded = offset + chunk.byteLength;
      const done = loaded >= file.size;
      const ack = await fileRequest(channel, requestID, {
        type: "chunk", request_id: requestID, path: targetPath,
        offset, data: arrayBufferToBase64(chunk), done,
      });
      if (ack.type !== "ack" || (ack.bytes_written ?? 0) !== loaded || (done && ack.done !== true)) {
        throw new Error("file transfer acknowledgment mismatch");
      }
      offset = loaded;
      onProgress?.(loaded, file.size);
    } while (offset < file.size);
  }, [fileRequest]);

  const closeDataChannels = useCallback((reason: string) => {
    rejectClipboard(reason);
    rejectFileTransfers(reason);
    for (const channel of [clipboardDCRef.current, fileDCRef.current]) {
      if (!channel) continue;
      channel.onmessage = null;
      channel.onclose = null;
      channel.onerror = null;
      channel.close();
    }
    clipboardDCRef.current = null;
    fileDCRef.current = null;
    peerRef.current = null;
  }, [rejectClipboard, rejectFileTransfers]);

  const openDataChannels = useCallback((pc: RTCPeerConnection) => {
    closeDataChannels("session replaced");
    peerRef.current = pc;
    const clipboard = pc.createDataChannel("clipboard", { ordered: true });
    const files = pc.createDataChannel("file-transfer", { ordered: true });
    clipboardDCRef.current = clipboard;
    fileDCRef.current = files;
    clipboard.onclose = () => {
      if (clipboardDCRef.current === clipboard) rejectClipboard("clipboard channel closed");
    };
    clipboard.onerror = () => {
      if (clipboardDCRef.current === clipboard) rejectClipboard("clipboard channel failed");
    };
    files.onclose = () => {
      if (fileDCRef.current === files) rejectFileTransfers("file transfer channel closed");
    };
    files.onerror = () => {
      if (fileDCRef.current === files) rejectFileTransfers("file transfer channel failed");
    };

    clipboard.onmessage = (event) => {
      if (clipboardDCRef.current !== clipboard) return;
      let payload: ClipboardChannelMessage;
      try { payload = JSON.parse(String(event.data)); } catch { return; }
      if (!payload || typeof payload !== "object") return;
      const pending = pendingClipboardRef.current;
      if (!pending) return;
      if (payload.type === "error") {
        rejectClipboard(payload.error || "clipboard request failed");
      } else if ((pending.mode === "get" && payload.type === "data") || (pending.mode === "set" && payload.type === "ack")) {
        clearTimeout(pending.timer);
        pendingClipboardRef.current = null;
        pending.resolve(typeof payload.text === "string" ? payload.text : "");
      }
    };

    files.onmessage = (event) => {
      if (fileDCRef.current !== files) return;
      let payload: FileTransferChannelMessage;
      try { payload = JSON.parse(String(event.data)); } catch { return; }
      if (!payload || typeof payload.request_id !== "string") return;
      const requestID = payload.request_id.trim();
      const waiter = fileTransferWaitersRef.current.get(requestID);
      if (!waiter) return;
      clearTimeout(waiter.timer);
      fileTransferWaitersRef.current.delete(requestID);
      if (payload.type === "error") waiter.reject(new Error(payload.error || "file transfer failed"));
      else waiter.resolve(payload);
    };
  }, [closeDataChannels, rejectClipboard, rejectFileTransfers]);

  const hasDataChannels = useCallback(() => Boolean(clipboardDCRef.current || fileDCRef.current), []);
  return { openDataChannels, closeDataChannels, hasDataChannels, requestClipboardText, writeClipboardText, uploadFile };
}
