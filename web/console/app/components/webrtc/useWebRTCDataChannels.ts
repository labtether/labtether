"use client";

import { useCallback, useRef } from "react";

interface ClipboardChannelMessage {
  type?: string;
  format?: string;
  text?: string;
  error?: string;
}

interface FileTransferChannelMessage {
  type?: string;
  request_id?: string;
  path?: string;
  data?: string;
  done?: boolean;
  bytes_written?: number;
  error?: string;
}

type PendingClipboardRequest =
  | {
      resolve: (value: string) => void;
      reject: (reason?: unknown) => void;
      mode: "get";
    }
  | {
      resolve: () => void;
      reject: (reason?: unknown) => void;
      mode: "set";
    };

function createRequestID(): string {
  if (
    typeof crypto !== "undefined" &&
    typeof crypto.randomUUID === "function"
  ) {
    return crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

export function useWebRTCDataChannels() {
  const clipboardDCRef = useRef<RTCDataChannel | null>(null);
  const fileDCRef = useRef<RTCDataChannel | null>(null);
  const pendingClipboardRef = useRef<PendingClipboardRequest | null>(null);
  const fileTransferWaitersRef = useRef(
    new Map<
      string,
      {
        resolve: (value: FileTransferChannelMessage) => void;
        reject: (reason?: unknown) => void;
      }
    >(),
  );
  const withOpenChannel = useCallback(
    (channel: RTCDataChannel | null, label: string) => {
      if (!channel || channel.readyState !== "open") {
        throw new Error(`${label} channel unavailable`);
      }
      return channel;
    },
    [],
  );

  const requestClipboardText = useCallback(async () => {
    const channel = withOpenChannel(clipboardDCRef.current, "clipboard");
    if (pendingClipboardRef.current) {
      throw new Error("clipboard request already in progress");
    }
    return new Promise<string>((resolve, reject) => {
      pendingClipboardRef.current = { resolve, reject, mode: "get" };
      channel.send(JSON.stringify({ type: "get", format: "text" }));
    });
  }, [withOpenChannel]);

  const writeClipboardText = useCallback(
    async (text: string) => {
      const channel = withOpenChannel(clipboardDCRef.current, "clipboard");
      if (pendingClipboardRef.current) {
        throw new Error("clipboard request already in progress");
      }
      return new Promise<void>((resolve, reject) => {
        pendingClipboardRef.current = { resolve, reject, mode: "set" };
        channel.send(JSON.stringify({ type: "set", format: "text", text }));
      });
    },
    [withOpenChannel],
  );

  const waitForFileTransferResponse = useCallback((requestID: string) => {
    return new Promise<FileTransferChannelMessage>((resolve, reject) => {
      fileTransferWaitersRef.current.set(requestID, { resolve, reject });
    });
  }, []);

  const uploadFile = useCallback(
    async (
      file: File,
      targetPath: string,
      onProgress?: (loaded: number, total: number) => void,
    ) => {
      const channel = withOpenChannel(fileDCRef.current, "file transfer");
      const requestID = createRequestID();
      const readyPromise = waitForFileTransferResponse(requestID);
      channel.send(
        JSON.stringify({
          type: "start",
          request_id: requestID,
          name: file.name,
          path: targetPath,
        }),
      );
      const ready = await readyPromise;
      if (ready.type === "error") {
        throw new Error(ready.error || "file transfer failed");
      }

      const chunkSize = 64 * 1024;
      let offset = 0;
      while (offset < file.size) {
        while (channel.bufferedAmount > 256 * 1024) {
          await new Promise((resolve) => window.setTimeout(resolve, 25));
        }
        const chunk = await file
          .slice(offset, offset + chunkSize)
          .arrayBuffer();
        const loaded = Math.min(file.size, offset + chunk.byteLength);
        const ackPromise = waitForFileTransferResponse(requestID);
        channel.send(
          JSON.stringify({
            type: "chunk",
            request_id: requestID,
            path: targetPath,
            data: arrayBufferToBase64(chunk),
            done: loaded >= file.size,
          }),
        );
        const ack = await ackPromise;
        if (ack.type === "error") {
          throw new Error(ack.error || "file transfer failed");
        }
        offset = loaded;
        onProgress?.(loaded, file.size);
      }
    },
    [waitForFileTransferResponse, withOpenChannel],
  );

  const openDataChannels = useCallback((pc: RTCPeerConnection) => {
    clipboardDCRef.current = pc.createDataChannel("clipboard", {
      ordered: true,
    });
    fileDCRef.current = pc.createDataChannel("file-transfer", {
      ordered: true,
    });

    clipboardDCRef.current.onmessage = (event) => {
      try {
        const payload = JSON.parse(
          String(event.data),
        ) as ClipboardChannelMessage;
        const pending = pendingClipboardRef.current;
        if (!pending) {
          return;
        }
        if (payload.type === "error") {
          pending.reject(
            new Error(payload.error || "clipboard request failed"),
          );
        } else if (pending.mode === "get" && payload.type === "data") {
          pending.resolve(payload.text || "");
        } else if (pending.mode === "set" && payload.type === "ack") {
          pending.resolve();
        } else {
          return;
        }
        pendingClipboardRef.current = null;
      } catch {
        // Ignore malformed clipboard payloads.
      }
    };

    fileDCRef.current.onmessage = (event) => {
      try {
        const payload = JSON.parse(
          String(event.data),
        ) as FileTransferChannelMessage;
        const requestID = payload.request_id?.trim();
        if (!requestID) {
          return;
        }
        const waiter = fileTransferWaitersRef.current.get(requestID);
        if (!waiter) {
          return;
        }
        fileTransferWaitersRef.current.delete(requestID);
        if (payload.type === "error") {
          waiter.reject(new Error(payload.error || "file transfer failed"));
          return;
        }
        waiter.resolve(payload);
      } catch {
        // Ignore malformed file transfer payloads.
      }
    };
  }, []);

  const closeDataChannels = useCallback((reason: string) => {
    clipboardDCRef.current = null;
    fileDCRef.current = null;
    if (pendingClipboardRef.current) {
      pendingClipboardRef.current.reject(new Error(reason));
      pendingClipboardRef.current = null;
    }
    fileTransferWaitersRef.current.forEach(({ reject }) =>
      reject(new Error(reason)),
    );
    fileTransferWaitersRef.current.clear();
  }, []);
  const hasDataChannels = useCallback(() => Boolean(clipboardDCRef.current || fileDCRef.current), []);

  return { openDataChannels, closeDataChannels, hasDataChannels, requestClipboardText, writeClipboardText, uploadFile };
}
