import type { TailscaleServeStatus } from "../../hooks/useTailscaleServeStatus";

export type RemoteAccessWay = "tailscale" | "own";

/**
 * The way chosen before the person picks one: the hub's own address once the hub reports no Tailscale on its host,
 * else Tailscale HTTPS (also while the hub's answer is still loading or could not be read).
 */
export function defaultRemoteAccessWay(status: TailscaleServeStatus | null): RemoteAccessWay {
  return status && !status.tailscale_installed ? "own" : "tailscale";
}
