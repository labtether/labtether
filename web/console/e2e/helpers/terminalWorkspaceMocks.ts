import { type Page } from "@playwright/test";

export const BASE_TS = "2026-01-01T12:00:00.000Z";

export function makeTerminalAsset(id: string, name: string) {
  return {
    id,
    type: "host",
    name,
    source: "agent",
    status: "online",
    last_seen_at: BASE_TS,
  };
}

export async function installTerminalWebSocketMock(page: Page) {
  await page.addInitScript(() => {
    type WsEvent = { url: string; data: string };
    const wsEvents: WsEvent[] = [];
    const sockets: MockTerminalWebSocket[] = [];

    class MockTerminalWebSocket {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;

      readyState = MockTerminalWebSocket.CONNECTING;
      binaryType: string = "arraybuffer";
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;
      url: string;

      constructor(url: string) {
        this.url = url;
        sockets.push(this);
        setTimeout(() => {
          this.readyState = MockTerminalWebSocket.OPEN;
          this.onopen?.(new Event("open"));
        }, 0);
      }

      send(data: unknown) {
        let rendered = "";
        if (typeof data === "string") {
          rendered = data;
        } else if (data instanceof ArrayBuffer) {
          rendered = `[binary:${data.byteLength}]`;
        } else if (ArrayBuffer.isView(data)) {
          rendered = `[binary:${data.byteLength}]`;
        } else {
          rendered = String(data);
        }
        wsEvents.push({ url: this.url, data: rendered });
      }

      close(code = 1000, reason = "") {
        this.readyState = MockTerminalWebSocket.CLOSED;
        this.onclose?.({
          code,
          reason,
          wasClean: code === 1000,
        } as CloseEvent);
      }
    }

    (
      window as unknown as {
        __emitTerminalWsMessage: (data: string, index?: number) => void;
        __terminalWsEvents: WsEvent[];
        WebSocket: unknown;
      }
    ).__emitTerminalWsMessage = (data: string, index = 0) => {
      sockets[index]?.onmessage?.({ data } as MessageEvent);
    };
    (
      window as unknown as {
        __terminalWsEvents: WsEvent[];
        WebSocket: unknown;
      }
    ).__terminalWsEvents = wsEvents;
    (window as unknown as { WebSocket: unknown }).WebSocket = MockTerminalWebSocket;
  });
}

export async function installClipboardMock(page: Page, initialText: string) {
  await page.addInitScript((startingText) => {
    let clipboardText = startingText;
    const clipboard = {
      readText: async () => clipboardText,
      writeText: async (next: unknown) => {
        clipboardText = typeof next === "string" ? next : String(next ?? "");
      },
    };
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: clipboard,
    });
  }, initialText);
}

export async function installDeniedClipboardMock(page: Page) {
  await page.addInitScript(() => {
    const errorFactory = (action: string) => {
      try {
        return new DOMException(`${action} denied`, "NotAllowedError");
      } catch {
        const error = new Error(`${action} denied`);
        error.name = "NotAllowedError";
        return error;
      }
    };
    const clipboard = {
      readText: async () => {
        throw errorFactory("clipboard-read");
      },
      writeText: async () => {
        throw errorFactory("clipboard-write");
      },
    };
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: clipboard,
    });
    document.execCommand = ((command: string) => command !== "copy") as typeof document.execCommand;
  });
}

export async function emitTerminalOutput(page: Page, data: string, index = 0) {
  await page.evaluate(
    ({ payload, socketIndex }) => {
      const win = window as unknown as {
        __emitTerminalWsMessage?: (message: string, index?: number) => void;
      };
      win.__emitTerminalWsMessage?.(payload, socketIndex);
    },
    { payload: data, socketIndex: index },
  );
}

export async function triggerTerminalShortcut(
  page: Page,
  key: string,
  options?: { shift?: boolean; meta?: boolean; ctrl?: boolean },
) {
  await page.evaluate(
    ({ eventKey, shift, meta, ctrl }) => {
      window.dispatchEvent(new KeyboardEvent("keydown", {
        key: eventKey,
        shiftKey: shift,
        metaKey: meta,
        ctrlKey: ctrl,
        bubbles: true,
        cancelable: true,
      }));
    },
    {
      eventKey: key,
      shift: options?.shift ?? false,
      meta: options?.meta ?? false,
      ctrl: options?.ctrl ?? true,
    },
  );
}

export async function commandSendEvents(page: Page, command: string): Promise<Array<{ url: string; data: string }>> {
  return page.evaluate((cmd) => {
    const win = window as unknown as {
      __terminalWsEvents?: Array<{ url: string; data: string }>;
    };
    return (win.__terminalWsEvents ?? []).filter((event) => event.data === cmd);
  }, command);
}
