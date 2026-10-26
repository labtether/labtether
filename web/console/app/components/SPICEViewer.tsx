"use client";

import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef } from "react";

export interface SPICEViewerHandle {
  disconnect: () => void;
  sendCtrlAltDel: () => void;
  focus: () => void;
}

interface SPICEViewerProps {
  wsUrl: string;
  password: string;
  onConnect?: () => void;
  onDisconnect?: (detail: { clean: boolean; reason?: string }) => void;
}

const SPICEViewer = forwardRef<SPICEViewerHandle, SPICEViewerProps>(function SPICEViewer(
  { wsUrl, password, onConnect, onDisconnect },
  ref,
) {
  const containerRef = useRef<HTMLDivElement>(null);
  const connRef = useRef<any>(null);
  const manualDisconnectRef = useRef(false);
  const disconnectedRef = useRef(false);
  const onConnectRef = useRef(onConnect);
  const onDisconnectRef = useRef(onDisconnect);
  const screenID = useMemo(() => `spice-screen-${Math.random().toString(36).slice(2, 9)}`, []);

  useEffect(() => {
    onConnectRef.current = onConnect;
    onDisconnectRef.current = onDisconnect;
  }, [onConnect, onDisconnect]);

  const emitDisconnect = useCallback((detail: { clean: boolean; reason?: string }) => {
    if (disconnectedRef.current) {
      return;
    }
    disconnectedRef.current = true;
    onDisconnectRef.current?.(detail);
  }, []);

  useImperativeHandle(ref, () => ({
    disconnect: () => {
      manualDisconnectRef.current = true;
      try {
        connRef.current?.stop?.();
      } catch {
        // ignore
      }
      emitDisconnect({ clean: true, reason: "user disconnected" });
    },
    sendCtrlAltDel: () => {
      const canvas = containerRef.current?.querySelector("canvas");
      if (!connRef.current || !canvas) return;
      // The library's helper depends on a global `sc`. Use this session's
      // keyboard canvas so multiple viewers stay isolated.
      for (const [type, keyCode, ctrlKey, altKey] of [
        ["keydown", 17, true, false],
        ["keydown", 18, true, true],
        ["keydown", 46, true, true],
        ["keyup", 46, true, true],
        ["keyup", 18, true, false],
        ["keyup", 17, false, false],
      ] as const) {
        canvas.dispatchEvent(new KeyboardEvent(type, { keyCode, ctrlKey, altKey, cancelable: true }));
      }
    },
    focus: () => {
      (containerRef.current?.querySelector("canvas") ?? containerRef.current)?.focus();
    },
  }), [emitDisconnect]);

  useEffect(() => {
    manualDisconnectRef.current = false;
    disconnectedRef.current = false;
    let cancelled = false;
    let connection: any = null;

    void import("@spice-project/spice-html5/src/main.js").then((spiceModule) => {
      if (cancelled) {
        return;
      }
      try {
        connection = new spiceModule.SpiceMainConn({
          uri: wsUrl,
          screen_id: screenID,
          password,
          onerror: (err: unknown) => {
            if (cancelled) return;
            emitDisconnect({ clean: false, reason: err instanceof Error ? err.message : "SPICE connection error" });
          },
          onsuccess: () => {
            if (cancelled) return;
            onConnectRef.current?.();
          },
        });
        connRef.current = connection;
      } catch (error) {
        emitDisconnect({ clean: false, reason: error instanceof Error ? error.message : "SPICE initialization failed" });
      }
    }).catch((error: unknown) => {
      if (cancelled) return;
      emitDisconnect({ clean: false, reason: error instanceof Error ? error.message : "SPICE client failed to load" });
    });

    return () => {
      cancelled = true;
      if (connection) {
        try {
          connection?.stop?.();
        } catch {
          // ignore
        }
      }
      connRef.current = null;
    };
  }, [emitDisconnect, password, screenID, wsUrl]);

  return (
    <div
      ref={containerRef}
      id={screenID}
      className="vncContainer vncConnected"
      tabIndex={0}
    />
  );
});

export default SPICEViewer;
