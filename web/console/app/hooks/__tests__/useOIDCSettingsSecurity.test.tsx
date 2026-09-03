import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useOIDCSettings } from "../useOIDCSettings";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
let latest: ReturnType<typeof useOIDCSettings> | null;
let putBodies: Record<string, unknown>[];
let configured: boolean;

function Harness() {
  latest = useOIDCSettings();
  return null;
}

function response(body: Record<string, unknown>, ok = true) {
  return {
    ok,
    status: ok ? 200 : 500,
    json: async () => body,
  } as Response;
}

beforeEach(() => {
  latest = null;
  putBodies = [];
  configured = true;
  vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "PUT") {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>;
      putBodies.push(body);
      if (body.clear_client_secret === true) configured = false;
      if (typeof body.client_secret === "string" && body.client_secret) configured = true;
      return response({ client_secret_configured: configured });
    }
    return response({
      enabled: false,
      client_secret_configured: configured,
      sources: { client_secret: "db" },
    });
  }));
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function mountHook() {
  await act(async () => {
    root.render(<Harness />);
    await Promise.resolve();
  });
  if (!latest) throw new Error("hook did not render");
}

describe("useOIDCSettings secret handling", () => {
  it("keeps saved secrets out of form state and only clears them explicitly", async () => {
    await mountHook();
    expect(latest?.clientSecret).toBe("");
    expect(latest?.clientSecretConfigured).toBe(true);

    await act(async () => {
      await latest?.save();
    });
    expect(putBodies[0]).not.toHaveProperty("client_secret");
    expect(putBodies[0]).not.toHaveProperty("clear_client_secret");

    await act(async () => {
      latest?.setClientSecret("replacement-secret");
    });
    await act(async () => {
      await latest?.save();
    });
    expect(putBodies[1]).toMatchObject({ client_secret: "replacement-secret" });
    expect(putBodies[1]).not.toHaveProperty("clear_client_secret");

    await act(async () => {
      latest?.removeClientSecret();
    });
    await act(async () => {
      await latest?.save();
    });
    expect(putBodies[2]).toMatchObject({ clear_client_secret: true });
    expect(putBodies[2]).not.toHaveProperty("client_secret");
  });
});
