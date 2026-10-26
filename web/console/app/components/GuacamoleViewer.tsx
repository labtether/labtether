"use client";

import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef } from "react";
import type { Client, Keyboard, Mouse } from "guacamole-common-js";
import { readGuacamoleClipboard, writeGuacamoleClipboard } from "./guacamole/clipboard";

type GuacModule = typeof import("guacamole-common-js");

export interface GuacamoleViewerHandle {
  disconnect: () => void;
  sendCtrlAltDel: () => void;
  sendKey: (keysym: number, down: boolean) => void;
  focus: () => void;
}

interface GuacamoleViewerProps {
  wsUrl: string | null;
  onConnect?: () => void;
  onDisconnect?: (detail: { clean: boolean; reason?: string }) => void;
}

const GuacamoleViewer = forwardRef<GuacamoleViewerHandle, GuacamoleViewerProps>(function GuacamoleViewer(
  { wsUrl, onConnect, onDisconnect },
  ref,
) {
  const containerRef = useRef<HTMLDivElement>(null);
  const clientRef = useRef<Client | null>(null);
  const keyboardRef = useRef<Keyboard | null>(null);
  const guacModuleRef = useRef<GuacModule | null>(null);
  const releasePointerRef = useRef<(() => void) | null>(null);
  const heldKeysRef = useRef(new Set<number>());
  const manualDisconnectRef = useRef(false);
  const disconnectedRef = useRef(false);
  const onConnectRef = useRef(onConnect);
  const onDisconnectRef = useRef(onDisconnect);

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

  const sendKey = useCallback((keysym: number, down: boolean) => {
    if (!clientRef.current) return;
    if (down) heldKeysRef.current.add(keysym);
    else heldKeysRef.current.delete(keysym);
    clientRef.current.sendKeyEvent(down ? 1 : 0, keysym);
  }, []);

  const releaseInput = useCallback(() => {
    keyboardRef.current?.reset();
    for (const keysym of heldKeysRef.current) sendKey(keysym, false);
    heldKeysRef.current.clear();
    releasePointerRef.current?.();
  }, [sendKey]);

  useEffect(() => {
    const onVisibility = () => { if (document.visibilityState !== "visible") releaseInput(); };
    window.addEventListener("blur", releaseInput);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("blur", releaseInput);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [releaseInput]);

  useImperativeHandle(ref, () => ({
    disconnect: () => {
      releaseInput();
      manualDisconnectRef.current = true;
      if (clientRef.current) {
        clientRef.current.disconnect();
      }
    },
    sendCtrlAltDel: () => {
      const client = clientRef.current;
      if (!client) return;
      client.sendKeyEvent(1, 0xffe3); // Ctrl
      client.sendKeyEvent(1, 0xffe9); // Alt
      client.sendKeyEvent(1, 0xffff); // Delete
      client.sendKeyEvent(0, 0xffff);
      client.sendKeyEvent(0, 0xffe9);
      client.sendKeyEvent(0, 0xffe3);
    },
    sendKey,
    focus: () => {
      containerRef.current?.focus();
    },
  }), [releaseInput, sendKey]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || keyboardRef.current) {
      return;
    }

    let cancelled = false;
    void import("guacamole-common-js")
      .then((module) => {
        if (cancelled) {
          return;
        }
        const Guacamole = ((module as { default?: GuacModule }).default ?? module) as GuacModule;
        guacModuleRef.current = Guacamole;
        const keyboard = new Guacamole.Keyboard(container);
        keyboard.onkeydown = (keysym: number) => {
          const client = clientRef.current;
          if (!client) {
            return true;
          }
          sendKey(keysym, true);
          return false;
        };
        keyboard.onkeyup = (keysym: number) => {
          const client = clientRef.current;
          if (!client) {
            return;
          }
          sendKey(keysym, false);
        };
        keyboardRef.current = keyboard;
      })
      .catch(() => {
        // Keyboard setup failure should not block rendering.
      });

    return () => {
      cancelled = true;
      if (keyboardRef.current) {
        try {
          keyboardRef.current.reset?.();
        } catch {
          // ignore
        }
        keyboardRef.current.onkeydown = null;
        keyboardRef.current.onkeyup = null;
        keyboardRef.current = null;
      }
    };
  }, [sendKey]);

  useEffect(() => {
    const container = containerRef.current;
    if (!wsUrl || !container) {
      return;
    }

    manualDisconnectRef.current = false;
    disconnectedRef.current = false;
    let stopped = false;
    let tunnel: InstanceType<GuacModule["WebSocketTunnel"]> | null = null;
    let client: Client | null = null;
    let displayEl: HTMLElement | null = null;
    let mouse: Mouse | null = null;
    let touchscreen: Mouse.Touchscreen | null = null;
    let pointerEnabled = true;
    let releasingPointer = false;
    const beginPointerInput = () => { pointerEnabled = true; container.focus(); };
    const clipboardReaders = new Set<() => void>();
    let pasteHandler: ((event: ClipboardEvent) => void) | null = null;

    const isContainerFocused = () => {
      const active = document.activeElement;
      if (!active) {
        return false;
      }
      return active === container || container.contains(active);
    };

    const initClient = (Guacamole: GuacModule) => {
      if (stopped) {
        return;
      }

      tunnel = new Guacamole.WebSocketTunnel(wsUrl);
      client = new Guacamole.Client(tunnel);
      clientRef.current = client;

      const nextDisplay = client.getDisplay().getElement() as HTMLElement | null;
      if (!nextDisplay) {
        emitDisconnect({ clean: false, reason: "failed to initialize guacamole display" });
        return;
      }
      displayEl = nextDisplay;
      container.appendChild(displayEl);

      mouse = new Guacamole.Mouse(displayEl);
      const sendMouseState = (state: Mouse.State) => {
        if (stopped || !pointerEnabled || clientRef.current !== client) return;
        if (!releasingPointer && (state?.left || state?.middle || state?.right)) {
          container.focus();
        }
        client?.sendMouseState(state, true);
      };
      mouse.onmousedown = sendMouseState;
      mouse.onmouseup = sendMouseState;
      mouse.onmousemove = sendMouseState;
      touchscreen = new Guacamole.Mouse.Touchscreen(displayEl);
      touchscreen.onmousedown = sendMouseState;
      touchscreen.onmouseup = sendMouseState;
      touchscreen.onmousemove = sendMouseState;
      displayEl.addEventListener("mousedown", beginPointerInput, true);
      displayEl.addEventListener("touchstart", beginPointerInput, true);
      displayEl.addEventListener("touchcancel", releaseInput, true);
      releasePointerRef.current = () => {
        releasingPointer = true;
        // Guacamole 1.5 has no touchcancel handler and reset() leaves its private
        // gesture timers alive. An empty touchend takes its end_gesture() path.
        const cancelledTouch = new Event("touchend");
        Object.defineProperties(cancelledTouch, {
          touches: { value: [] }, changedTouches: { value: [] },
        });
        displayEl?.dispatchEvent(cancelledTouch);
        mouse?.reset();
        touchscreen?.reset();
        releasingPointer = false;
        pointerEnabled = false;
      };

      pasteHandler = (event: ClipboardEvent) => {
        if (stopped || clientRef.current !== client || !isContainerFocused()) {
          return;
        }
        const text = event.clipboardData?.getData("text");
        if (text === undefined || !event.clipboardData?.types.includes("text/plain")) return;
        event.preventDefault();
        try {
          const stream = client!.createClipboardStream("text/plain");
          writeGuacamoleClipboard(stream, text);
        } catch {
          // ignore
        }
      };
      container.addEventListener("paste", pasteHandler);

      client.onclipboard = (stream: InstanceType<GuacModule["InputStream"]>, mimetype: string) => {
        if (stopped || !/^text\/plain(?:;|$)/i.test(mimetype)) {
          stream.sendAck("Unsupported clipboard type", 0x0100);
          return;
        }
        try {
          const stopReading = readGuacamoleClipboard(stream, (text) => {
            clipboardReaders.delete(stopReading);
            if (stopped || clientRef.current !== client || !isContainerFocused()) return;
            navigator.clipboard?.writeText(text).catch(() => {
              // Clipboard permission still belongs to the browser.
            });
          });
          clipboardReaders.add(stopReading);
        } catch {
          // ignore
        }
      };

      client.onstatechange = (state: number) => {
        if (stopped || clientRef.current !== client) return;
        // 3 = CONNECTED, 5 = DISCONNECTED
        if (state === 3) onConnectRef.current?.();
        if (state === 5) {
          const clean = manualDisconnectRef.current;
          manualDisconnectRef.current = false;
          emitDisconnect({ clean, reason: clean ? "user disconnected" : "session ended" });
        }
      };

      client.onerror = () => {
        if (stopped || clientRef.current !== client) return;
        emitDisconnect({ clean: false, reason: "guacamole error" });
      };

      client.connect();
    };

    if (guacModuleRef.current) {
      initClient(guacModuleRef.current);
    } else {
      void import("guacamole-common-js")
        .then((module) => {
          if (stopped) {
            return;
          }
          const Guacamole = ((module as { default?: GuacModule }).default ?? module) as GuacModule;
          guacModuleRef.current = Guacamole;
          initClient(Guacamole);
        })
        .catch(() => {
          if (stopped) return;
          emitDisconnect({ clean: false, reason: "failed to load guacamole viewer" });
        });
    }

    return () => {
      releaseInput();
      stopped = true;
      releasePointerRef.current = null;
      clipboardReaders.forEach((stopReading) => stopReading());
      clipboardReaders.clear();
      if (pasteHandler) {
        container.removeEventListener("paste", pasteHandler);
        pasteHandler = null;
      }
      if (mouse) {
        mouse.onmousedown = null;
        mouse.onmouseup = null;
        mouse.onmousemove = null;
      }
      if (touchscreen) {
        touchscreen.onmousedown = null;
        touchscreen.onmouseup = null;
        touchscreen.onmousemove = null;
      }
      if (client) {
        client.onclipboard = null;
        client.onstatechange = null;
        client.onerror = null;
      }
      try {
        client?.disconnect();
      } catch {
        // ignore
      }
      if (displayEl && displayEl.parentNode === container) {
        displayEl.removeEventListener("mousedown", beginPointerInput, true);
        displayEl.removeEventListener("touchstart", beginPointerInput, true);
        displayEl.removeEventListener("touchcancel", releaseInput, true);
        container.removeChild(displayEl);
      }
      clientRef.current = null;
      tunnel = null;
      mouse = null;
    };
  }, [emitDisconnect, releaseInput, wsUrl]);

  return (
    <div
      ref={containerRef}
      className="vncContainer vncConnected"
      tabIndex={0}
      onMouseDown={() => containerRef.current?.focus()}
      onBlur={releaseInput}
    />
  );
});

export default GuacamoleViewer;
