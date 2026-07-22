import { spawn } from "node:child_process";
import { once } from "node:events";
import path from "node:path";
import {
  createRuntimeProxyServer,
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

const childArguments = process.argv.slice(2);
if (childArguments.length === 0) childArguments.push(path.resolve("server.js"));

const nextServer = spawn(process.execPath, childArguments, {
  cwd: process.cwd(),
  env: process.env,
  stdio: "inherit",
});

let stopping = false;
let requestedSignal = "SIGTERM";

function stop(signal = "SIGTERM") {
  if (stopping) return;
  stopping = true;
  requestedSignal = signal;
  proxy.close();
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
