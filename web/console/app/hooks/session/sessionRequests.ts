import { buildBrowserWsUrl } from "../../lib/ws";
import { persistentSessionIDFromPayload } from "../persistentSessionPayload";
import type { DesktopProtocol, QuickConnectParams, SessionType, SpiceTicket, TerminalConnectOptions } from "./sessionTypes";

type SessionRequestOptions = {
  type: SessionType;
  connectTarget: string;
  existingSession: { id: string; target: string } | null;
  quickConnectParams?: QuickConnectParams;
  defaultActorID: string;
  quality: string;
  protocol: DesktopProtocol;
  display: string;
  record: boolean;
  directTarget: TerminalConnectOptions["directTarget"];
};

export async function createSession({
  type, connectTarget, existingSession, quickConnectParams,
  defaultActorID, quality, protocol, display, record, directTarget,
}: SessionRequestOptions): Promise<string> {
  let sessionId = "";
  if (type === "terminal") {
    // Reuse terminal session for same target
    if (
      existingSession &&
      existingSession.target === connectTarget
    ) {
      sessionId = existingSession.id;
    } else if (quickConnectParams) {
      // Quick Connect: ephemeral session with inline SSH credentials
      const sessionRes = await fetch("/api/terminal/quick-session", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(quickConnectParams),
      });
      const sessionPayload = (await sessionRes.json()) as {
        error?: string;
        session?: { id: string };
        sessionId?: string;
      };
      if (!sessionRes.ok) {
        throw new Error(
          sessionPayload.error ||
            `Failed to create quick session (${sessionRes.status})`,
        );
      }
      sessionId =
        sessionPayload.session?.id || sessionPayload.sessionId || "";
      if (!sessionId) throw new Error("No session ID returned");
    } else {
      // Use persistent sessions so tmux sessions survive disconnect/reconnect.
      // Step 1: Ensure a persistent session exists for this actor+target.
      const ensureRes = await fetch("/api/terminal/persistent-sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ target: connectTarget }),
      });
      let persistentId = "";
      if (ensureRes.ok) {
        persistentId = persistentSessionIDFromPayload(
          await ensureRes.json().catch(() => null),
        );
      }

      if (persistentId) {
        // Step 2: Attach to the persistent session (reuses tmux).
        const attachRes = await fetch(
          `/api/terminal/persistent-sessions/${encodeURIComponent(persistentId)}/attach`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({}),
          },
        );
        const attachPayload = (await attachRes.json()) as {
          error?: string;
          session?: { id?: string; persistent_session_id?: string };
        } | null;
        if (!attachRes.ok) {
          throw new Error(
            attachPayload?.error ||
              `Failed to attach session (${attachRes.status})`,
          );
        }
        sessionId = attachPayload?.session?.id || "";
        if (!sessionId) throw new Error("No session ID returned from attach");
      } else {
        // Fallback: create a plain ephemeral session if persistent flow fails.
        const sessionRes = await fetch("/api/terminal/session", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            target: connectTarget,
            actorId: defaultActorID,
            mode: "interactive",
          }),
        });
        const sessionPayload = (await sessionRes.json()) as {
          error?: string;
          session?: { id: string };
          sessionId?: string;
        };
        if (!sessionRes.ok) {
          throw new Error(
            sessionPayload.error ||
              `Failed to create session (${sessionRes.status})`,
          );
        }
        sessionId =
          sessionPayload.session?.id || sessionPayload.sessionId || "";
        if (!sessionId) throw new Error("No session ID returned");
      }
    }
  } else {
    // Desktop: always create new session
    const sessionRes = await fetch("/api/desktop/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        target: connectTarget,
        quality,
        protocol,
        display,
        record,
        direct_target: directTarget,
      }),
    });
    const sessionPayload = (await sessionRes.json()) as {
      error?: string;
      session?: { id: string };
      sessionId?: string;
    };
    if (!sessionRes.ok) {
      throw new Error(
        sessionPayload.error ||
          `Failed to create session (${sessionRes.status})`,
      );
    }
    sessionId =
      sessionPayload.session?.id || sessionPayload.sessionId || "";
    if (!sessionId) throw new Error("No session ID returned");
  }

  return sessionId;
}

export async function requestSpiceTicket(sessionId: string): Promise<SpiceTicket> {
  const spiceRes = await fetch("/api/desktop/spice-ticket", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sessionId }),
  });
  const spicePayload = (await spiceRes.json()) as {
    error?: string;
    wsUrl?: string;
    streamPath?: string;
    secure?: boolean;
    password?: string;
    type?: string;
    ca?: string;
    proxy?: string;
  };
  if (!spiceRes.ok || typeof spicePayload.password !== "string") {
    throw new Error(
      spicePayload.error ||
        `Failed to get SPICE ticket (${spiceRes.status})`,
    );
  }
  const resolvedSpiceWsUrl =
    spicePayload.wsUrl ||
    (spicePayload.streamPath
      ? buildBrowserWsUrl(spicePayload.streamPath, {
          secure: spicePayload.secure,
        })
      : undefined);
  if (!resolvedSpiceWsUrl) {
    throw new Error("SPICE ticket response missing stream endpoint");
  }
  return {
    wsUrl: resolvedSpiceWsUrl,
    password: spicePayload.password,
    type: spicePayload.type,
    ca: spicePayload.ca,
    proxy: spicePayload.proxy,
  };
}

export async function requestStreamTicket({ type, sessionId, terminalShell }: {
  type: SessionType;
  sessionId: string;
  terminalShell: string;
}) {
  const ticketEndpoint =
    type === "terminal"
      ? "/api/terminal/stream-ticket"
      : "/api/desktop/stream-ticket";
  const ticketBody: { sessionId: string; terminalShell?: string } = {
    sessionId,
  };
  if (terminalShell) {
    ticketBody.terminalShell = terminalShell;
  }
  const ticketRes = await fetch(ticketEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(ticketBody),
  });
  const ticketPayload = (await ticketRes.json()) as {
    error?: string;
    wsUrl?: string;
    streamPath?: string;
    audioWsUrl?: string;
    audioStreamPath?: string;
    secure?: boolean;
    vncPassword?: string;
  };
  if (
    !ticketRes.ok ||
    (!ticketPayload.wsUrl && !ticketPayload.streamPath)
  ) {
    throw new Error(
      ticketPayload.error ||
        `Failed to get stream ticket (${ticketRes.status})`,
    );
  }

  const builtWsUrl =
    ticketPayload.wsUrl ||
    buildBrowserWsUrl(ticketPayload.streamPath!, {
      secure: ticketPayload.secure,
    });
  const builtAudioWsUrl =
    ticketPayload.audioWsUrl ||
    (ticketPayload.audioStreamPath
      ? buildBrowserWsUrl(ticketPayload.audioStreamPath, {
          secure: ticketPayload.secure,
        })
      : null);
  return {
    wsUrl: builtWsUrl,
    audioWsUrl: builtAudioWsUrl,
    vncPassword: typeof ticketPayload.vncPassword === "string" && ticketPayload.vncPassword.length > 0
      ? ticketPayload.vncPassword
      : null,
  };
}
