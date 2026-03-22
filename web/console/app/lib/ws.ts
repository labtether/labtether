/**
 * Build a WebSocket URL from a stream path on the current frontend origin.
 *
 * In dev/prod, Next.js rewrites proxy desktop/terminal/events websocket paths
 * to the backend. That keeps all browser WS connections on the same origin
 * as the UI and avoids separate backend-origin cert trust prompts.
 */
type BuildBrowserWsOptions = {
  secure?: boolean;
};

export function buildBrowserWsUrl(streamPath: string, options?: BuildBrowserWsOptions): string {
  const loc = window.location;
  const secure = options?.secure ?? loc.protocol === "https:";
  const wsProtocol = secure ? "wss:" : "ws:";
  const host = loc.host; // includes port when present
  const path = streamPath.startsWith("/") ? streamPath : `/${streamPath}`;
  return `${wsProtocol}//${host}${path}`;
}
