import { createServer as createHttpServer, request as createHttpRequest } from "node:http";
import { request as createHttpsRequest } from "node:https";
import { isIP } from "node:net";
import { checkServerIdentity } from "node:tls";

export const RUNTIME_PROXY_HOST = "127.0.0.1";
export const RUNTIME_PROXY_PORT = 3011;

const MAX_REQUEST_TARGET_BYTES = 16 * 1024;
const UPSTREAM_HANDSHAKE_TIMEOUT_MS = 10_000;
const CONTROL_OR_SPACE = /[\u0000-\u0020\u007f]/;
const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);
const HEADER_NAME_TOKEN = /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/;

function connectionNamedHeaders(rawValue) {
  const values = Array.isArray(rawValue) ? rawValue : [rawValue];
  const names = new Set();
  for (const raw of values) {
    if (typeof raw !== "string") continue;
    for (const candidate of raw.split(",")) {
      const name = candidate.trim().toLowerCase();
      if (name && HEADER_NAME_TOKEN.test(name)) names.add(name);
    }
  }
  return names;
}

function blockedHopByHopHeaders(headers) {
  const blocked = new Set(HOP_BY_HOP_HEADERS);
  for (const name of connectionNamedHeaders(headers?.connection)) blocked.add(name);
  return blocked;
}

function normalizeHostname(hostname) {
  return String(hostname ?? "").trim().replace(/^\[|\]$/g, "").toLowerCase();
}

export function parseProxyTarget(rawBaseURL, rawConnectHost = "") {
  const configured = String(rawBaseURL ?? "").trim();
  if (!configured) {
    throw new Error("LABTETHER_API_BASE_URL must be set for the console runtime proxy");
  }

  let url;
  try {
    url = new URL(configured);
  } catch {
    throw new Error("LABTETHER_API_BASE_URL must be an absolute HTTP(S) URL");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("LABTETHER_API_BASE_URL must use http or https");
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new Error("LABTETHER_API_BASE_URL cannot contain credentials, a query, or a fragment");
  }
  if (url.pathname !== "/") {
    throw new Error("LABTETHER_API_BASE_URL cannot contain a path");
  }
  const verificationHostname = normalizeHostname(url.hostname);
  if (!verificationHostname || verificationHostname === "0.0.0.0" || verificationHostname === "::") {
    throw new Error("LABTETHER_API_BASE_URL must name a concrete host");
  }

  const connectHostname = parseConnectHostname(rawConnectHost) || verificationHostname;
  return Object.freeze({
    url,
    verificationHostname,
    connectHostname,
    port: Number(url.port || (url.protocol === "https:" ? 443 : 80)),
  });
}

function parseConnectHostname(rawValue) {
  const value = String(rawValue ?? "").trim();
  if (!value) return "";
  if (CONTROL_OR_SPACE.test(value) || /[\\/@?#,]/.test(value)) {
    throw new Error("LABTETHER_WS_PROXY_CONNECT_HOST must contain one hostname or IP address");
  }

  const authority = isIP(value) === 6 ? `[${value}]` : value;
  let parsed;
  try {
    parsed = new URL(`http://${authority}`);
  } catch {
    throw new Error("LABTETHER_WS_PROXY_CONNECT_HOST is invalid");
  }
  if (parsed.port || parsed.username || parsed.password || parsed.pathname !== "/" || parsed.search || parsed.hash) {
    throw new Error("LABTETHER_WS_PROXY_CONNECT_HOST must not include a port or URL components");
  }
  const hostname = normalizeHostname(parsed.hostname);
  if (!hostname || hostname === "0.0.0.0" || hostname === "::") {
    throw new Error("LABTETHER_WS_PROXY_CONNECT_HOST must name a concrete host");
  }
  return hostname;
}

export function isAllowedProxyPath(pathname) {
  if (pathname === "/ws/events" || pathname === "/ws/agent") return true;
  if (pathname.startsWith("/desktop/sessions/") || pathname.startsWith("/terminal/sessions/")) return true;
  return /^\/portainer\/assets\/[^/]+\/containers\/[^/]+\/exec$/.test(pathname);
}

function parseRequestTarget(rawTarget) {
  if (
    typeof rawTarget !== "string"
    || !rawTarget.startsWith("/")
    || rawTarget.startsWith("//")
    || rawTarget.includes("\\")
    || Buffer.byteLength(rawTarget) > MAX_REQUEST_TARGET_BYTES
  ) {
    return null;
  }
  try {
    const parsed = new URL(rawTarget, "http://runtime-proxy.invalid");
    if (parsed.origin !== "http://runtime-proxy.invalid" || !isAllowedProxyPath(parsed.pathname)) return null;
    return parsed;
  } catch {
    return null;
  }
}

function parseForwardedHost(rawValue) {
  if (Array.isArray(rawValue) || typeof rawValue !== "string") return null;
  const value = rawValue.trim();
  if (!value || CONTROL_OR_SPACE.test(value) || /[\\/@?#,]/.test(value)) return null;

  try {
    const parsed = new URL(`http://${value}`);
    if (parsed.username || parsed.password || parsed.pathname !== "/" || parsed.search || parsed.hash) return null;
    return { host: parsed.host, hostname: normalizeHostname(parsed.hostname) };
  } catch {
    return null;
  }
}

function isLoopbackHostname(hostname) {
  return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1";
}

function hostnamesMatch(left, right) {
  return left === right || (isLoopbackHostname(left) && isLoopbackHostname(right));
}

export function validateForwardingContext(headers) {
  const forwardedHost = parseForwardedHost(headers["x-forwarded-host"]);
  const rawOrigin = typeof headers.origin === "string" ? headers.origin.trim() : "";
  let forwardedProto = "";

  if (rawOrigin && rawOrigin !== "null") {
    let origin;
    try {
      origin = new URL(rawOrigin);
    } catch {
      return { ok: false, reason: "invalid_origin" };
    }

    if (origin.protocol === "http:" || origin.protocol === "https:") {
      if (
        origin.username
        || origin.password
        || origin.pathname !== "/"
        || origin.search
        || origin.hash
        || !forwardedHost
        || !hostnamesMatch(normalizeHostname(origin.hostname), forwardedHost.hostname)
      ) {
        return { ok: false, reason: "origin_host_mismatch" };
      }
      forwardedProto = origin.protocol.slice(0, -1);
    }
  }

  if (!forwardedProto) {
    const candidate = typeof headers["x-forwarded-proto"] === "string"
      ? headers["x-forwarded-proto"].split(",", 1)[0].trim().toLowerCase()
      : "";
    if (candidate === "http" || candidate === "https") forwardedProto = candidate;
  }

  return {
    ok: true,
    forwardedHost: forwardedHost?.host ?? "",
    forwardedProto,
  };
}

function buildUpstreamHeaders(incomingHeaders, target, forwarding, isWebSocket) {
  const headers = {};
  const blockedHeaders = blockedHopByHopHeaders(incomingHeaders);
  for (const [name, value] of Object.entries(incomingHeaders)) {
    const lowerName = name.toLowerCase();
    if (
      value == null
      || lowerName === "host"
      || lowerName === "forwarded"
      || lowerName.startsWith("x-forwarded-")
      || blockedHeaders.has(lowerName)
    ) {
      continue;
    }
    headers[lowerName] = value;
  }

  headers.host = target.url.host;
  if (forwarding.forwardedHost) headers["x-forwarded-host"] = forwarding.forwardedHost;
  if (forwarding.forwardedProto) headers["x-forwarded-proto"] = forwarding.forwardedProto;
  if (isWebSocket) {
    headers.connection = "Upgrade";
    headers.upgrade = "websocket";
  }
  return headers;
}

function upstreamRequestOptions(request, target, parsedTarget, forwarding, isWebSocket) {
  const options = {
    hostname: target.connectHostname,
    port: target.port,
    method: request.method,
    path: `${parsedTarget.pathname}${parsedTarget.search}`,
    headers: buildUpstreamHeaders(request.headers, target, forwarding, isWebSocket),
    rejectUnauthorized: true,
  };
  if (target.url.protocol === "https:") {
    if (!isIP(target.verificationHostname)) options.servername = target.verificationHostname;
    options.checkServerIdentity = (_hostname, cert) => checkServerIdentity(target.verificationHostname, cert);
  }
  return options;
}

function requestFactory(target) {
  return target.url.protocol === "https:" ? createHttpsRequest : createHttpRequest;
}

function safeErrorCode(error) {
  const code = typeof error?.code === "string" ? error.code : "UPSTREAM_ERROR";
  return /^[A-Z0-9_]+$/.test(code) ? code : "UPSTREAM_ERROR";
}

function logUpstreamError(pathname, error) {
  console.error(`console-runtime-proxy: upstream failure for ${pathname} (${safeErrorCode(error)})`);
}

function proxyHttpRequest(request, response, target, parsedTarget, forwarding) {
  const upstream = requestFactory(target)(
    upstreamRequestOptions(request, target, parsedTarget, forwarding, false),
    (upstreamResponse) => {
      const responseHeaders = {};
      const blockedHeaders = blockedHopByHopHeaders(upstreamResponse.headers);
      for (const [name, value] of Object.entries(upstreamResponse.headers)) {
        if (value != null && !blockedHeaders.has(name.toLowerCase())) responseHeaders[name] = value;
      }
      response.writeHead(upstreamResponse.statusCode ?? 502, upstreamResponse.statusMessage, responseHeaders);
      upstreamResponse.pipe(response);
    },
  );

  upstream.setTimeout(UPSTREAM_HANDSHAKE_TIMEOUT_MS, () => upstream.destroy(Object.assign(new Error("timeout"), { code: "ETIMEDOUT" })));
  upstream.on("error", (error) => {
    logUpstreamError(parsedTarget.pathname, error);
    if (!response.headersSent) response.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    response.end("Bad Gateway");
  });
  request.on("aborted", () => upstream.destroy());
  request.pipe(upstream);
}

function writeRawResponseHead(socket, response) {
  const statusCode = response.statusCode ?? 502;
  const statusMessage = response.statusMessage || "Bad Gateway";
  let block = `HTTP/${response.httpVersion || "1.1"} ${statusCode} ${statusMessage}\r\n`;
  for (let index = 0; index < response.rawHeaders.length; index += 2) {
    block += `${response.rawHeaders[index]}: ${response.rawHeaders[index + 1]}\r\n`;
  }
  socket.write(`${block}\r\n`);
}

function rejectUpgrade(socket, statusCode, statusMessage) {
  if (!socket.destroyed) {
    socket.end(
      `HTTP/1.1 ${statusCode} ${statusMessage}\r\nConnection: close\r\nContent-Length: 0\r\n\r\n`,
    );
  }
}

function proxyWebSocketUpgrade(request, socket, head, target, parsedTarget, forwarding) {
  let upgraded = false;
  const upstream = requestFactory(target)(
    upstreamRequestOptions(request, target, parsedTarget, forwarding, true),
  );

  upstream.setTimeout(UPSTREAM_HANDSHAKE_TIMEOUT_MS, () => upstream.destroy(Object.assign(new Error("timeout"), { code: "ETIMEDOUT" })));
  upstream.on("upgrade", (upstreamResponse, upstreamSocket, upstreamHead) => {
    upgraded = true;
    upstream.setTimeout(0);
    writeRawResponseHead(socket, upstreamResponse);
    if (upstreamHead.length > 0) socket.write(upstreamHead);
    if (head.length > 0) upstreamSocket.write(head);
    socket.on("error", () => upstreamSocket.destroy());
    upstreamSocket.on("error", () => socket.destroy());
    socket.pipe(upstreamSocket).pipe(socket);
  });
  upstream.on("response", (upstreamResponse) => {
    writeRawResponseHead(socket, upstreamResponse);
    upstreamResponse.pipe(socket);
  });
  upstream.on("error", (error) => {
    logUpstreamError(parsedTarget.pathname, error);
    if (!upgraded) rejectUpgrade(socket, 502, "Bad Gateway");
  });
  socket.on("close", () => {
    if (!upgraded) upstream.destroy();
  });
  upstream.end();
}

export function createRuntimeProxyServer(target) {
  const server = createHttpServer({ maxHeaderSize: 16 * 1024 }, (request, response) => {
    const parsedTarget = parseRequestTarget(request.url);
    if (!parsedTarget) {
      response.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
      response.end("Not Found");
      return;
    }
    if (request.method === "CONNECT" || request.method === "TRACE") {
      response.writeHead(405, { allow: "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS" });
      response.end();
      return;
    }
    const forwarding = validateForwardingContext(request.headers);
    if (!forwarding.ok) {
      response.writeHead(403, { "content-type": "text/plain; charset=utf-8" });
      response.end("Forbidden");
      return;
    }
    proxyHttpRequest(request, response, target, parsedTarget, forwarding);
  });

  server.on("upgrade", (request, socket, head) => {
    const parsedTarget = parseRequestTarget(request.url);
    if (!parsedTarget) {
      rejectUpgrade(socket, 404, "Not Found");
      return;
    }
    if (request.method !== "GET" || String(request.headers.upgrade ?? "").toLowerCase() !== "websocket") {
      rejectUpgrade(socket, 400, "Bad Request");
      return;
    }
    const forwarding = validateForwardingContext(request.headers);
    if (!forwarding.ok) {
      rejectUpgrade(socket, 403, "Forbidden");
      return;
    }
    proxyWebSocketUpgrade(request, socket, head, target, parsedTarget, forwarding);
  });

  return server;
}
