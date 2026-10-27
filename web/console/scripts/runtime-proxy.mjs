import { createServer as createHttpServer, request as createHttpRequest } from "node:http";
import { request as createHttpsRequest } from "node:https";
import { isIP } from "node:net";
import { checkServerIdentity } from "node:tls";

export const RUNTIME_PROXY_HOST = "127.0.0.1";
export const RUNTIME_PROXY_PORT = 3011;
// The bundled console ingress fetches the per-boot ingress key from this port.
// It is not published; only containers on the console network can reach it.
export const INGRESS_KEY_PORT = 3001;
export const INGRESS_HOSTNAME = "console-ingress";
const INGRESS_KEY_HEADER = "x-labtether-ingress-key";

export function normalizePeerAddress(raw) {
  const value = String(raw ?? "").trim();
  return value.startsWith("::ffff:") && value.includes(".") ? value.slice(7) : value;
}

function ipv4ToNumber(address) {
  const parts = String(address).split(".");
  if (parts.length !== 4) return null;
  let value = 0;
  for (const part of parts) {
    if (!/^[0-9]{1,3}$/.test(part) || Number(part) > 255) return null;
    value = value * 256 + Number(part);
  }
  return value;
}

// Reports whether peer is in the IPv4 subnet of a local interface.
export function ipv4InSubnet(peer, address, netmask) {
  const peerValue = ipv4ToNumber(peer);
  const addressValue = ipv4ToNumber(address);
  const maskValue = ipv4ToNumber(netmask);
  if (peerValue === null || addressValue === null || maskValue === null) return false;
  return ((peerValue & maskValue) >>> 0) === ((addressValue & maskValue) >>> 0);
}

// Serves the ingress key to the bundled console ingress, which sends it with
// every request so the console can trust the client address it appended.
// isAllowedPeer must accept only the ingress's own address.
export function createIngressKeyServer(key, isAllowedPeer = () => false) {
  return createHttpServer({ maxHeaderSize: 8 * 1024 }, (request, response) => {
    if (!isAllowedPeer(normalizePeerAddress(request.socket.remoteAddress))) {
      response.writeHead(403, { "content-type": "text/plain; charset=utf-8" });
      response.end("Forbidden");
      return;
    }
    if (request.method !== "GET" || request.url !== "/ingress-key") {
      response.writeHead(404, { "content-type": "text/plain; charset=utf-8" });
      response.end("Not Found");
      return;
    }
    response.writeHead(200, {
      "content-type": "text/plain; charset=utf-8",
      "content-length": Buffer.byteLength(key),
      "cache-control": "no-store",
    });
    response.end(key);
  });
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
