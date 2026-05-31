const assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const path = require("node:path");
const test = require("node:test");

const {
  buildRedirectLocation,
  parsePortArg,
  parsePorts,
} = require("./http-redirect");

test("parsePorts uses dev defaults when no ports are supplied", () => {
  assert.deepEqual(parsePorts(["node", "http-redirect.js"]), {
    httpPort: 3080,
    httpsPort: 3000,
  });
});

test("parsePortArg rejects malformed or unsafe port values", () => {
  for (const value of ["30abc", "1e3", "0", "65536", "-1", "", " 3080"]) {
    assert.throws(
      () => parsePortArg(value, "httpPort", 3080),
      /httpPort must be an integer port from 1 to 65535/,
    );
  }
});

test("buildRedirectLocation rewrites the configured port only", () => {
  assert.equal(
    buildRedirectLocation("localhost:3080", "/console", 3080, 3000),
    "https://localhost:3000/console",
  );
  assert.equal(
    buildRedirectLocation("lab.example.test", "/console", 3080, 3000),
    "https://lab.example.test/console",
  );
});

test("cli exits before listening when the port is malformed", () => {
  const scriptPath = path.join(__dirname, "http-redirect.js");
  const result = spawnSync(process.execPath, [scriptPath, "30abc", "3000"], {
    encoding: "utf8",
  });

  assert.equal(result.status, 2);
  assert.match(result.stderr, /httpPort must be an integer port from 1 to 65535/);
});
