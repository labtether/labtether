import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { useTerminalWhen } from "../useTerminalWhen";

// Risk: the Hub's container runs in UTC and next-intl formats in the server's zone, so a session detached at 23:45
// in Sydney read "Detached 12:45" (seen on the 2.0 lab Hub, 2026-11-10). The time must be in the viewer's zone.
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let host: HTMLDivElement;
const zoneBefore = process.env.TZ;

function Shown({ iso }: { iso: string }) {
  const when = useTerminalWhen();
  return <span>{when(iso)}</span>;
}

beforeEach(() => {
  process.env.TZ = "Pacific/Auckland";
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-06-10T02:00:00Z"));
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.useRealTimers();
  if (zoneBefore === undefined) delete process.env.TZ;
  else process.env.TZ = zoneBefore;
});

it("shows a session's time in the viewer's zone, not the server's", () => {
  act(() => {
    root.render(
      <NextIntlClientProvider locale="en" timeZone="UTC" messages={{}}>
        <Shown iso="2026-06-10T00:45:00Z" />
      </NextIntlClientProvider>,
    );
  });
  // 00:45 UTC is 12:45 in Auckland (UTC+12 in June).
  expect(host.textContent).toBe("12:45");
});
