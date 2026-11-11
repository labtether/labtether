"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import HostKeyDialog, { type HostKeyCheck, type HostKeyDialogPhase } from "./HostKeyDialog";

/** Where a connection goes: Quick connect's host and port, or a bookmark to a host. */
export type HostKeyTarget = { host: string; port: number; bookmarkId?: string; strict?: boolean };

/** Ask before connecting: resolves true once the host's SSH key is trusted, false when the person stops. */
export type HostKeyGate = (target: HostKeyTarget) => Promise<boolean>;

type State = { target: HostKeyTarget; phase: HostKeyDialogPhase; check?: HostKeyCheck; error?: string; busy?: boolean };

// A host that answers at once never shows the dialog; a slow one shows "Reading the host key…".
const SHOW_CHECKING_AFTER_MS = 400;

async function post(action: string, body: Record<string, unknown>): Promise<{ ok: boolean; data: HostKeyCheck & { error?: string } }> {
  const res = await fetch(`/api/terminal/host-keys/${action}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = ((await res.json().catch(() => null)) ?? {}) as HostKeyCheck & { error?: string };
  return { ok: res.ok, data };
}

function requestBody(target: HostKeyTarget): Record<string, unknown> {
  return target.bookmarkId
    ? { bookmark_id: target.bookmarkId }
    : { host: target.host, port: target.port, strict_host_key: target.strict !== false };
}

/**
 * Trust on first use for terminal SSH. The Hub reads the key the host shows (stopping before any login); a key it
 * has not saved is shown with Trust and connect, and the Hub pins exactly that key. A changed key is refused; the
 * saved one can be removed, then the new one is asked about again.
 */
export function useHostKeyGate(): { gate: HostKeyGate; dialog: ReactNode } {
  const [state, setState] = useState<State | null>(null);
  const [visible, setVisible] = useState(false);
  const resolveRef = useRef<((ok: boolean) => void) | null>(null);
  const timerRef = useRef<number | null>(null);

  const finish = useCallback((ok: boolean) => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    timerRef.current = null;
    setState(null);
    setVisible(false);
    resolveRef.current?.(ok);
    resolveRef.current = null;
  }, []);

  useEffect(() => () => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    resolveRef.current?.(false);
  }, []);

  const runCheck = useCallback(async (target: HostKeyTarget) => {
    setState({ target, phase: "checking" });
    try {
      const { ok, data } = await post("check", requestBody(target));
      if (!ok) {
        setState({ target, phase: "error", error: data.error });
        setVisible(true);
      } else if (data.status === "trusted" || data.status === "unchecked") {
        finish(true);
      } else {
        setState({ target, phase: data.status === "changed" ? "changed" : "unknown", check: data });
        setVisible(true);
      }
    } catch {
      setState({ target, phase: "error" });
      setVisible(true);
    }
  }, [finish]);

  const gate = useCallback<HostKeyGate>((target) => {
    resolveRef.current?.(false);
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve;
      setVisible(false);
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setVisible(true), SHOW_CHECKING_AFTER_MS);
      void runCheck(target);
    });
  }, [runCheck]);

  const trust = useCallback(async () => {
    if (!state?.check?.host_key) return;
    setState({ ...state, busy: true, error: undefined });
    const { ok, data } = await post("trust", { ...requestBody(state.target), host_key: state.check.host_key }).catch(() => ({ ok: false, data: {} as HostKeyCheck & { error?: string } }));
    if (ok) finish(true);
    else setState({ ...state, busy: false, error: data.error });
  }, [state, finish]);

  const removeSaved = useCallback(async () => {
    if (!state) return;
    setState({ ...state, busy: true, error: undefined });
    const { ok, data } = await post("forget", requestBody(state.target)).catch(() => ({ ok: false, data: {} as HostKeyCheck & { error?: string } }));
    if (ok) await runCheck(state.target);
    else setState({ ...state, busy: false, error: data.error });
  }, [state, runCheck]);

  const dialog = state && visible ? (
    <HostKeyDialog
      phase={state.phase}
      target={state.target}
      check={state.check}
      error={state.error}
      busy={state.busy === true}
      onTrust={() => void trust()}
      onRemove={() => void removeSaved()}
      onCancel={() => finish(false)}
    />
  ) : null;
  return { gate, dialog };
}
