import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import { createServer } from "node:http";
import { connect, createServer as createTcpServer } from "node:net";
import path from "node:path";

const root = path.resolve(import.meta.dirname, "..");
const standaloneServer = path.join(root, ".next", "standalone", "server.js");
if (!existsSync(standaloneServer)) {
  throw new Error("standalone Next server is missing; run npm run build first");
}

function listen(server) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => resolve(server.address().port));
  });
}

function close(server) {
  return new Promise((resolve) => server.close(resolve));
}

async function reservePort() {
  const server = createTcpServer();
  const port = await listen(server);
  await close(server);
  return port;
}

function websocketRequest(port) {
  return new Promise((resolve, reject) => {
    const socket = connect(port, "127.0.0.1");
    let response = "";
    socket.setEncoding("utf8");
    socket.setTimeout(3_000, () => socket.destroy(new Error("upgrade response timed out")));
    socket.once("connect", () => {
      socket.write(
        "GET /ws/events?ticket=standalone-secret HTTP/1.1\r\n"
        + "Host: console.example\r\n"
        + "Connection: Upgrade\r\n"
        + "Upgrade: websocket\r\n"
        + "Sec-WebSocket-Version: 13\r\n"
        + "Sec-WebSocket-Key: c3RhbmRhbG9uZS10ZXN0\r\n" // gitleaks:allow -- deterministic public WebSocket test nonce
        + "Cookie: labtether_session=standalone-session\r\n"
        + "Origin: https://console.example\r\n\r\n",
      );
    });
    socket.on("data", (chunk) => {
      response += chunk;
      if (response.includes("\r\n\r\n")) socket.end();
    });
    socket.once("end", () => resolve(response));
    socket.once("error", reject);
  });
}

async function waitForUpgrade(port, child, logs) {
  const deadline = Date.now() + 30_000;
  let lastError;
  while (Date.now() < deadline) {
    if (child.exitCode != null || child.signalCode != null) {
      throw new Error(`standalone console exited before the probe:\n${logs.join("")}`);
    }
    try {
      const response = await websocketRequest(port);
      if (response.includes("101 Switching Protocols")) return response;
      lastError = new Error(`unexpected response: ${response.split("\r\n", 1)[0]}`);
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`standalone upgrade probe failed: ${lastError?.message ?? "timeout"}\n${logs.join("")}`);
}

let observedUpgrade;
const hub = createServer((_request, response) => {
  response.writeHead(404);
  response.end();
});
hub.on("upgrade", (request, socket) => {
  observedUpgrade = {
    url: request.url,
    cookie: request.headers.cookie,
    origin: request.headers.origin,
    forwardedHost: request.headers["x-forwarded-host"],
  };
  const key = String(request.headers["sec-websocket-key"] ?? "");
  const accept = createHash("sha1")
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");
  socket.end(
    "HTTP/1.1 101 Switching Protocols\r\n"
    + "Connection: Upgrade\r\n"
    + "Upgrade: websocket\r\n"
    + `Sec-WebSocket-Accept: ${accept}\r\n\r\n`,
  );
});

const hubPort = await listen(hub);
const consolePort = await reservePort();
const logs = [];
const child = spawn(
  process.execPath,
  [path.join(root, "scripts", "console-runtime.mjs"), standaloneServer],
  {
    cwd: root,
    env: {
      ...process.env,
      HOSTNAME: "127.0.0.1",
      PORT: String(consolePort),
      LABTETHER_API_BASE_URL: `http://127.0.0.1:${hubPort}`,
    },
    stdio: ["ignore", "pipe", "pipe"],
  },
);
child.stdout.on("data", (chunk) => logs.push(chunk.toString()));
child.stderr.on("data", (chunk) => logs.push(chunk.toString()));

try {
  await waitForUpgrade(consolePort, child, logs);
  const expected = {
    url: "/ws/events?ticket=standalone-secret",
    cookie: "labtether_session=standalone-session",
    origin: "https://console.example",
    forwardedHost: "console.example",
  };
  if (JSON.stringify(observedUpgrade) !== JSON.stringify(expected)) {
    throw new Error(`standalone proxy changed protected upgrade data: ${JSON.stringify(observedUpgrade)}`);
  }
  console.log("standalone runtime proxy upgrade: pass");
} finally {
  child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    new Promise((resolve) => setTimeout(resolve, 5_000)),
  ]);
  if (child.exitCode == null && child.signalCode == null) child.kill("SIGKILL");
  await close(hub);
}
