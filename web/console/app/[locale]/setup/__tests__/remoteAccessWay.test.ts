import { describe, expect, it } from "vitest";

import type { TailscaleServeStatus } from "../../../hooks/useTailscaleServeStatus";
import { defaultRemoteAccessWay } from "../remoteAccessWay";

const status = (extra: Partial<TailscaleServeStatus>) =>
  ({ tailscale_installed: false, logged_in: false, serve_status: "not_installed", serve_configured: false,
    recommendation_state: "", recommendation_message: "", can_manage: false, management_mode: "guided",
    desired_mode: "serve", desired_mode_source: "default", desired_target_source: "default", ...extra }) as TailscaleServeStatus;

// Risk: Finish is off while Tailscale HTTPS is chosen and not set up. On a host without Tailscale the first-run
// setup must open on the hub's own address, or the person cannot finish without finding the other choice.
describe("first-run remote access choice", () => {
  it("opens on the hub's own address when the hub reports no Tailscale", () => {
    expect(defaultRemoteAccessWay(status({ tailscale_installed: false }))).toBe("own");
  });

  it("keeps Tailscale HTTPS when Tailscale is installed or the hub has not answered", () => {
    expect(defaultRemoteAccessWay(status({ tailscale_installed: true, serve_status: "login_required" }))).toBe("tailscale");
    expect(defaultRemoteAccessWay(status({ tailscale_installed: true, logged_in: true, serve_status: "configured", serve_configured: true }))).toBe("tailscale");
    expect(defaultRemoteAccessWay(null)).toBe("tailscale");
  });
});
