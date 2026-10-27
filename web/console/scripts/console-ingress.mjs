import { createServer, request as httpRequest } from "node:http";
import { connect } from "node:net";
import { pathToFileURL } from "node:url";

const LISTEN_HOST = "0.0.0.0";
const LISTEN_PORT = 3000;
const TARGET_HOST = "web-console";
const TARGET_PORT = 3000;
// The console runtime hands out a per-boot key on this unpublished port. The
// key proves to the console that a request came through this ingress.
const INGRESS_KEY_PORT = 3001;
const INGRESS_KEY_HEADER = "x-labtether-ingress-key";
const KEY_REFRESH_MS = 60_000;
const KEY_RETRY_MS = 5_000;
const CONNECT_TIMEOUT_MS = 10_000;
const SHUTDOWN_TIMEOUT_MS = 10_000;
const DEFAULT_MAX_CONNECTIONS = 512;
const MAX_CONNECTIONS_HARD_LIMIT = 4096;
const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "proxy-connection",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "expect",
]);

export function parseMaxConnections(rawValue) {
  const value = String(rawValue ?? "").trim();
  if (!/^[1-9][0-9]*$/.test(value)) return DEFAULT_MAX_CONNECTIONS;
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed > MAX_CONNECTIONS_HARD_LIMIT) {
    return DEFAULT_MAX_CONNECTIONS;
  }
  return parsed;
}

function peerAddress(socket) {
  const raw = String(socket?.remoteAddress ?? "").trim();
  return raw.startsWith("::ffff:") && raw.includes(".") ? raw.slice(7) : raw;
}

// Builds the upstream header list. The address this ingress accepted is
// appended to X-Forwarded-For; entries a client wrote stay to its left, and the
// console counts only the right-hand entries real proxies added.
export function forwardedRequestHeaders(rawHeaders, peer, ingressKey, keepUpgrade) {
  const connectionNamed = new Set();
  for (let index = 0; index < rawHeaders.length; index += 2) {
    if (rawHeaders[index].toLowerCase() !== "connection") continue;
    for (const token of String(rawHeaders[index + 1]).split(",")) {
      const name = token.trim().toLowerCase();
      if (name) connectionNamed.add(name);
    }
  }
  const headers = [];
  let forwardedFor = "";
  for (let index = 0; index < rawHeaders.length; index += 2) {
    const name = rawHeaders[index];
    const value = rawHeaders[index + 1];
    const lowerName = name.toLowerCase();
    if (lowerName === "x-forwarded-for") {
      forwardedFor = forwardedFor ? `${forwardedFor}, ${value}` : String(value);
      continue;
    }
    if (lowerName === INGRESS_KEY_HEADER) continue;
    if (keepUpgrade && (lowerName === "connection" || lowerName === "upgrade")) {
      headers.push(name, value);
      continue;
    }
    if (HOP_BY_HOP_HEADERS.has(lowerName) || connectionNamed.has(lowerName)) continue;
    headers.push(name, value);
  }
  if (!peer) {
    // Without a peer address there is no entry this ingress can vouch for.
    if (forwardedFor.trim()) headers.push("X-Forwarded-For", forwardedFor.trim());
    return headers;
  }
  headers.push("X-Forwarded-For", forwardedFor.trim() ? `${forwardedFor.trim()}, ${peer}` : peer);
  if (ingressKey) headers.push("X-Labtether-Ingress-Key", ingressKey);
  return headers;
}

function responseHeaders(rawHeaders) {
  const connectionNamed = new Set();
  for (let index = 0; index < rawHeaders.length; index += 2) {
    if (rawHeaders[index].toLowerCase() !== "connection") continue;
    for (const token of String(rawHeaders[index + 1]).split(",")) connectionNamed.add(token.trim().toLowerCase());
  }
  const headers = [];
  for (let index = 0; index < rawHeaders.length; index += 2) {
    const lowerName = rawHeaders[index].toLowerCase();
    if (HOP_BY_HOP_HEADERS.has(lowerName) || connectionNamed.has(lowerName)) continue;
    headers.push(rawHeaders[index], rawHeaders[index + 1]);
  }
  return headers;
}

function boundConnect(target, onConnect, onFailure) {
  const upstream = connect({ host: target.host, port: target.port, allowHalfOpen: false });
  upstream.setNoDelay(true);
  upstream.setTimeout(CONNECT_TIMEOUT_MS);
  upstream.once("connect", () => {
    upstream.setTimeout(0);
    onConnect(upstream);
  });
  upstream.once("timeout", () => upstream.destroy());
  upstream.once("error", onFailure);
  return upstream;
}

function proxyRequest(request, response, target, ingressKey) {
  const upstream = httpRequest({
    method: request.method,
    path: request.url,
    headers: forwardedRequestHeaders(request.rawHeaders, peerAddress(request.socket), ingressKey(), false),
    createConnection: (_options, callback) => boundConnect(target, (socket) => callback(null, socket), callback),
  }, (upstreamResponse) => {
    response.writeHead(upstreamResponse.statusCode ?? 502, upstreamResponse.statusMessage, responseHeaders(upstreamResponse.rawHeaders));
    upstreamResponse.once("error", () => response.destroy());
    upstreamResponse.pipe(response);
  });
  upstream.once("error", () => {
    if (!response.headersSent) {
      response.writeHead(502, { "content-type": "text/plain; charset=utf-8", connection: "close" });
      response.end("Bad Gateway");
    } else {
      response.destroy();
    }
  });
  response.once("close", () => {
    if (!response.writableFinished) upstream.destroy();
  });
  request.pipe(upstream);
}

function proxyUpgrade(request, downstream, head, target, ingressKey) {
  const fail = () => {
    if (!downstream.destroyed) downstream.destroy();
  };
  const upstream = boundConnect(target, (socket) => {
    const headers = forwardedRequestHeaders(request.rawHeaders, peerAddress(downstream), ingressKey(), true);
    let block = `${request.method} ${request.url} HTTP/${request.httpVersion}\r\n`;
    for (let index = 0; index < headers.length; index += 2) block += `${headers[index]}: ${headers[index + 1]}\r\n`;
    socket.write(`${block}\r\n`);
    if (head?.length) socket.write(head);
    downstream.pipe(socket);
    socket.pipe(downstream);
  }, fail);
  downstream.once("error", () => upstream.destroy());
  downstream.once("close", () => upstream.destroy());
  upstream.once("close", fail);
}

export function createConsoleIngress(
  maxConnections = DEFAULT_MAX_CONNECTIONS,
  target = { host: TARGET_HOST, port: TARGET_PORT },
  ingressKey = () => "",
) {
  const downstreamSockets = new Set();
  const server = createServer({ maxHeaderSize: 16 * 1024 }, (request, response) => {
    proxyRequest(request, response, target, ingressKey);
  });
  // Console file uploads can take longer than Node's default request window.
  server.requestTimeout = 0;
  server.maxConnections = maxConnections;
  server.on("connection", (socket) => {
    downstreamSockets.add(socket);
    socket.setNoDelay(true);
    socket.setKeepAlive(true, 30_000);
    socket.once("close", () => downstreamSockets.delete(socket));
  });
  server.on("upgrade", (request, socket, head) => proxyUpgrade(request, socket, head, target, ingressKey));
  server.on("clientError", (_error, socket) => {
    if (socket.writable) socket.end("HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n");
    else socket.destroy();
  });
  return { server, downstreamSockets };
}

// Keeps the console runtime's current ingress key. Without a key, requests
// still flow; the console then treats the client address as unknown.
export function startIngressKeyRefresh(target = { host: TARGET_HOST, port: INGRESS_KEY_PORT }) {
  let current = "";
  let timer = null;
  const schedule = (delay) => {
    timer = setTimeout(refresh, delay);
    timer.unref();
  };
  const refresh = () => {
    const request = httpRequest({ host: target.host, port: target.port, path: "/ingress-key", timeout: CONNECT_TIMEOUT_MS }, (response) => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => {
        if (body.length < 256) body += chunk;
      });
      response.once("end", () => {
        const key = body.trim();
        const valid = response.statusCode === 200 && /^[0-9a-f]{32,128}$/.test(key);
        if (valid) current = key;
        schedule(valid ? KEY_REFRESH_MS : KEY_RETRY_MS);
      });
    });
    request.once("timeout", () => request.destroy());
    request.once("error", () => schedule(KEY_RETRY_MS));
    request.end();
  };
  refresh();
  return { current: () => current, stop: () => timer && clearTimeout(timer) };
}

function isMainModule() {
  return process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href;
}

if (isMainModule()) {
  const maxConnections = parseMaxConnections(process.env.LABTETHER_CONSOLE_INGRESS_MAX_CONNECTIONS);
  const keyRefresh = startIngressKeyRefresh();
  const { server, downstreamSockets } = createConsoleIngress(
    maxConnections,
    { host: TARGET_HOST, port: TARGET_PORT },
    keyRefresh.current,
  );
  let shuttingDown = false;

  const shutdown = () => {
    if (shuttingDown) return;
    shuttingDown = true;
    keyRefresh.stop();
    server.close(() => process.exit(0));
    setTimeout(() => {
      for (const socket of downstreamSockets) socket.destroy();
      process.exit(0);
    }, SHUTDOWN_TIMEOUT_MS).unref();
  };

  for (const signal of ["SIGINT", "SIGTERM"]) process.once(signal, shutdown);
  server.once("error", (error) => {
    const code = typeof error?.code === "string" && /^[A-Z0-9_]+$/.test(error.code)
      ? error.code
      : "INGRESS_ERROR";
    console.error(`console-ingress: listener failed (${code})`);
    process.exit(1);
  });
  server.listen({ host: LISTEN_HOST, port: LISTEN_PORT, exclusive: true }, () => {
    console.log(`console-ingress: forwarding http/3000 with a ${maxConnections}-connection ceiling`);
  });
}
