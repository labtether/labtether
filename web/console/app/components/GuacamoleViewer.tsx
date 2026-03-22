"use client";

import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef } from "react";
import type { Client, Keyboard, Mouse } from "guacamole-common-js";

type GuacModule = typeof import("guacamole-common-js");

export interface GuacamoleViewerHandle {
  disconnect: () => void;
  sendCtrlAltDel: () => void;
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

  useImperativeHandle(ref, () => ({
    disconnect: () => {
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
    focus: () => {
      containerRef.current?.focus();
    },
  }), []);

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
          client.sendKeyEvent(1, keysym);
          return false;
        };
        keyboard.onkeyup = (keysym: number) => {
          const client = clientRef.current;
          if (!client) {
            return;
          }
          client.sendKeyEvent(0, keysym);
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
  }, []);

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
        if (state?.left || state?.middle || state?.right) {
          container.focus();
        }
        client?.sendMouseState(state);
      };
      mouse.onmousedown = sendMouseState;
      mouse.onmouseup = sendMouseState;
      mouse.onmousemove = sendMouseState;

      pasteHandler = (event: ClipboardEvent) => {
        if (!isContainerFocused()) {
          return;
        }
        const text = event.clipboardData?.getData("text");
        if (!text) {
          return;
        }
        event.preventDefault();
        try {
          const stream = client!.createClipboardStream("text/plain");
          const writer = new Guacamole.StringWriter(stream);
          writer.sendText(text);
          writer.sendEnd();
        } catch {
          // ignore
        }
      };
      container.addEventListener("paste", pasteHandler);

      client.onclipboard = (stream: InstanceType<GuacModule["InputStream"]>, mimetype: string) => {
        if (mimetype !== "text/plain") {
          return;
        }
        try {
          const reader = new Guacamole.StringReader(stream);
          let text = "";
          reader.ontext = (chunk: string) => {
            text += chunk;
          };
          reader.onend = () => {
            if (!text || !isContainerFocused()) {
              return;
            }
            if (navigator.clipboard?.writeText) {
              navigator.clipboard.writeText(text).catch(() => {
                // ignore clipboard permission failures
              });
            }
          };
        } catch {
          // ignore
        }
      };

      client.onstatechange = (state: number) => {
        // 3 = CONNECTED, 5 = DISCONNECTED
        if (state === 3) onConnectRef.current?.();
        if (state === 5) {
          const clean = manualDisconnectRef.current;
          manualDisconnectRef.current = false;
          emitDisconnect({ clean, reason: clean ? "user disconnected" : "session ended" });
        }
      };

      client.onerror = () => {
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
          emitDisconnect({ clean: false, reason: "failed to load guacamole viewer" });
        });
    }

    return () => {
      stopped = true;
      keyboardRef.current?.reset?.();
      if (pasteHandler) {
        container.removeEventListener("paste", pasteHandler);
        pasteHandler = null;
      }
      if (mouse) {
        mouse.onmousedown = null;
        mouse.onmouseup = null;
        mouse.onmousemove = null;
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
        container.removeChild(displayEl);
      }
      clientRef.current = null;
      tunnel = null;
      mouse = null;
    };
  }, [emitDisconnect, wsUrl]);

  return (
    <div
      ref={containerRef}
      className="vncContainer vncConnected"
      tabIndex={0}
      onMouseDown={() => containerRef.current?.focus()}
    />
  );
});

export default GuacamoleViewer;
