import createNextIntlPlugin from 'next-intl/plugin';
import type { NextConfig } from "next";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const withNextIntl = createNextIntlPlugin('./i18n/request.ts');

const currentDir = path.dirname(fileURLToPath(import.meta.url));
const backendAPIBaseURL = (process.env.LABTETHER_API_BASE_URL ?? "http://localhost:8080").replace(/\/+$/, "");

// Next.js rewrite-based WebSocket proxying performs strict TLS hostname
// verification that ignores NODE_TLS_REJECT_UNAUTHORIZED. When the backend
// uses Tailscale (or another cert whose SAN differs from "localhost"), WS
// upgrades fail with ERR_TLS_CERT_ALTNAME_INVALID. Probe the backend's TLS
// info endpoint to discover the actual cert hostname so rewrite destinations
// use a matching origin.
function resolveWSBaseURL(configured: string): string {
  if (process.env.NODE_ENV !== "development") return configured;
  try {
    const parsed = new URL(configured);
    if (parsed.protocol !== "https:" || !["localhost", "127.0.0.1", "::1"].includes(parsed.hostname)) {
      return configured;
    }
    const httpPort = process.env.LABTETHER_HTTP_PORT || "8080";
    const raw = execFileSync("curl", ["-sf", "--max-time", "2", `http://localhost:${httpPort}/api/v1/tls/info`], {
      encoding: "utf8",
      timeout: 3000,
    }).trim();
    const info = JSON.parse(raw) as { cert_dns_names?: string[]; https_port?: number };
    if (info.cert_dns_names?.[0] && info.https_port) {
      const resolved = `https://${info.cert_dns_names[0]}:${info.https_port}`;
      console.log(`next.config: WS rewrite base resolved to ${resolved} (TLS auto-detect)`);
      return resolved;
    }
  } catch {
    // Backend not reachable — fall through to configured URL.
  }
  return configured;
}

const wsBaseURL = resolveWSBaseURL(backendAPIBaseURL);

const nextConfig: NextConfig = {
  output: "standalone",
  transpilePackages: ["@novnc/novnc"],
  images: { formats: ["image/avif", "image/webp"] },
  turbopack: {
    root: currentDir
  },
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "X-DNS-Prefetch-Control", value: "on" },
          { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
          { key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" },
        ],
      },
    ];
  },
  async rewrites() {
    return [
      // Keep browser WebSockets on the frontend origin and proxy upgrade traffic
      // to the hub backend. This avoids a second cert trust prompt for :8443.
      // Uses wsBaseURL which resolves the real TLS hostname in dev to avoid
      // cert mismatch on WebSocket upgrade.
      {
        source: "/desktop/sessions/:path*",
        destination: `${wsBaseURL}/desktop/sessions/:path*`,
      },
      {
        source: "/terminal/sessions/:path*",
        destination: `${wsBaseURL}/terminal/sessions/:path*`,
      },
      {
        source: "/ws/events",
        destination: `${wsBaseURL}/ws/events`,
      },
      {
        source: "/ws/agent",
        destination: `${wsBaseURL}/ws/agent`,
      },
      // Portainer container exec WebSocket — proxied so the browser can
      // upgrade on the same origin as the UI without a separate cert prompt.
      {
        source: "/portainer/assets/:assetId/containers/:containerId/exec",
        destination: `${wsBaseURL}/portainer/assets/:assetId/containers/:containerId/exec`,
      },
    ];
  },
};

export default withNextIntl(nextConfig);
