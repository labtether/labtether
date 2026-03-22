declare module "@novnc/novnc/lib/rfb" {
  interface RFBOptions {
    wsProtocols?: string[];
    credentials?: { username?: string; password?: string; target?: string };
  }

  class RFB {
    constructor(target: HTMLElement, urlOrChannel: string | WebSocket, options?: RFBOptions);

    scaleViewport: boolean;
    resizeSession: boolean;
    showDotCursor: boolean;
    qualityLevel: number;
    compressionLevel: number;
    viewOnly: boolean;
    clipViewport: boolean;
    dragViewport: boolean;
    focusOnClick: boolean;

    sendCtrlAltDel(): void;
    sendCredentials(credentials: { username?: string; password?: string; target?: string }): void;
    sendKey(keysym: number, code: string | undefined, down?: boolean): void;
    clipboardPasteFrom(text: string): void;
    focus(): void;
    blur(): void;
    disconnect(): void;
    addEventListener(event: string, handler: (e: any) => void): void;
    removeEventListener(event: string, handler: (e: any) => void): void;
  }

  export default RFB;
}
