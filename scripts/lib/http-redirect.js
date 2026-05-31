// Tiny HTTP -> HTTPS redirect server for local development.
// Usage: node http-redirect.js [httpPort] [httpsPort]
const http = require("http");

const DEFAULT_HTTP_PORT = 3080;
const DEFAULT_HTTPS_PORT = 3000;

function parsePortArg(value, name, defaultPort) {
  const raw = value ?? String(defaultPort);
  if (!/^[0-9]+$/.test(raw)) {
    throw new Error(`${name} must be an integer port from 1 to 65535`);
  }

  const port = Number(raw);
  if (!Number.isSafeInteger(port) || port < 1 || port > 65535) {
    throw new Error(`${name} must be an integer port from 1 to 65535`);
  }

  return port;
}

function parsePorts(argv = process.argv) {
  return {
    httpPort: parsePortArg(argv[2], "httpPort", DEFAULT_HTTP_PORT),
    httpsPort: parsePortArg(argv[3], "httpsPort", DEFAULT_HTTPS_PORT),
  };
}

function buildRedirectLocation(hostHeader, url, httpPort, httpsPort) {
  const host = (hostHeader || "localhost").replace(":" + httpPort, ":" + httpsPort);
  return "https://" + host + (url || "/");
}

function startRedirectServer(httpPort, httpsPort) {
  return http.createServer((req, res) => {
    res.writeHead(301, {
      Location: buildRedirectLocation(req.headers.host, req.url, httpPort, httpsPort),
    });
    res.end();
  }).listen(httpPort, "0.0.0.0", () => {
    console.log("HTTP redirect: http://localhost:" + httpPort + " -> https://localhost:" + httpsPort);
  });
}

function main() {
  let ports;
  try {
    ports = parsePorts();
  } catch (error) {
    console.error(error.message);
    process.exit(2);
  }

  startRedirectServer(ports.httpPort, ports.httpsPort);
}

if (require.main === module) {
  main();
}

module.exports = {
  buildRedirectLocation,
  parsePortArg,
  parsePorts,
  startRedirectServer,
};
