import type { DesktopMockEnvironment } from "./desktopBrowserEnvironment";

// Serialized into the same init script as the shared environment.
export function initDesktopMediaMocks({ auditState }: DesktopMockEnvironment) {
  HTMLMediaElement.prototype.play = function mockPlay() {
    return Promise.resolve();
  };
  HTMLMediaElement.prototype.pause = function mockPause() {
    return undefined;
  };

  class MockSourceBuffer {
    updating = false;
    mode = "sequence";
    private updateEndListeners: Array<(event: Event) => void> = [];

    addEventListener(type: string, listener: (event: Event) => void) {
      if (type === "updateend") {
        this.updateEndListeners.push(listener);
      }
    }

    appendBuffer(_buffer: ArrayBuffer) {
      this.updating = true;
      window.setTimeout(() => {
        this.updating = false;
        for (const listener of this.updateEndListeners) {
          listener(new Event("updateend"));
        }
      }, 0);
    }
  }

  class MockMediaSource {
    static isTypeSupported() {
      return true;
    }

    readyState: "closed" | "open" | "ended" = "closed";
    private sourceOpenListeners: Array<(event: Event) => void> = [];

    constructor() {
      window.setTimeout(() => {
        this.readyState = "open";
        for (const listener of this.sourceOpenListeners) {
          listener(new Event("sourceopen"));
        }
      }, 0);
    }

    addEventListener(type: string, listener: (event: Event) => void) {
      if (type === "sourceopen") {
        this.sourceOpenListeners.push(listener);
      }
    }

    removeEventListener(type: string, listener: (event: Event) => void) {
      if (type !== "sourceopen") {
        return;
      }
      this.sourceOpenListeners = this.sourceOpenListeners.filter(
        (candidate) => candidate !== listener,
      );
    }

    addSourceBuffer() {
      return new MockSourceBuffer() as unknown as SourceBuffer;
    }

    endOfStream() {
      this.readyState = "ended";
    }
  }

  Object.defineProperty(window, "MediaSource", {
    configurable: true,
    writable: true,
    value: MockMediaSource,
  });

  URL.createObjectURL = () => "blob:desktop-audio";
  URL.revokeObjectURL = () => undefined;

  class MockMediaRecorder {
    static isTypeSupported() {
      return true;
    }

    mimeType: string;
    ondataavailable: ((event: BlobEvent) => void) | null = null;
    onerror: ((event: Event) => void) | null = null;
    onstop: (() => void) | null = null;

    constructor(_stream: MediaStream, options?: { mimeType?: string }) {
      this.mimeType = options?.mimeType ?? "video/webm";
    }

    start() {
      auditState.mediaRecorderStarts += 1;
    }

    stop() {
      auditState.mediaRecorderStops += 1;
      this.ondataavailable?.({
        data: new Blob(["desktop-recording"], { type: this.mimeType }),
      } as BlobEvent);
      this.onstop?.();
    }
  }

  (window as unknown as { MediaRecorder: unknown }).MediaRecorder = MockMediaRecorder;
}
