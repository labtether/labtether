// Type definitions for @novnc/novnc
declare module "@novnc/novnc/lib/rfb" {
  interface RFBOptions {
    shared?: boolean;
    credentials?: { username?: string; password?: string; target?: string };
    repeaterID?: string;
    wsProtocols?: string[];
  }

  export interface RFBDisconnectEvent {
    detail: {
      clean: boolean;
      reason?: string;
    };
  }

  export interface RFBSecurityFailureEvent {
    detail: {
      status: number;
      reason: string;
    };
  }

  export interface RFBCredentialsRequiredEvent {
    detail: {
      types: string[];
    };
  }

  export interface RFBClipboardEvent {
    detail: {
      text: string;
    };
  }

  type RFBEventMap = {
    connect: Event;
    disconnect: RFBDisconnectEvent;
    securityfailure: RFBSecurityFailureEvent;
    credentialsrequired: RFBCredentialsRequiredEvent;
    clipboard: RFBClipboardEvent;
    bell: Event;
    desktopname: Event;
    capabilities: Event;
  };

  class RFB {
    constructor(target: HTMLElement, urlOrChannel: string | WebSocket, options?: RFBOptions);

    // Properties
    viewOnly: boolean;
    focusOnClick: boolean;
    clipViewport: boolean;
    dragViewport: boolean;
    scaleViewport: boolean;
    resizeSession: boolean;
    showDotCursor: boolean;
    background: string;
    qualityLevel: number;
    compressionLevel: number;
    capabilities: { power: boolean };

    // Methods
    disconnect(): void;
    sendCredentials(credentials: { username?: string; password?: string; target?: string }): void;
    sendKey(keysym: number, code: string | null, down?: boolean): void;
    sendCtrlAltDel(): void;
    focus(): void;
    blur(): void;
    machineShutdown(): void;
    machineReboot(): void;
    machineReset(): void;
    clipboardPasteFrom(text: string): void;

    addEventListener<K extends keyof RFBEventMap>(
      type: K,
      listener: (event: RFBEventMap[K]) => void
    ): void;
    removeEventListener<K extends keyof RFBEventMap>(
      type: K,
      listener: (event: RFBEventMap[K]) => void
    ): void;
  }

  export default RFB;
}
