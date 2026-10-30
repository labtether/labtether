import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { lookup } from "node:dns/promises";
import { once } from "node:events";
import { networkInterfaces } from "node:os";
import path from "node:path";
import {
  createIngressKeyServer,
  createRuntimeProxyServer,
  INGRESS_HOSTNAME,
  INGRESS_KEY_PORT,
  ipv4InSubnet,
  parseProxyTarget,
  RUNTIME_PROXY_HOST,
  RUNTIME_PROXY_PORT,
} from "./runtime-proxy.mjs";

const target = parseProxyTarget(
  process.env.LABTETHER_API_BASE_URL || "http://localhost:8080",
  process.env.LABTETHER_WS_PROXY_CONNECT_HOST,
);
const proxy = createRuntimeProxyServer(target);

proxy.listen(RUNTIME_PROXY_PORT, RUNTIME_PROXY_HOST);
await once(proxy, "listening");

// A per-boot key lets the console trust the client address the bundled ingress
// adds. The key is served only where that ingress exists: on the interface
// that faces it, and only to its address. Images without the ingress (the
// all-in-one image, the dev Compose) never serve it, so nobody can learn it.
const ingressKey = randomBytes(32).toString("hex");
const ingressPeers = new Set();
const keyServer = createIngressKeyServer(ingressKey, (peer) => ingressPeers.has(peer));
keyServer.on("error", (error) => {
  console.error(`console-runtime: ingress key port unavailable (${error?.code || "LISTEN_ERROR"})`);
});
let keyServerStarting = false;

function localAddressFacing(peer) {
  for (const entries of Object.values(networkInterfaces())) {
    for (const entry of entries ?? []) {
      if (entry.family === "IPv4" && !entry.internal && ipv4InSubnet(peer, entry.address, entry.netmask)) {
        return entry.address;
      }
    }
  }
  return "";
}

async function refreshIngressKeyServer() {
  let peers = [];
  try {
    peers = (await lookup(INGRESS_HOSTNAME, { all: true, family: 4 })).map((entry) => entry.address);
  } catch {
    peers = [];
  }
  ingressPeers.clear();
  for (const peer of peers) ingressPeers.add(peer);
  if (keyServer.listening || keyServerStarting || peers.length === 0) return;
  const bindAddress = localAddressFacing(peers[0]);
  if (!bindAddress) return;
  keyServerStarting = true;
  keyServer.once("listening", () => {
    keyServerStarting = false;
  });
  keyServer.once("error", () => {
    keyServerStarting = false;
  });
  keyServer.listen(INGRESS_KEY_PORT, bindAddress);
}

// The ingress starts after this container is healthy, so keep looking for it.
// A slow name lookup must not delay the console start.
void refreshIngressKeyServer();
const ingressRefreshTimer = setInterval(() => {
  void refreshIngressKeyServer();
}, 10_000);
ingressRefreshTimer.unref();

const childArguments = process.argv.slice(2);
if (childArguments.length === 0) childArguments.push(path.resolve("server.js"));

const childEnvironment = { ...process.env, LABTETHER_INGRESS_KEY: ingressKey };
const nextServer = spawn(process.execPath, childArguments, {
  cwd: process.cwd(),
  env: childEnvironment,
  stdio: "inherit",
});

let stopping = false;
let requestedSignal = "SIGTERM";

function stop(signal = "SIGTERM") {
  if (stopping) return;
  stopping = true;
  requestedSignal = signal;
  proxy.close();
  clearInterval(ingressRefreshTimer);
  if (keyServer.listening) keyServer.close();
  if (!nextServer.killed) nextServer.kill(signal);
  setTimeout(() => {
    if (nextServer.exitCode == null && nextServer.signalCode == null) nextServer.kill("SIGKILL");
  }, 10_000).unref();
}

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => stop(signal));
}

nextServer.on("error", (error) => {
  console.error(`console-runtime: failed to start Next server (${error.code || "SPAWN_ERROR"})`);
  stop("SIGTERM");
});

nextServer.on("exit", (code, signal) => {
  const finish = () => {
    if (stopping && signal === requestedSignal) process.exit(0);
    if (signal) process.kill(process.pid, signal);
    process.exit(code ?? 1);
  };
  if (proxy.listening) proxy.close(finish);
  else finish();
});
