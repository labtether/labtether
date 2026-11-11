import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import terminal from "../../../../messages/en/terminal.json";
import { useHostKeyGate, type HostKeyGate } from "../useHostKeyGate";

// Risk: trust on first use must pin exactly the key the person was shown, and only after Trust and connect; a
// changed key must never be trusted without the person removing the saved one first.
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const KEY = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB7sYdemoKeyForTestsOnly000000000000000000000";
let root: Root;
let host: HTMLDivElement;
let gate: HostKeyGate;
let calls: Array<{ action: string; body: Record<string, unknown> }>;

function Harness() {
  const keys = useHostKeyGate();
  gate = keys.gate;
  return <>{keys.dialog}</>;
}

function answer(check: Record<string, unknown>) {
  calls = [];
  vi.stubGlobal("fetch", vi.fn(async (url: string, init: RequestInit) => {
    const action = url.split("/").pop() ?? "";
    calls.push({ action, body: JSON.parse(String(init.body)) });
    const data = action === "check" ? check : { status: "trusted" };
    return new Response(JSON.stringify(data), { status: 200 });
  }));
}

const flush = () => act(async () => { await vi.advanceTimersByTimeAsync(500); });
const button = (name: string) => [...document.querySelectorAll("button")].find((b) => b.textContent === name);

beforeEach(() => {
  vi.useFakeTimers();
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  act(() => root.render(<NextIntlClientProvider locale="en" messages={{ terminal }}><Harness /></NextIntlClientProvider>));
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("pins the shown key only after Trust and connect", async () => {
  answer({ status: "unknown", host: "192.0.2.10", port: 22, host_key: KEY, fingerprint: "SHA256:demo", key_type: "ssh-ed25519" });
  let result: boolean | undefined;
  void gate({ host: "192.0.2.10", port: 22 }).then((ok) => { result = ok; });
  await flush();
  expect(document.body.textContent).toContain("SHA256:demo");
  expect(calls.map((call) => call.action)).toEqual(["check"]);
  await act(async () => { button("Trust and connect")?.click(); await vi.advanceTimersByTimeAsync(10); });
  expect(calls[1]).toEqual({ action: "trust", body: { host: "192.0.2.10", port: 22, strict_host_key: true, host_key: KEY } });
  expect(result).toBe(true);
});

it("never trusts a changed key: Cancel connects nothing", async () => {
  answer({ status: "changed", host: "192.0.2.10", port: 22, host_key: KEY, fingerprint: "SHA256:new", saved_fingerprint: "SHA256:old" });
  let result: boolean | undefined;
  void gate({ host: "192.0.2.10", port: 22 }).then((ok) => { result = ok; });
  await flush();
  expect(document.body.textContent).toContain("SHA256:old");
  expect(button("Trust and connect")).toBeUndefined();
  await act(async () => { button("Cancel")?.click(); await vi.advanceTimersByTimeAsync(10); });
  expect(result).toBe(false);
  expect(calls.map((call) => call.action)).toEqual(["check"]);
});
