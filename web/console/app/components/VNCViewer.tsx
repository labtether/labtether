"use client";

import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type RFB from "@novnc/novnc/lib/rfb";
import type {
  RFBDisconnectEvent,
  RFBSecurityFailureEvent,
  RFBCredentialsRequiredEvent,
  RFBClipboardEvent,
} from "@novnc/novnc/lib/rfb";

export type VNCViewerHandle = {
  disconnect: () => void;
  sendCtrlAltDel: () => void;
  sendCredentials: (creds: { username?: string; password?: string }) => void;
  clipboardPasteFrom: (text: string) => void;
  focus: () => void;
  requestPointerLock: () => void;
  exitPointerLock: () => void;
  sendKey: (keysym: number, down: boolean) => void;
};

export type VNCCredentialRequest = {
  types: string[]; // e.g. ["username", "password"] for ARD, ["password"] for VNC auth
};

type VNCViewerProps = {
  wsUrl: string | null;
  onConnect?: () => void;
  onDisconnect?: (detail: { clean: boolean; reason?: string }) => void;
  onError?: (message: string) => void;
  onCredentialsRequired?: (request: VNCCredentialRequest) => void;
  quality?: string;
  scalingMode?: "fit" | "native" | "fill";
  viewOnly?: boolean;
};

type TestWindow = Window & {
  __labtetherTestRFBClass?: new (
    target: HTMLDivElement,
    url: string,
    options: { wsProtocols: string[] },
  ) => RFB;
};

const VNCViewer = forwardRef<VNCViewerHandle, VNCViewerProps>(function VNCViewer(
  { wsUrl, onConnect, onDisconnect, onError, onCredentialsRequired, quality, scalingMode, viewOnly },
  ref
) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);
  const rfbDisconnectedRef = useRef(true);
  const pasteHandlerRef = useRef<((e: ClipboardEvent) => void) | null>(null);
  const [connected, setConnected] = useState(false);

  const getInputTarget = () => {
    const container = containerRef.current;
    if (!container) {
      return null;
    }
    const canvas = container.querySelector("canvas");
    if (canvas instanceof HTMLCanvasElement) {
      return canvas;
    }
    return container;
  };

  useEffect(() => {
    const container = containerRef.current;
    if (!connected || !container) {
      if (container) {
        delete container.dataset.cursorFallback;
      }
      return;
    }

    const inputTarget = getInputTarget();
    if (!(inputTarget instanceof HTMLCanvasElement)) {
      delete container.dataset.cursorFallback;
      return;
    }

    const updateCursorFallback = () => {
      const cursor = inputTarget.style.cursor.trim();
      const pointerLocked = document.pointerLockElement === inputTarget;
      const needsFallback = !pointerLocked && (cursor === "" || cursor === "none");
      if (needsFallback) {
        container.dataset.cursorFallback = "visible";
      } else {
        delete container.dataset.cursorFallback;
      }
    };

    updateCursorFallback();

    const observer = new MutationObserver(() => {
      updateCursorFallback();
    });
    observer.observe(inputTarget, {
      attributes: true,
      attributeFilter: ["style"],
    });
    document.addEventListener("pointerlockchange", updateCursorFallback);

    return () => {
      observer.disconnect();
      document.removeEventListener("pointerlockchange", updateCursorFallback);
      delete container.dataset.cursorFallback;
    };
  }, [connected]);

  // Stable refs for callbacks — avoids tearing down VNC on parent re-render.
  const onConnectRef = useRef(onConnect);
  const onDisconnectRef = useRef(onDisconnect);
  const onErrorRef = useRef(onError);
  const onCredentialsRequiredRef = useRef(onCredentialsRequired);
  const qualityRef = useRef(quality);
  const scalingModeRef = useRef(scalingMode);
  onConnectRef.current = onConnect;
  onDisconnectRef.current = onDisconnect;
  onErrorRef.current = onError;
  onCredentialsRequiredRef.current = onCredentialsRequired;
  qualityRef.current = quality;
  scalingModeRef.current = scalingMode;

  const disconnectRfb = (target: RFB | null) => {
    if (!target || rfbDisconnectedRef.current) {
      return;
    }
    rfbDisconnectedRef.current = true;
    try {
      target.disconnect();
    } catch {
      // ignore cleanup errors
    }
  };

  useImperativeHandle(ref, () => ({
    disconnect: () => {
      if (rfbRef.current) {
        disconnectRfb(rfbRef.current);
        rfbRef.current = null;
        setConnected(false);
      }
    },
    sendCtrlAltDel: () => {
      if (rfbRef.current) {
        rfbRef.current.sendCtrlAltDel();
      }
    },
    sendCredentials: (creds: { username?: string; password?: string }) => {
      if (rfbRef.current) {
        rfbRef.current.sendCredentials(creds);
      }
    },
    clipboardPasteFrom: (text: string) => {
      if (rfbRef.current) {
        rfbRef.current.clipboardPasteFrom(text);
      }
    },
    focus: () => {
      if (rfbRef.current && "focus" in rfbRef.current) {
        (
          rfbRef.current as unknown as {
            focus: (options?: FocusOptions) => void;
          }
        ).focus({ preventScroll: true });
        return;
      }
      getInputTarget()?.focus();
    },
    requestPointerLock: () => {
      getInputTarget()?.requestPointerLock();
    },
    exitPointerLock: () => {
      const inputTarget = getInputTarget();
      if (
        document.pointerLockElement === inputTarget ||
        document.pointerLockElement === containerRef.current
      ) {
        document.exitPointerLock();
      }
    },
    sendKey: (keysym: number, down: boolean) => {
      if (rfbRef.current) {
        rfbRef.current.sendKey(keysym, undefined, down);
      }
    },
  }));

  useEffect(() => {
    const container = containerRef.current;
    if (!wsUrl || !container) return;

    let rfb: RFB | null = null;
    let cancelled = false;

    const init = async () => {
      try {
        // Allow browser tests to inject a lightweight RFB shim without
        // patching the app bundle or shipping a production-only code path.
        const testRFBClass = (window as TestWindow).__labtetherTestRFBClass;
        const RFBClass =
          testRFBClass ??
          (await import("@novnc/novnc/lib/rfb")).default;
        if (cancelled) {
          return;
        }

        rfb = new RFBClass(container, wsUrl, {
          wsProtocols: ["binary"],
        });
        rfbDisconnectedRef.current = false;

        const mode = scalingModeRef.current ?? "fit";
        rfb.scaleViewport = mode !== "native";
        rfb.resizeSession = mode === "fill";
        rfb.clipViewport = mode === "native";
        rfb.dragViewport = false;
        rfb.showDotCursor = true;

        // Quality settings
        const q = qualityRef.current;
        if (q === "low") {
          rfb.qualityLevel = 3;
          rfb.compressionLevel = 9;
        } else if (q === "high") {
          rfb.qualityLevel = 9;
          rfb.compressionLevel = 2;
        } else {
          rfb.qualityLevel = 6;
          rfb.compressionLevel = 6;
        }

        rfb.addEventListener("connect", () => {
          setConnected(true);
          onConnectRef.current?.();
        });

        rfb.addEventListener("disconnect", (e: RFBDisconnectEvent) => {
          rfbDisconnectedRef.current = true;
          setConnected(false);
          onDisconnectRef.current?.({ clean: e.detail.clean, reason: e.detail.reason });
        });

        rfb.addEventListener("securityfailure", (e: RFBSecurityFailureEvent) => {
          onErrorRef.current?.(`VNC security failure: ${e.detail.status} - ${e.detail.reason}`);
        });

        rfb.addEventListener("credentialsrequired", (e: RFBCredentialsRequiredEvent) => {
          const types: string[] = e.detail?.types ?? ["password"];
          onCredentialsRequiredRef.current?.({ types });
        });

        // Clipboard: remote → local
        rfb.addEventListener("clipboard", (e: RFBClipboardEvent) => {
          const text = e.detail?.text;
          if (text && navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(text).catch(() => {
              // Browser denied clipboard access — ignore silently.
            });
          }
        });

        // Clipboard: local → remote (intercept paste on container)
        const handlePaste = (e: ClipboardEvent) => {
          const text = e.clipboardData?.getData("text");
          if (text && rfbRef.current) {
            e.preventDefault();
            rfbRef.current.clipboardPasteFrom(text);
          }
        };
        pasteHandlerRef.current = handlePaste;
        container.addEventListener("paste", handlePaste);

        rfbRef.current = rfb;
        if (cancelled) {
          disconnectRfb(rfb);
          rfb = null;
          rfbRef.current = null;
        }
      } catch (err: unknown) {
        if (cancelled) {
          return;
        }
        const message = err instanceof Error ? err.message : "unknown error";
        onErrorRef.current?.(`Failed to initialize VNC: ${message}`);
      }
    };

    init();

    return () => {
      cancelled = true;
      if (pasteHandlerRef.current) {
        container.removeEventListener("paste", pasteHandlerRef.current);
        pasteHandlerRef.current = null;
      }
      if (rfb) {
        disconnectRfb(rfb);
        rfb = null;
        rfbRef.current = null;
      }
      setConnected(false);
    };
  }, [wsUrl]);

  useEffect(() => {
    if (!rfbRef.current) return;
    const rfb = rfbRef.current;
    const mode = scalingMode ?? "fit";
    rfb.scaleViewport = mode !== "native";
    rfb.resizeSession = mode === "fill";
    rfb.clipViewport = mode === "native";
    rfb.dragViewport = false;
  }, [scalingMode]);

  useEffect(() => {
    if (rfbRef.current) {
      rfbRef.current.viewOnly = viewOnly ?? false;
    }
  }, [viewOnly]);

  useEffect(() => {
    if (!rfbRef.current) return;
    const rfb = rfbRef.current;
    if (quality === "low") {
      rfb.qualityLevel = 3;
      rfb.compressionLevel = 9;
    } else if (quality === "high") {
      rfb.qualityLevel = 9;
      rfb.compressionLevel = 2;
    } else {
      rfb.qualityLevel = 6;
      rfb.compressionLevel = 6;
    }
  }, [quality]);

  return (
    <div
      ref={containerRef}
      className={`vncContainer${connected ? " vncConnected" : ""}${(scalingMode ?? "fit") === "native" ? " vncNative" : ""}`}
      tabIndex={-1}
      onMouseDownCapture={(event) => {
        if (event.target instanceof HTMLCanvasElement) {
          return;
        }
        const inputTarget = getInputTarget();
        if (inputTarget instanceof HTMLCanvasElement) {
          event.preventDefault();
          inputTarget.focus();
        }
      }}
      onMouseDown={() => {
        const inputTarget = getInputTarget();
        if (inputTarget instanceof HTMLCanvasElement) {
          inputTarget.focus();
        } else {
          containerRef.current?.focus();
        }
      }}
      onClick={() => {
        const inputTarget = getInputTarget();
        if (inputTarget instanceof HTMLCanvasElement) {
          inputTarget.focus();
        } else {
          containerRef.current?.focus();
        }
      }}
      onContextMenu={(e) => e.preventDefault()}
    />
  );
});

export default VNCViewer;
