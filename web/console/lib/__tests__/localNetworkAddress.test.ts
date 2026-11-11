import { describe, expect, it } from "vitest";

import { isLocalNetworkHTTPAddress } from "../localNetworkAddress";

// The switch may be offered only for plain http to the local network; the Hub
// enforces the same rule, so a public or loopback address never gets it.
describe("isLocalNetworkHTTPAddress", () => {
  it.each([
    "http://192.168.1.20:8123", "http://10.0.0.5:8123", "http://172.20.1.1:8123", "http://100.101.102.103:8123",
    "http://[fd7a:115c:a1e0::1]:8123",
    "http://homeassistant.local:8123", "http://ha.tail1234.ts.net:8123",
  ])("offers the switch for %s", (url) => {
    expect(isLocalNetworkHTTPAddress(url)).toBe(true);
  });

  it.each([
    "https://192.168.1.20:8123", "http://8.8.8.8:8123", "http://127.0.0.1:8123", "http://172.32.0.1",
    "http://100.128.0.1", "http://169.254.10.1:8123", "http://169.254.169.254", "http://[fe80::1]:8123",
    "http://[fd00:ec2::254]", "http://example.com", "http://user:pw@192.168.1.20", "http://[::1]:8123", "not a url",
  ])("does not offer it for %s", (url) => {
    expect(isLocalNetworkHTTPAddress(url)).toBe(false);
  });
});
