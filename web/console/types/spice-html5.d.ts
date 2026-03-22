declare module "@spice-project/spice-html5/src/main.js" {
  export interface SpiceMainConnOptions {
    uri: string;
    screen_id: string;
    password?: string;
    onerror?: (error: unknown) => void;
    onsuccess?: () => void;
    onagent?: () => void;
    message_id?: string;
    dump_id?: string;
  }

  export class SpiceMainConn {
    constructor(options: SpiceMainConnOptions);
    stop(): void;
  }

  export function sendCtrlAltDel(): void;
}
