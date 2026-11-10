import { NextIntlClientProvider } from "next-intl";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import auth from "../../../messages/de/auth.json";
import { usePBSSettings } from "../usePBSSettings";

// Risk: a connector test line shown in English to a German reader (the Hub's own "pbs API reachable" or the
// browser's "Failed to fetch"), or the Hub's reason shown with the token secret in it.
const SECRET = "pbs-secret-value";
let latest: ReturnType<typeof usePBSSettings>;
let root: Root;
let container: HTMLDivElement;

function Probe() {
  latest = usePBSSettings();
  return null;
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

async function mountWith(testReply: () => Promise<Response>) {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => (String(url).endsWith("/test") ? testReply() : Promise.resolve(json({ configured: false })))),
  );
  await act(async () => {
    root.render(
      <NextIntlClientProvider locale="de" messages={{ auth }}>
        <Probe />
      </NextIntlClientProvider>,
    );
  });
  await act(async () => latest.setTokenSecret(SECRET));
  await act(async () => latest.testConnection());
}

beforeEach(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("connector test lines", () => {
  it("says a working connection in the reader's language, not in the Hub's words", async () => {
    await mountWith(async () =>
      json({ status: "ok", message: "pbs API reachable (3.2); token can see 0 datastores", release: "3.2", visible_datastores: 0 }),
    );
    expect(latest.error).toBe("");
    expect(latest.message).toBe("Die Verbindung zu PBS funktioniert (Version 3.2). Dieses Token sieht keine Datastores.");
  });

  it("keeps the Hub's reason for a refusal and hides the secret in it", async () => {
    await mountWith(async () => json({ error: `pbs api returned 401: token_secret=${SECRET}` }, 502));
    expect(latest.error).toBe("Der Verbindungstest für PBS ist fehlgeschlagen: pbs api returned 401: token_secret=[redacted]");
  });

  it("does not show the browser's network text", async () => {
    await mountWith(() => Promise.reject(new TypeError("Failed to fetch")));
    expect(latest.error).toBe("Der Verbindungstest für PBS ist fehlgeschlagen.");
  });
});
