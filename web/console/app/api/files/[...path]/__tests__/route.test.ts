// @vitest-environment node

import { createHash } from "node:crypto";
import { describe, expect, it } from "vitest";

import { authorizeFileProxyRequest, proxyToHub } from "../route";

function request(method: string, headers: Record<string, string> = {}): Request {
  return new Request("https://hub.example/api/files/asset-1/upload", {
    method,
    headers,
  });
}

describe("file proxy route-local authorization", () => {
  it("rejects unauthenticated downloads", async () => {
    const response = authorizeFileProxyRequest(request("GET"));
    expect(response?.status).toBe(401);
  });

  it("rejects cross-origin uploads with a session", async () => {
    const response = authorizeFileProxyRequest(request("POST", {
      cookie: "labtether_session=test-session",
      host: "hub.example",
      origin: "https://evil.example",
    }));
    expect(response?.status).toBe(403);
  });

  it("allows same-origin streaming uploads with a session", () => {
    const response = authorizeFileProxyRequest(request("POST", {
      cookie: "labtether_session=test-session",
      host: "hub.example",
      origin: "https://hub.example",
    }));
    expect(response).toBeNull();
  });

  it("forwards a body larger than the former 10 MiB proxy limit byte-for-byte", async () => {
    const chunk = new Uint8Array(1024 * 1024).fill(0x5a);
    const chunkCount = 12;
    const expectedHash = createHash("sha256");
    for (let i = 0; i < chunkCount; i += 1) expectedHash.update(chunk);

    let emitted = 0;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (emitted >= chunkCount) {
          controller.close();
          return;
        }
        controller.enqueue(chunk);
        emitted += 1;
      },
    });
    const upload = new Request("https://hub.example/api/files/asset-1/upload", {
      method: "POST",
      headers: {
        cookie: "labtether_session=test-session",
        host: "hub.example",
        origin: "https://hub.example",
        "content-type": "application/octet-stream",
      },
      body,
      duplex: "half",
    });

    let observedBytes = 0;
    const observedHash = createHash("sha256");
    let observedDuplex: RequestInit["duplex"];
    const response = await proxyToHub(
      upload,
      { params: Promise.resolve({ path: ["asset-1", "upload"] }) },
      {
        resolveBaseURLs: async () => ({ api: "http://hub.internal" }),
        fetchUpstream: async (_url, init) => {
          observedDuplex = init?.duplex;
          const reader = (init?.body as ReadableStream<Uint8Array>).getReader();
          for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            observedBytes += value.byteLength;
            observedHash.update(value);
          }
          return Response.json({ ok: true });
        },
      },
    );

    expect(response.status).toBe(200);
    expect(observedDuplex).toBe("half");
    expect(observedBytes).toBe(chunk.byteLength * chunkCount);
    expect(observedHash.digest("hex")).toBe(expectedHash.digest("hex"));
  });

  it("preserves the hub's upload-too-large response", async () => {
    const upload = request("POST", {
      cookie: "labtether_session=test-session",
      host: "hub.example",
      origin: "https://hub.example",
    });
    const response = await proxyToHub(
      upload,
      { params: Promise.resolve({ path: ["asset-1", "upload"] }) },
      {
        resolveBaseURLs: async () => ({ api: "http://hub.internal" }),
        fetchUpstream: async () => Response.json(
          { error: "file exceeds 512 MB limit" },
          { status: 413 },
        ),
      },
    );
    expect(response.status).toBe(413);
    await expect(response.json()).resolves.toEqual({ error: "file exceeds 512 MB limit" });
  });

  it("drops stale encoding and length headers from decoded upstream bodies", async () => {
    const download = request("GET", {
      cookie: "labtether_session=test-session",
      host: "hub.example",
    });
    const response = await proxyToHub(
      download,
      { params: Promise.resolve({ path: ["asset-1", "list"] }) },
      {
        resolveBaseURLs: async () => ({ api: "http://hub.internal" }),
        fetchUpstream: async () => new Response("decoded", {
          headers: {
            "content-encoding": "gzip",
            "content-length": "1234",
            "content-type": "application/json",
          },
        }),
      },
    );

    expect(response.headers.get("content-encoding")).toBeNull();
    expect(response.headers.get("content-length")).toBeNull();
    expect(response.headers.get("content-type")).toBe("application/json");
  });
});
