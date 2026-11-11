import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import terminal from "../../../../messages/en/terminal.json";
import type { QuickConnectParams } from "../../../hooks/useSession";
import QuickConnectDialog from "../QuickConnectDialog";
import { useQuickConnectFlow } from "../useQuickConnectFlow";

// Risk: the Quick connect form steps aside for the host key check. Cancel there must bring the form back with what
// the person typed (address and password), not an empty form (reported by the owner, 2026-11-11).
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let host: HTMLDivElement;
let answer: (trusted: boolean) => void;
const connected: QuickConnectParams[] = [];

function Harness() {
  const flow = useQuickConnectFlow({
    requested: true,
    closeRequest: () => undefined,
    connect: (params) => connected.push(params),
    gate: () => new Promise<boolean>((resolve) => { answer = resolve; }),
  });
  return <QuickConnectDialog open={flow.open} draft={flow.draft} onClose={flow.onClose} onConnect={flow.onConnect} />;
}

function type(id: string, value: string) {
  const input = document.getElementById(id) as HTMLInputElement;
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

const field = (id: string) => (document.getElementById(id) as HTMLInputElement | null)?.value;
const settle = () => act(async () => { await vi.advanceTimersByTimeAsync(50); });

beforeEach(async () => {
  vi.useFakeTimers();
  connected.length = 0;
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  act(() => root.render(<NextIntlClientProvider locale="en" messages={{ terminal }}><Harness /></NextIntlClientProvider>));
  await settle();
  act(() => { type("tm-qc-addr", "demo@192.0.2.10"); type("tm-qc-pass", "pw-typed"); });
  await act(async () => { [...document.querySelectorAll("button")].find((b) => b.textContent === "Connect")?.click(); });
  await settle();
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.useRealTimers();
});

it("brings the form back with what was typed when the host key check is cancelled", async () => {
  expect(field("tm-qc-addr")).toBeUndefined();
  await act(async () => { answer(false); });
  await settle();
  expect(field("tm-qc-addr")).toBe("demo@192.0.2.10");
  expect(field("tm-qc-pass")).toBe("pw-typed");
  expect(connected).toEqual([]);
});

it("connects once the host key is trusted", async () => {
  await act(async () => { answer(true); });
  await settle();
  expect(connected).toEqual([expect.objectContaining({ host: "192.0.2.10", username: "demo", password: "pw-typed" })]);
});
