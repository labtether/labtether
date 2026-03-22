/**
 * Type declarations for guacamole-common-js.
 * Covers the subset of the API used by GuacamoleViewer.
 */
declare module "guacamole-common-js" {
  export class Client {
    constructor(tunnel: WebSocketTunnel);
    connect(data?: string): void;
    disconnect(): void;
    sendKeyEvent(pressed: 0 | 1, keysym: number): void;
    sendMouseState(state: Mouse.State): void;
    getDisplay(): Display;
    createClipboardStream(mimetype: string): OutputStream;
    onclipboard: ((stream: InputStream, mimetype: string) => void) | null;
    onstatechange: ((state: number) => void) | null;
    onerror: ((status: Status) => void) | null;
  }

  export class WebSocketTunnel {
    constructor(url: string);
  }

  export class Display {
    getElement(): HTMLElement;
  }

  export class Keyboard {
    constructor(element: HTMLElement);
    reset(): void;
    onkeydown: ((keysym: number) => boolean | void) | null;
    onkeyup: ((keysym: number) => void) | null;
  }

  export class Mouse {
    constructor(element: HTMLElement);
    onmousedown: ((state: Mouse.State) => void) | null;
    onmouseup: ((state: Mouse.State) => void) | null;
    onmousemove: ((state: Mouse.State) => void) | null;
  }

  export namespace Mouse {
    interface State {
      x: number;
      y: number;
      left: boolean;
      middle: boolean;
      right: boolean;
      up: boolean;
      down: boolean;
    }
  }

  export class StringWriter {
    constructor(stream: OutputStream);
    sendText(text: string): void;
    sendEnd(): void;
  }

  export class StringReader {
    constructor(stream: InputStream);
    ontext: ((text: string) => void) | null;
    onend: (() => void) | null;
  }

  export class OutputStream {
    sendBlob(data: string): void;
    sendEnd(): void;
  }

  export class InputStream {
    onblob: ((data: string) => void) | null;
    onend: (() => void) | null;
  }

  export class Status {
    code: number;
    message: string;
  }
}
