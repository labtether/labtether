"use client";

import { useTranslations } from "next-intl";
import { useMemo, useRef } from "react";

import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

/**
 * A refusal the Hub answered with its own reason. Anything else that is thrown (no network, a reply that is not
 * JSON) shows the console's translated line, not browser text such as "Failed to fetch".
 */
export class HubRefusal extends Error {}

/** Reads a settings reply; a refusal throws the Hub's reason (empty when it gave none). */
export async function readHubReply<T extends { error?: string }>(response: Response): Promise<T> {
  const payload = (await response.json()) as T;
  if (!response.ok) throw new HubRefusal(typeof payload?.error === "string" ? payload.error : "");
  return payload;
}

type TestReply = { release?: string; visible_datastores?: number };

export type ConnectorWords = {
  loadFailed: (err: unknown) => string;
  saveFailed: (err: unknown, secrets?: string[]) => string;
  saveBusy: () => string;
  saved: (warning?: string) => string;
  testOk: (reply?: TestReply) => string;
  testFailed: (err: unknown, secrets?: string[]) => string;
  saveFirst: () => string;
  syncStarted: () => string;
  syncFailed: (err: unknown) => string;
};

const hubReason = (err: unknown) => (err instanceof HubRefusal ? err.message : "");

/**
 * The lines a connector's settings show after load, save, test and sync, in the person's language. The Hub's own
 * reason for a refusal is technical text ("dial tcp ...: connection refused") and is shown as it arrives.
 * The returned object keeps its identity, so it is safe in hook dependency lists.
 */
export function useConnectorWords(name: string): ConnectorWords {
  const t = useTranslations("auth.addDevice");
  const tRef = useRef(t);
  tRef.current = t;

  return useMemo<ConnectorWords>(() => {
    const say = (key: string, values: Record<string, string> = {}) => tRef.current(key, { name, ...values });
    return {
      loadFailed: (err) => sanitizeErrorMessage(hubReason(err), say("form.loadFailed")),
      saveFailed: (err, secrets = []) => sanitizeErrorMessage(hubReason(err), say("form.saveFailed"), secrets),
      saveBusy: () => say("form.saveBusy"),
      saved: (warning) => (warning ? `${say("form.saved")} ${warning}` : say("form.saved")),
      testOk: (reply) => {
        const version = reply?.release?.trim();
        const ok = version ? say("form.testOkVersion", { version }) : say("form.testOk");
        return reply?.visible_datastores === 0 ? `${ok} ${say("form.noDatastores")}` : ok;
      },
      testFailed: (err, secrets = []) => {
        const reason = sanitizeErrorMessage(hubReason(err), "", secrets);
        return reason ? say("form.testFailedReason", { reason }) : say("form.testFailed");
      },
      saveFirst: () => say("form.saveFirst"),
      syncStarted: () => say("sync.started"),
      syncFailed: (err) => {
        const reason = sanitizeErrorMessage(hubReason(err), "");
        return reason ? say("sync.startFailed", { error: reason }) : say("form.syncFailed");
      },
    };
  }, [name]);
}
