import type { Page } from "@playwright/test";
import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
} from "./consoleApiMocks";

const BASE_TS = "2026-01-01T12:00:00.000Z";

function makeDesktopAsset(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-host-1",
    name: "Lab Host",
    type: "host",
    source: "agent",
    status: "online",
    platform: "linux",
    last_seen_at: BASE_TS,
    metadata: {
      hostname: "lab-host",
      webrtc_available: "true",
    },
    ...overrides,
  };
}

const DISPLAY_LIST = [
  {
    name: "Display 1",
    width: 2560,
    height: 1440,
    primary: true,
    offset_x: 0,
    offset_y: 0,
  },
  {
    name: "Display 2",
    width: 1920,
    height: 1080,
    primary: false,
    offset_x: 2560,
    offset_y: 0,
  },
];

export async function installDesktopApiMocks(
  page: Page,
  options: {
    sessionRequests: Array<Record<string, unknown>>;
    fileUploads?: Array<{ path: string; body: string }>;
    streamTicketStatus?: number;
    vncStreamTicketStatus?: number;
    webrtcStreamTicketStatus?: number;
    vncPassword?: string;
    assetOverrides?: Record<string, unknown>;
    connectedAgentAssetIDs?: string[];
  },
) {
  const asset = makeDesktopAsset(options.assetOverrides);
  let sessionCounter = 0;
  let lastSessionProtocol = "webrtc";
  let remoteClipboardText = "remote clipboard text";
  const fileUploads = options.fileUploads ?? [];
  const connectedAgentAssetIDs = options.connectedAgentAssetIDs ?? [asset.id];

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({ assets: [asset] }),
    liveStatusPayload: buildLiveStatusPayload({ assets: [asset] }),
    customRoute: async ({ pathname, method, requestBody, fulfillJSON, route, url }) => {
      if (pathname === "/api/agents/connected") {
        await fulfillJSON({ assets: connectedAgentAssetIDs });
        return true;
      }
      if (pathname === `/api/displays/${encodeURIComponent(asset.id)}`) {
        await fulfillJSON({ displays: DISPLAY_LIST });
        return true;
      }
      if (pathname === `/api/v1/nodes/${asset.id}/displays`) {
        await fulfillJSON({ displays: DISPLAY_LIST });
        return true;
      }
      if (pathname === `/api/v1/nodes/${asset.id}/clipboard/get` && method === "POST") {
        await fulfillJSON({ format: "text", text: remoteClipboardText });
        return true;
      }
      if (pathname === `/api/v1/nodes/${asset.id}/clipboard/set` && method === "POST") {
        remoteClipboardText =
          typeof requestBody.text === "string" ? requestBody.text : "";
        await fulfillJSON({ ok: true });
        return true;
      }
      if (pathname === `/api/files/${asset.id}/upload` && method === "POST") {
        fileUploads.push({
          path: url.searchParams.get("path") ?? "",
          body: route.request().postData() ?? "",
        });
        await fulfillJSON({ ok: true });
        return true;
      }
      if (pathname === "/api/desktop/session" && method === "POST") {
        options.sessionRequests.push({ ...requestBody });
        sessionCounter += 1;
        if (typeof requestBody?.protocol === "string") {
          lastSessionProtocol = requestBody.protocol;
        }
        await fulfillJSON({
          session: { id: `desktop-session-${sessionCounter}` },
        });
        return true;
      }
      if (pathname === "/api/desktop/stream-ticket" && method === "POST") {
        const streamTicketStatus =
          lastSessionProtocol === "vnc"
            ? options.vncStreamTicketStatus ?? options.streamTicketStatus
            : options.webrtcStreamTicketStatus ?? options.streamTicketStatus;
        if (streamTicketStatus && streamTicketStatus >= 400) {
          await fulfillJSON(
            { error: `Failed to get stream ticket (${streamTicketStatus})` },
            streamTicketStatus,
          );
          return true;
        }
        if (lastSessionProtocol === "vnc") {
          await fulfillJSON({
            wsUrl: `ws://desktop-vnc.test/session-${sessionCounter}`,
            audioWsUrl: `ws://desktop-audio.test/session-${sessionCounter}`,
            vncPassword: options.vncPassword,
            secure: false,
          });
          return true;
        }
        await fulfillJSON({
          wsUrl: `ws://desktop-webrtc.test/session-${sessionCounter}`,
          secure: false,
        });
        return true;
      }
      return false;
    },
  });
}
