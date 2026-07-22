import createNextIntlPlugin from 'next-intl/plugin';
import type { NextConfig } from "next";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { consoleSecurityHeaders } from "./lib/securityHeaders";

const withNextIntl = createNextIntlPlugin('./i18n/request.ts');

const currentDir = path.dirname(fileURLToPath(import.meta.url));
const isDevelopment = process.env.NODE_ENV === "development";
const runtimeProxyBaseURL = "http://127.0.0.1:3011";

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
        headers: consoleSecurityHeaders(isDevelopment),
      },
    ];
  },
  async rewrites() {
    return [
      // Keep browser WebSockets on the frontend origin and proxy upgrade traffic
      // through the loopback-only runtime proxy. The proxy resolves and verifies
      // LABTETHER_API_BASE_URL when the container starts, so standalone builds
      // do not bake one deployment's backend address into their rewrites.
      {
        source: "/desktop/sessions/:path*",
        destination: `${runtimeProxyBaseURL}/desktop/sessions/:path*`,
      },
      {
        source: "/terminal/sessions/:path*",
        destination: `${runtimeProxyBaseURL}/terminal/sessions/:path*`,
      },
      {
        source: "/ws/events",
        destination: `${runtimeProxyBaseURL}/ws/events`,
      },
      {
        source: "/ws/agent",
        destination: `${runtimeProxyBaseURL}/ws/agent`,
      },
      // Portainer container exec WebSocket — proxied so the browser can
      // upgrade on the same origin as the UI without a separate cert prompt.
      {
        source: "/portainer/assets/:assetId/containers/:containerId/exec",
        destination: `${runtimeProxyBaseURL}/portainer/assets/:assetId/containers/:containerId/exec`,
      },
    ];
  },
};

export default withNextIntl(nextConfig);
