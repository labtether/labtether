import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { createServer as createHttpsServer } from "node:https";
import { request as createHttpRequest } from "node:http";
import { createConnection } from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

const currentDir = path.dirname(fileURLToPath(import.meta.url));
const consoleRoot = path.resolve(currentDir, "..");
const httpHost = process.env.PLAYWRIGHT_HTTP_HOST ?? "127.0.0.1";
const httpPort = Number(process.env.PLAYWRIGHT_HTTP_PORT ?? "4174");
const httpsHost = process.env.PLAYWRIGHT_HTTPS_HOST ?? "127.0.0.1";
const httpsPort = Number(process.env.PLAYWRIGHT_HTTPS_PORT ?? "4173");
const certPath = process.env.PLAYWRIGHT_TLS_CERT_PATH
  ?? path.join(consoleRoot, "e2e", "certs", "playwright-selfsigned-cert.pem");
const keyPath = process.env.PLAYWRIGHT_TLS_KEY_PATH
  ?? path.join(consoleRoot, "e2e", "certs", "playwright-selfsigned-key.pem");

function ensureTLSMaterial() {
  if (existsSync(certPath) && existsSync(keyPath)) {
    return;
  }
  mkdirSync(path.dirname(certPath), { recursive: true });
  const openssl = spawn(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      keyPath,
      "-out",
      certPath,
      "-days",
      "7",
      "-subj",
      `/CN=${httpsHost}`,
      "-addext",
      `subjectAltName=DNS:${httpsHost},IP:127.0.0.1`,
    ],
    { stdio: "inherit" },
  );
  openssl.on("exit", (code) => {
    if ((code ?? 1) !== 0) {
      console.error("failed to generate Playwright TLS material with openssl");
      process.exit(code ?? 1);
    }
    startProxy();
  });
  openssl.on("error", (error) => {
    console.error(`failed to launch openssl for Playwright TLS material: ${error.message}`);
    process.exit(1);
  });
}

const standalone = spawn(
  process.execPath,
  [path.join(".next", "standalone", "server.js")],
  {
    cwd: consoleRoot,
    env: {
      ...process.env,
      HOSTNAME: httpHost,
      PORT: String(httpPort),
    },
    stdio: "inherit",
  },
);

let shuttingDown = false;

function shutdown(code = 0) {
  if (shuttingDown) {
    return;
  }
  shuttingDown = true;
  if (server) {
    server.close(() => {
      if (!standalone.killed) {
        standalone.kill("SIGTERM");
      }
    });
  } else if (!standalone.killed) {
    standalone.kill("SIGTERM");
  }
  setTimeout(() => process.exit(code), 250).unref();
}

function forwardRequest(clientRequest, clientResponse) {
  const forwardedHost = clientRequest.headers.host ?? `${httpsHost}:${httpsPort}`;
  const upstreamRequest = createHttpRequest(
    {
      hostname: httpHost,
      port: httpPort,
      method: clientRequest.method,
      path: clientRequest.url,
      headers: {
        ...clientRequest.headers,
        host: `${httpHost}:${httpPort}`,
        "x-forwarded-host": forwardedHost,
        "x-forwarded-proto": "https",
      },
    },
    (upstreamResponse) => {
      clientResponse.writeHead(
        upstreamResponse.statusCode ?? 502,
        upstreamResponse.statusMessage,
        upstreamResponse.headers,
      );
      upstreamResponse.pipe(clientResponse);
    },
  );

  upstreamRequest.on("error", (error) => {
    if (!clientResponse.headersSent) {
      clientResponse.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    }
    clientResponse.end(`playwright https proxy error: ${error.message}`);
  });

  clientRequest.pipe(upstreamRequest);
}

function formatUpgradeHeaders(request) {
  const forwardedHost = request.headers.host ?? `${httpsHost}:${httpsPort}`;
  let headerLines = `${request.method} ${request.url} HTTP/${request.httpVersion}\r\n`;
  for (const [headerName, headerValue] of Object.entries(request.headers)) {
    if (headerName.toLowerCase() === "host" || headerValue == null) {
      continue;
    }
    if (Array.isArray(headerValue)) {
      for (const value of headerValue) {
        headerLines += `${headerName}: ${value}\r\n`;
      }
      continue;
    }
    headerLines += `${headerName}: ${headerValue}\r\n`;
  }
  headerLines += `host: ${httpHost}:${httpPort}\r\n`;
  headerLines += `x-forwarded-host: ${forwardedHost}\r\n`;
  headerLines += "x-forwarded-proto: https\r\n\r\n";
  return headerLines;
}

let server;

function startProxy() {
  server = createHttpsServer(
    {
      cert: readFileSync(certPath),
      key: readFileSync(keyPath),
    },
    forwardRequest,
  );

  server.on("upgrade", (request, socket, head) => {
    const upstreamSocket = createConnection(httpPort, httpHost, () => {
      upstreamSocket.write(formatUpgradeHeaders(request));
      if (head.length > 0) {
        upstreamSocket.write(head);
      }
      socket.pipe(upstreamSocket).pipe(socket);
    });

    upstreamSocket.on("error", () => {
      socket.destroy();
    });

    socket.on("error", () => {
      upstreamSocket.destroy();
    });
  });

  server.listen(httpsPort, httpsHost, () => {
    console.log(
      `Playwright self-signed HTTPS proxy listening on https://${httpsHost}:${httpsPort} -> http://${httpHost}:${httpPort}`,
    );
  });
}

standalone.on("exit", (code, signal) => {
  if (shuttingDown) {
    process.exit(code ?? 0);
    return;
  }
  if (signal) {
    console.error(`Standalone Next server exited from signal ${signal}`);
    shutdown(1);
    return;
  }
  if ((code ?? 0) !== 0) {
    console.error(`Standalone Next server exited with code ${code}`);
  }
  shutdown(code ?? 0);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => shutdown(0));
}

ensureTLSMaterial();
