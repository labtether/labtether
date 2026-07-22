import { createServer } from "node:http";
import { connect } from "node:net";
import { afterEach, describe, expect, it } from "vitest";
import {
  createRuntimeProxyServer,
  isAllowedProxyPath,
  parseProxyTarget,
  validateForwardingContext,
} from "../runtime-proxy.mjs";

const openServers = [];

async function listen(server, host = "127.0.0.1") {
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, host, resolve);
  });
  openServers.push(server);
  return server.address().port;
}

async function closeServer(server) {
  if (!server.listening) return;
  await new Promise((resolve) => server.close(resolve));
}

function rawRequest(port, request) {
  return new Promise((resolve, reject) => {
    const socket = connect(port, "127.0.0.1");
    let response = "";
    socket.setEncoding("utf8");
    socket.once("connect", () => socket.write(request));
    socket.on("data", (chunk) => {
      response += chunk;
      if (response.includes("\r\n\r\n")) socket.end();
    });
    socket.once("end", () => resolve(response));
    socket.once("error", reject);
  });
}

afterEach(async () => {
  await Promise.all(openServers.splice(0).map(closeServer));
});

describe("runtime proxy configuration", () => {
  it("accepts an HTTPS runtime target and separate connection hostname", () => {
    const target = parseProxyTarget("https://hub.internal.example:8443", "labtether");
    expect(target.url.origin).toBe("https://hub.internal.example:8443");
    expect(target.connectHostname).toBe("labtether");
    expect(target.port).toBe(8443);
  });

  it("normalizes IPv6 hostnames for connection and certificate verification", () => {
    const target = parseProxyTarget("https://[::1]:8443", "::1");
    expect(target.verificationHostname).toBe("::1");
    expect(target.connectHostname).toBe("::1");
  });

  it.each([
    "file:///tmp/hub.sock",
    "https://user:secret@hub.example",
    "https://hub.example/api",
    "https://hub.example?redirect=https://evil.example",
    "https://0.0.0.0:8443",
  ])("rejects unsafe runtime target %s", (value) => {
    expect(() => parseProxyTarget(value)).toThrow();
  });

  it("allows only the existing same-origin stream routes", () => {
    expect(isAllowedProxyPath("/ws/events")).toBe(true);
    expect(isAllowedProxyPath("/ws/agent")).toBe(true);
    expect(isAllowedProxyPath("/terminal/sessions/session-1/stream")).toBe(true);
    expect(isAllowedProxyPath("/desktop/sessions/session-1/stream")).toBe(true);
    expect(isAllowedProxyPath("/portainer/assets/a/containers/c/exec")).toBe(true);
    expect(isAllowedProxyPath("/api/v1/admin/users")).toBe(false);
    expect(isAllowedProxyPath("/portainer/assets/a/containers/c/exec/extra")).toBe(false);
  });

  it("requires a network Origin to match Next's forwarded console host", () => {
    expect(validateForwardingContext({
      origin: "https://console.example:9443",
      "x-forwarded-host": "console.example:9443",
    })).toMatchObject({ ok: true, forwardedHost: "console.example:9443", forwardedProto: "https" });

    expect(validateForwardingContext({
      origin: "https://evil.example",
      "x-forwarded-host": "console.example",
    })).toEqual({ ok: false, reason: "origin_host_mismatch" });
  });
});

describe("runtime proxy forwarding", () => {
  it("preserves ticket queries, cookies, Origin and a validated forwarded host on upgrade", async () => {
    let observed;
    const upstream = createServer();
    upstream.on("upgrade", (request, socket) => {
      observed = {
        url: request.url,
        cookie: request.headers.cookie,
        origin: request.headers.origin,
        forwardedHost: request.headers["x-forwarded-host"],
        host: request.headers.host,
      };
      socket.end(
        "HTTP/1.1 101 Switching Protocols\r\n"
        + "Connection: Upgrade\r\n"
        + "Upgrade: websocket\r\n"
        + "Sec-WebSocket-Accept: test-accept\r\n\r\n",
      );
    });
    const upstreamPort = await listen(upstream);
    const proxy = createRuntimeProxyServer(parseProxyTarget(`http://127.0.0.1:${upstreamPort}`));
    const proxyPort = await listen(proxy);

    const response = await rawRequest(
      proxyPort,
      "GET /ws/events?ticket=one-time-secret HTTP/1.1\r\n"
      + "Host: 127.0.0.1:3011\r\n"
      + "Connection: Upgrade\r\n"
      + "Upgrade: websocket\r\n"
      + "Sec-WebSocket-Version: 13\r\n"
      + "Sec-WebSocket-Key: dGVzdC1ub25jZQ==\r\n" // gitleaks:allow -- deterministic public WebSocket test nonce
      + "Cookie: labtether_session=session-value\r\n"
      + "Origin: https://console.example\r\n"
      + "X-Forwarded-Host: console.example\r\n"
      + "X-Forwarded-Proto: http\r\n\r\n",
    );

    expect(response).toContain("101 Switching Protocols");
    expect(observed).toEqual({
      url: "/ws/events?ticket=one-time-secret",
      cookie: "labtether_session=session-value",
      origin: "https://console.example",
      forwardedHost: "console.example",
      host: `127.0.0.1:${upstreamPort}`,
    });
  });

  it("rejects a mismatched browser Origin before contacting the upstream", async () => {
    let upstreamRequests = 0;
    const upstream = createServer(() => {
      upstreamRequests += 1;
    });
    upstream.on("upgrade", () => {
      upstreamRequests += 1;
    });
    const upstreamPort = await listen(upstream);
    const proxy = createRuntimeProxyServer(parseProxyTarget(`http://127.0.0.1:${upstreamPort}`));
    const proxyPort = await listen(proxy);

    const response = await rawRequest(
      proxyPort,
      "GET /ws/events HTTP/1.1\r\n"
      + "Host: 127.0.0.1:3011\r\n"
      + "Connection: Upgrade\r\n"
      + "Upgrade: websocket\r\n"
      + "Origin: https://evil.example\r\n"
      + "X-Forwarded-Host: console.example\r\n\r\n",
    );

    expect(response).toContain("403 Forbidden");
    expect(upstreamRequests).toBe(0);
  });

  it("continues to proxy ordinary HTTP requests on an allowlisted rewrite", async () => {
    let observed;
    const upstream = createServer((request, response) => {
      observed = { method: request.method, url: request.url, authorization: request.headers.authorization };
      response.writeHead(202, { "content-type": "application/json" });
      response.end('{"ok":true}');
    });
    const upstreamPort = await listen(upstream);
    const proxy = createRuntimeProxyServer(parseProxyTarget(`http://127.0.0.1:${upstreamPort}`));
    const proxyPort = await listen(proxy);

    const response = await fetch(`http://127.0.0.1:${proxyPort}/ws/agent?probe=1`, {
      headers: { authorization: "Bearer retained-token" },
    });

    expect(response.status).toBe(202);
    expect(await response.json()).toEqual({ ok: true });
    expect(observed).toEqual({
      method: "GET",
      url: "/ws/agent?probe=1",
      authorization: "Bearer retained-token",
    });
  });

  it("removes headers nominated by Connection in both proxy directions", async () => {
    let observed;
    const upstream = createServer((request, response) => {
      observed = {
        removed: request.headers["x-remove-request"],
        retained: request.headers["x-end-to-end-request"],
      };
      response.writeHead(200, {
        Connection: "close, x-remove-response",
        "X-Remove-Response": "must-not-cross",
        "X-End-To-End-Response": "retained",
      });
      response.end("ok");
    });
    const upstreamPort = await listen(upstream);
    const proxy = createRuntimeProxyServer(parseProxyTarget(`http://127.0.0.1:${upstreamPort}`));
    const proxyPort = await listen(proxy);

    const response = await rawRequest(
      proxyPort,
      "GET /ws/agent HTTP/1.1\r\n"
      + `Host: 127.0.0.1:${proxyPort}\r\n`
      + "Connection: close, x-remove-request\r\n"
      + "X-Remove-Request: must-not-cross\r\n"
      + "X-End-To-End-Request: retained\r\n\r\n",
    );

    expect(response).toContain("200 OK");
    expect(observed).toEqual({ removed: undefined, retained: "retained" });
    expect(response.toLowerCase()).not.toContain("x-remove-response:");
    expect(response.toLowerCase()).toContain("x-end-to-end-response: retained");
  });
});
