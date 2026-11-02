"use client";

import { useEffect, useRef, useState } from "react";
import { useFormatter, useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";
import { ensureArray, ensureRecord } from "../../../lib/responseGuards";

interface PendingAgent {
  asset_id: string;
  hostname: string;
  platform: string;
  remote_ip: string;
  connected_at: string;
  device_fingerprint?: string;
  device_key_alg?: string;
  identity_verified: boolean;
  identity_verified_at?: string;
}

/** Polls the hub's agents waiting for approval every 5 s while the page is visible. */
function usePendingAgents() {
  const [pending, setPending] = useState<PendingAgent[]>([]);
  const inFlightRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    let intervalID: ReturnType<typeof setInterval> | null = null;

    const poll = async () => {
      if (inFlightRef.current || document.visibilityState === "hidden") return;
      inFlightRef.current = true;
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const res = await fetch("/api/agents/pending", { cache: "no-store", signal: controller.signal });
        if (res.ok) {
          const data = ensureRecord(await res.json().catch(() => null));
          setPending(ensureArray<PendingAgent>(data?.agents));
        }
      } catch {
        // A failed poll keeps the last list; the next poll tries again.
      } finally {
        if (abortRef.current === controller) abortRef.current = null;
        inFlightRef.current = false;
      }
    };

    const start = () => {
      void poll();
      if (intervalID === null) intervalID = setInterval(() => void poll(), 5000);
    };
    const stop = () => {
      if (intervalID !== null) {
        clearInterval(intervalID);
        intervalID = null;
      }
      abortRef.current?.abort();
      abortRef.current = null;
      inFlightRef.current = false;
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") start();
      else stop();
    };

    if (document.visibilityState === "visible") start();
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stop();
    };
  }, []);

  return { pending, setPending };
}

/**
 * Agents that connected without an enrollment token and wait for approval, as one sheet above the device list.
 * Approve stays off until the agent proves its identity; Reject turns it away.
 */
export function PendingAgents() {
  const t = useTranslations("devices.pending");
  const format = useFormatter();
  const { pending, setPending } = usePendingAgents();
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState<{ id: string; action: "approve" | "reject" } | null>(null);

  if (pending.length === 0) return null;

  const decide = async (agent: PendingAgent, action: "approve" | "reject") => {
    setBusy({ id: agent.asset_id, action });
    setActionError(null);
    try {
      const response = await fetch(`/api/agents/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ asset_id: agent.asset_id }),
      });
      if (!response.ok) throw new Error(String(response.status));
      setPending((list) => list.filter((a) => a.asset_id !== agent.asset_id));
    } catch {
      setActionError(t(action === "approve" ? "approveFailed" : "rejectFailed", { name: agent.hostname }));
    } finally {
      setBusy((current) => (current?.id === agent.asset_id ? null : current));
    }
  };

  const connectedAt = (iso: string) => {
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? "" : format.dateTime(date, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  };

  return (
    <section className="ld-wait" aria-labelledby="ld-wait-h">
      <h2 id="ld-wait-h" className="ld-wait-h">{t("title")}</h2>
      {actionError ? <p className="ld-wait-err" role="alert">{actionError}</p> : null}
      {pending.map((agent) => {
        const time = connectedAt(agent.connected_at);
        const isBusy = busy?.id === agent.asset_id;
        return (
          <div key={agent.asset_id} className="ld-wait-row">
            <div className="ld-wait-ln">
              <span className="ld-nm">{agent.hostname}</span>
              <p className="ld-wait-sub">{[agent.platform, agent.remote_ip, time ? t("connected", { time }) : ""].filter(Boolean).join(" · ")}</p>
              <p className="ld-wait-fp">{agent.device_fingerprint || t("noFingerprint")}</p>
            </div>
            <span className={`ld-wait-st${agent.identity_verified ? "" : " warn"}`}>
              {agent.identity_verified ? t("verified") : t("unverified")}
            </span>
            <div className="ld-wait-acts">
              <Button
                size="sm"
                disabled={isBusy || !agent.identity_verified}
                loading={isBusy && busy?.action === "approve"}
                onClick={() => void decide(agent, "approve")}
              >
                {t("approve")}
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={isBusy}
                loading={isBusy && busy?.action === "reject"}
                onClick={() => void decide(agent, "reject")}
              >
                {t("reject")}
              </Button>
            </div>
          </div>
        );
      })}
    </section>
  );
}
