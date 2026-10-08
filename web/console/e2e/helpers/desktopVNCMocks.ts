import type { DesktopMockEnvironment } from "./desktopBrowserEnvironment";

// Serialized into the same init script as the shared environment.
export function initDesktopVNCMocks({ settings, auditState }: DesktopMockEnvironment) {
  type DisconnectPlan = DesktopMockEnvironment["settings"]["vncDisconnects"][number];
  type CredentialPromptPlan = DesktopMockEnvironment["settings"]["vncCredentialPrompts"][number];

  class MockRFB {
    private scaleViewportValue = true;
    private resizeSessionValue = false;
    private clipViewportValue = false;
    private dragViewportValue = false;
    showDotCursor = true;
    private qualityLevelValue = 6;
    private compressionLevelValue = 6;
    private viewOnlyValue = false;
    private readonly canvas: HTMLCanvasElement;
    private disconnected = false;
    private readonly listeners = new Map<string, Array<(event: Event) => void>>();
    private readonly url: string;
    private credentialPrompt:
      | CredentialPromptPlan
      | null;
    private credentialAttempts = 0;
    private disconnectPlan: DisconnectPlan | null = null;

    constructor(
      target: HTMLDivElement,
      url: string,
      _options: { wsProtocols: string[] },
    ) {
      const canvas = document.createElement("canvas");
      canvas.tabIndex = -1;
      canvas.style.cursor = "none";
      target.appendChild(canvas);
      this.canvas = canvas;

      this.url = url;
      this.credentialPrompt = null;

      window.setTimeout(() => {
        if (this.disconnected) {
          return;
        }
        auditState.vncConnections.push(url);
        this.credentialPrompt =
          settings.vncCredentialPrompts[auditState.vncConnections.length - 1] ??
          null;
        this.disconnectPlan =
          settings.vncDisconnects[auditState.vncConnections.length - 1] ?? null;

        if (this.credentialPrompt) {
          this.emit("credentialsrequired", {
            detail: {
              types: this.credentialPrompt.types,
            },
          } as unknown as Event);
        } else if (!this.disconnectPlan?.beforeConnect) {
          this.emit("connect", new Event("connect"));
        }

        if (!this.disconnectPlan) {
          return;
        }
        window.setTimeout(() => {
          if (this.disconnected) {
            return;
          }
          this.disconnected = true;
          this.emit("disconnect", {
            detail: {
              clean: this.disconnectPlan?.clean ?? false,
              reason:
                this.disconnectPlan?.reason ??
                (this.disconnectPlan?.clean
                  ? "user disconnected"
                  : "network interrupted"),
            },
          } as unknown as Event);
        }, this.disconnectPlan.afterMs);
      }, 0);

      canvas.addEventListener("keydown", (event) => {
        auditState.inputMessages.push({
          label: "vnc-dom-keydown",
          payload: JSON.stringify({ key: event.key, code: event.code }),
        });
      });
      canvas.addEventListener("keyup", (event) => {
        auditState.inputMessages.push({
          label: "vnc-dom-keyup",
          payload: JSON.stringify({ key: event.key, code: event.code }),
        });
      });

    }

    private syncRuntimeState() {
      auditState.vncRuntime = {
        scaleViewport: this.scaleViewportValue,
        resizeSession: this.resizeSessionValue,
        clipViewport: this.clipViewportValue,
        dragViewport: this.dragViewportValue,
        qualityLevel: this.qualityLevelValue,
        compressionLevel: this.compressionLevelValue,
        viewOnly: this.viewOnlyValue,
      };
    }

    get scaleViewport() {
      return this.scaleViewportValue;
    }

    set scaleViewport(value: boolean) {
      this.scaleViewportValue = value;
      this.syncRuntimeState();
    }

    get resizeSession() {
      return this.resizeSessionValue;
    }

    set resizeSession(value: boolean) {
      this.resizeSessionValue = value;
      this.syncRuntimeState();
    }

    get clipViewport() {
      return this.clipViewportValue;
    }

    set clipViewport(value: boolean) {
      this.clipViewportValue = value;
      this.syncRuntimeState();
    }

    get dragViewport() {
      return this.dragViewportValue;
    }

    set dragViewport(value: boolean) {
      this.dragViewportValue = value;
      this.syncRuntimeState();
    }

    get qualityLevel() {
      return this.qualityLevelValue;
    }

    set qualityLevel(value: number) {
      this.qualityLevelValue = value;
      this.syncRuntimeState();
    }

    get compressionLevel() {
      return this.compressionLevelValue;
    }

    set compressionLevel(value: number) {
      this.compressionLevelValue = value;
      this.syncRuntimeState();
    }

    get viewOnly() {
      return this.viewOnlyValue;
    }

    set viewOnly(value: boolean) {
      this.viewOnlyValue = value;
      this.syncRuntimeState();
    }

    focus() {
      this.canvas.focus();
    }

    addEventListener(type: string, listener: (event: Event) => void) {
      const listeners = this.listeners.get(type) ?? [];
      listeners.push(listener);
      this.listeners.set(type, listeners);
    }

    disconnect() {
      if (this.disconnected) {
        return;
      }
      this.disconnected = true;
      this.emit("disconnect", {
        detail: {
          clean: true,
          reason: "user disconnected",
        },
      } as unknown as Event);
    }

    sendCtrlAltDel() {
      auditState.inputMessages.push({
        label: "vnc-ctrl-alt-del",
        payload: this.url,
      });
    }

    sendCredentials(creds: { username?: string; password?: string }) {
      auditState.inputMessages.push({
        label: "vnc-credentials",
        payload: JSON.stringify(creds),
      });
      if (!this.credentialPrompt || this.disconnected) {
        return;
      }
      this.credentialAttempts += 1;
      if (this.credentialPrompt.outcome === "securityfailure") {
        const reason =
          this.credentialPrompt.reason ?? "Authentication failed";
        this.emit("securityfailure", {
          detail: {
            status: 1,
            reason,
          },
        } as unknown as Event);
        this.disconnected = true;
        this.emit("disconnect", {
          detail: {
            clean: false,
            reason,
          },
        } as unknown as Event);
        return;
      }
      if (
        this.credentialPrompt.outcome === "repeatprompt" &&
        this.credentialAttempts === 1
      ) {
        this.emit("credentialsrequired", {
          detail: {
            types: this.credentialPrompt.types,
          },
        } as unknown as Event);
        return;
      }
      this.emit("connect", new Event("connect"));
    }

    clipboardPasteFrom(text: string) {
      auditState.inputMessages.push({
        label: "vnc-clipboard",
        payload: text,
      });
    }

    sendKey(keysym: number, _code?: string, down?: boolean) {
      auditState.inputMessages.push({
        label: "vnc-key",
        payload: JSON.stringify({ keysym, down: down ?? false }),
      });
    }

    private emit(type: string, event: Event) {
      const listeners = this.listeners.get(type) ?? [];
      for (const listener of listeners) {
        listener(event);
      }
    }
  }

  (window as unknown as { __labtetherTestRFBClass: unknown }).__labtetherTestRFBClass = MockRFB;
}
