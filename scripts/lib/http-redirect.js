// Tiny HTTP -> HTTPS redirect server for local development.
// Usage: node http-redirect.js [httpPort] [httpsPort]
const http = require("http");
const httpPort = parseInt(process.argv[2] || "3080", 10);
const httpsPort = parseInt(process.argv[3] || "3000", 10);

http.createServer((req, res) => {
  const host = (req.headers.host || "localhost").replace(":" + httpPort, ":" + httpsPort);
  res.writeHead(301, { Location: "https://" + host + req.url });
  res.end();
}).listen(httpPort, "0.0.0.0", () => {
  console.log("HTTP redirect: http://localhost:" + httpPort + " -> https://localhost:" + httpsPort);
});
