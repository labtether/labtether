import { createServer, createConnection } from "node:net";
import { pathToFileURL } from "node:url";

const LISTEN_HOST = "0.0.0.0";
const LISTEN_PORT = 3000;
const TARGET_HOST = "web-console";
const TARGET_PORT = 3000;
const CONNECT_TIMEOUT_MS = 10_000;
const SHUTDOWN_TIMEOUT_MS = 10_000;
const DEFAULT_MAX_CONNECTIONS = 512;
const MAX_CONNECTIONS_HARD_LIMIT = 4096;

export function parseMaxConnections(rawValue) {
  const value = String(rawValue ?? "").trim();
  if (!/^[1-9][0-9]*$/.test(value)) return DEFAULT_MAX_CONNECTIONS;
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed > MAX_CONNECTIONS_HARD_LIMIT) {
    return DEFAULT_MAX_CONNECTIONS;
  }
  return parsed;
}

function destroyPair(downstream, upstream) {
  if (!downstream.destroyed) downstream.destroy();
  if (!upstream.destroyed) upstream.destroy();
}

export function createConsoleIngress(maxConnections = DEFAULT_MAX_CONNECTIONS) {
  const downstreamSockets = new Set();
  const server = createServer({ allowHalfOpen: false, pauseOnConnect: true }, (downstream) => {
    if (downstreamSockets.size >= maxConnections) {
      downstream.destroy();
      return;
    }

    downstreamSockets.add(downstream);
    downstream.setNoDelay(true);
    downstream.setKeepAlive(true, 30_000);

    const upstream = createConnection({
      host: TARGET_HOST,
      port: TARGET_PORT,
      allowHalfOpen: false,
    });
    upstream.setNoDelay(true);
    upstream.setKeepAlive(true, 30_000);
    upstream.setTimeout(CONNECT_TIMEOUT_MS);

    upstream.once("connect", () => {
      upstream.setTimeout(0);
      downstream.resume();
      downstream.pipe(upstream);
      upstream.pipe(downstream);
    });
    upstream.once("timeout", () => destroyPair(downstream, upstream));
    upstream.once("error", () => destroyPair(downstream, upstream));
    downstream.once("error", () => destroyPair(downstream, upstream));
    downstream.once("close", () => {
      downstreamSockets.delete(downstream);
      if (!upstream.destroyed) upstream.destroy();
    });
    upstream.once("close", () => {
      if (!downstream.destroyed) downstream.destroy();
    });
  });

  server.maxConnections = maxConnections;
  return { server, downstreamSockets };
}

function isMainModule() {
  return process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href;
}

if (isMainModule()) {
  const maxConnections = parseMaxConnections(process.env.LABTETHER_CONSOLE_INGRESS_MAX_CONNECTIONS);
  const { server, downstreamSockets } = createConsoleIngress(maxConnections);
  let shuttingDown = false;

  const shutdown = () => {
    if (shuttingDown) return;
    shuttingDown = true;
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
    console.log(`console-ingress: forwarding tcp/3000 with a ${maxConnections}-connection ceiling`);
  });
}
