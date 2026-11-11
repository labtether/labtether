// "Allow http on my network" for Home Assistant is offered only for an http://
// address on the local network: an RFC 1918 or tailnet (100.64.0.0/10) IPv4
// address, a unique-local IPv6 address (not AWS's fd00:ec2::/32), or a name
// ending in .local or .ts.net. Link-local addresses are not offered. The Hub checks the same rule again and
// resolves a name on every connection, so this only decides what to offer.

function ipv4Octets(host: string): number[] | null {
  const parts = host.split(".");
  if (parts.length !== 4 || !parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255)) return null;
  return parts.map(Number);
}

function isLocalIPv4(octets: number[]): boolean {
  const [a, b] = octets;
  return a === 10
    || (a === 172 && b >= 16 && b <= 31)
    || (a === 192 && b === 168)
    || (a === 100 && b >= 64 && b <= 127);
}

function isLocalIPv6(host: string): boolean {
  if (!host.includes(":")) return false;
  const first = host.split(":")[0] ?? "";
  if (!/^[0-9a-f]{1,4}$/.test(first)) return false;
  const value = parseInt(first.padStart(4, "0"), 16);
  if (/^fd00:0*ec2(:|$)/.test(host)) return false; // cloud metadata
  return (value & 0xfe00) === 0xfc00;
}

export function isLocalNetworkHost(rawHost: string): boolean {
  const host = rawHost.trim().toLowerCase().replace(/^\[|\]$/g, "").replace(/\.$/, "");
  const octets = ipv4Octets(host);
  if (octets) return isLocalIPv4(octets);
  if (isLocalIPv6(host)) return true;
  return [".local", ".ts.net"].some((suffix) => host.endsWith(suffix) && host.length > suffix.length);
}

/** True for an http:// URL whose host is on the local network. */
export function isLocalNetworkHTTPAddress(rawURL: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(rawURL.trim());
  } catch {
    return false;
  }
  return parsed.protocol === "http:" && !parsed.username && !parsed.password && isLocalNetworkHost(parsed.hostname);
}
