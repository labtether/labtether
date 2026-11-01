import { createServer } from "node:http";
import { connect } from "node:net";
import { afterEach, describe, expect, it } from "vitest";
import { createConsoleIngress, forwardedRequestHeaders, parseMaxConnections } from "../console-ingress.mjs";
import { createIngressKeyServer, ipv4InSubnet } from "../runtime-proxy.mjs";

const openServers = [];

async function listen(server) {
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  openServers.push(server);
  return server.address().port;
}

function rawExchange(port, request, done) {
  return new Promise((resolve, reject) => {
    const socket = connect(port, "127.0.0.1");
    let response = "";
    socket.setEncoding("utf8");
    socket.once("connect", () => socket.write(request));
    socket.on("data", (chunk) => {
      response += chunk;
      if (done(response)) socket.end();
    });
    socket.once("end", () => resolve(response));
    socket.once("error", reject);
  });
}

afterEach(async () => {
  await Promise.all(openServers.splice(0).map((server) => new Promise((resolve) => {
    server.closeAllConnections?.();
    server.close(() => resolve());
  })));
});

// The console needs the real client address for per-client sign-in limits.
// The ingress must append the address it accepted, keep the Host, and replace
// any client-supplied ingress key with its own.
describe("console ingress forwarding", () => {
  it("appends the peer address and its key to plain requests", async () => {
    let observed;
    const upstream = createServer((request, response) => {
      observed = request.headers;
      response.writeHead(200, { "content-type": "text/plain" });
      response.end("ok");
    });
    const upstreamPort = await listen(upstream);
    const { server } = createConsoleIngress(8, { host: "127.0.0.1", port: upstreamPort }, () => "c".repeat(64));
    const ingressPort = await listen(server);

    const response = await rawExchange(
      ingressPort,
      "GET /login HTTP/1.1\r\nHost: console.example:3000\r\n"
      + "X-Forwarded-For: 198.51.100.1\r\nX-Labtether-Ingress-Key: forged\r\nConnection: close\r\n\r\n",
      (text) => text.includes("ok"),
    );

    expect(response).toContain("200");
    expect(observed.host).toBe("console.example:3000");
    expect(observed["x-forwarded-for"]).toBe("198.51.100.1, 127.0.0.1");
    expect(observed["x-labtether-ingress-key"]).toBe("c".repeat(64));
  });

  it("forwards WebSocket upgrades with the same headers", async () => {
    let observed;
    const upstream = createServer();
    upstream.on("upgrade", (request, socket) => {
      observed = request.headers;
      socket.end("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n");
    });
    const upstreamPort = await listen(upstream);
    const { server } = createConsoleIngress(8, { host: "127.0.0.1", port: upstreamPort }, () => "");
    const ingressPort = await listen(server);

    const response = await rawExchange(
      ingressPort,
      "GET /ws/events HTTP/1.1\r\nHost: console.example\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n"
      + "Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGVzdC1ub25jZQ==\r\n\r\n", // gitleaks:allow -- public WebSocket test nonce
      (text) => text.includes("\r\n\r\n"),
    );

    expect(response).toContain("101 Switching Protocols");
    expect(observed.upgrade).toBe("websocket");
    expect(observed["x-forwarded-for"]).toBe("127.0.0.1");
    expect(observed["x-labtether-ingress-key"]).toBeUndefined();
  });

  // A request target that Node's server accepts but http.request refuses
  // (such as a tab) must get 400, not stop the ingress for everyone.
  it("answers a malformed request target with 400 and keeps serving", async () => {
    const upstream = createServer((_request, response) => response.end("ok"));
    const upstreamPort = await listen(upstream);
    const { server } = createConsoleIngress(8, { host: "127.0.0.1", port: upstreamPort }, () => "");
    const ingressPort = await listen(server);

    const bad = await rawExchange(ingressPort, "GET /a\tb HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n", (text) => text.includes("\r\n\r\n"));
    expect(bad).toContain("400");
    const good = await rawExchange(ingressPort, "GET /ok HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n", (text) => text.includes("ok"));
    expect(good).toContain("200");
  });

  it("does not vouch for a request without a peer address", () => {
    const headers = forwardedRequestHeaders(["Host", "console", "X-Forwarded-For", "198.51.100.1"], "", "c".repeat(64), false);
    expect(headers).not.toContain("X-Labtether-Ingress-Key");
  });

  it("keeps the connection ceiling parser", () => {
    expect(parseMaxConnections("16")).toBe(16);
    expect(parseMaxConnections("0")).toBe(512);
  });
});

// Anyone who learns the key can forge client addresses, so only the ingress
// itself may read it.
describe("ingress key server", () => {
  const done = (text) => text.includes("\r\n\r\n") && /\r\n\r\n.+/s.test(text);
  const keyRequest = "GET /ingress-key HTTP/1.1\r\nHost: web-console\r\nConnection: close\r\n\r\n";

  it("serves the key only to the ingress address, at its fixed path", async () => {
    const port = await listen(createIngressKeyServer("d".repeat(64), (peer) => peer === "127.0.0.1"));
    const ok = await rawExchange(port, keyRequest, done);
    expect(ok).toContain("200");
    expect(ok).toContain(`\r\n\r\n${"d".repeat(64)}`);
    const missing = await rawExchange(port, "GET /other HTTP/1.1\r\nHost: web-console\r\nConnection: close\r\n\r\n", done);
    expect(missing).toContain("404");
  });

  it("refuses every other address", async () => {
    const port = await listen(createIngressKeyServer("d".repeat(64), () => false));
    const refused = await rawExchange(port, keyRequest, done);
    expect(refused).toContain("403");
    expect(refused).not.toContain("d".repeat(64));
  });

  it("matches the interface subnet that faces the ingress", () => {
    expect(ipv4InSubnet("172.20.0.5", "172.20.0.3", "255.255.0.0")).toBe(true);
    expect(ipv4InSubnet("192.168.1.9", "172.20.0.3", "255.255.0.0")).toBe(false);
  });
});
