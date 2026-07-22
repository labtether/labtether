import { describe, expect, it, vi } from "vitest";

// These tests exercise the exported command builders only. Isolate them from
// client providers so importing the component does not bootstrap Next routing.
vi.mock("../../../contexts/StatusContext", () => ({ useFastStatus: vi.fn(() => null) }));
vi.mock("../../../contexts/ToastContext", () => ({ useToast: vi.fn(() => ({ addToast: vi.fn() })) }));
vi.mock("../../../hooks/useEnrollment", () => ({ useEnrollment: vi.fn() }));

import {
  linuxInstallerCommand,
  manualInstallCommand,
  type LinuxInstallOptions,
} from "../AgentSetupStep";

const options: LinuxInstallOptions = {
  dockerEnabled: "auto",
  dockerEndpoint: "/var/run/docker.sock",
  dockerDiscoveryIntervalSec: "30",
  filesRootMode: "home",
  autoInstallVNC: true,
  autoUpdateEnabled: true,
  forceUpdate: false,
  includeEnrollmentToken: true,
};

describe("agent setup commands", () => {
  it("prompts into a private token file instead of putting a token in argv", () => {
    const command = linuxInstallerCommand("https://hub.example.com", options);

    expect(command).toContain("read -r -s");
    expect(command).toContain("--enrollment-token-file");
    expect(command).not.toContain("--enrollment-token '");
    expect(command).not.toContain("LABTETHER_ENROLLMENT_TOKEN=");
  });

  it("verifies the pinned CA before downloading or executing installer content", () => {
    const fingerprint = "A".repeat(64);
    const bootstrapURL = `https://hub.example.com/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=${fingerprint}`;
    const command = linuxInstallerCommand("https://hub.example.com", options, {
      kind: "lan",
      label: "LAN",
      host: "hub.example.com",
      hub_url: "https://hub.example.com",
      ws_url: "wss://hub.example.com/ws/agent",
      bootstrap_strategy: "pinned_ca_bootstrap",
      bootstrap_url: bootstrapURL,
    });

    expect(command).toContain(
      "curl -kfsSL 'https://hub.example.com/api/v1/ca.crt' -o \"$ca_file\"",
    );
    expect(command).toContain(
      `expected_ca_fingerprint='${fingerprint.toLowerCase()}'`,
    );
    expect(command).toContain(
      'curl --cacert "$ca_file" -fsSL \'https://hub.example.com/install.sh\' -o "$installer"',
    );
    expect(command).toContain("--tls-ca-file '/etc/labtether/ca.crt'");
    expect(command).not.toContain(bootstrapURL);
    expect(command.indexOf("CA fingerprint mismatch")).toBeLessThan(
      command.indexOf("sudo install -d"),
    );
    expect(command.indexOf("curl --cacert")).toBeLessThan(
      command.indexOf('sudo bash "$installer"'),
    );
  });

  it("fails closed to normal TLS when pinned bootstrap metadata is invalid", () => {
    const command = linuxInstallerCommand("https://hub.example.com", options, {
      kind: "lan",
      label: "LAN",
      host: "hub.example.com",
      hub_url: "https://hub.example.com",
      ws_url: "wss://hub.example.com/ws/agent",
      bootstrap_strategy: "pinned_ca_bootstrap",
      bootstrap_url: `https://attacker.example/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=${"a".repeat(64)}`,
    });

    expect(command).toContain(
      "curl -fsSL 'https://hub.example.com/install.sh' -o \"$installer\"",
    );
    expect(command).not.toContain("curl -k");
    expect(command).not.toContain("attacker.example");
  });

  it.each(["linux", "macos", "windows"] as const)(
    "uses a file-backed token for the %s manual command",
    (platform) => {
      const command = manualInstallCommand(
        platform,
        "wss://hub.example.com/ws/agent",
      );

      expect(command).toContain("LABTETHER_ENROLLMENT_TOKEN_FILE");
      expect(command).not.toContain("LABTETHER_ENROLLMENT_TOKEN=");
    },
  );

  it("quotes untrusted WebSocket URLs in the Windows command", () => {
    const command = manualInstallCommand(
      "windows",
      "wss://hub.example/it's/$([Console]::WriteLine('injected'))",
    );

    expect(command).toContain(
      "$env:LABTETHER_WS_URL='wss://hub.example/it''s/$([Console]::WriteLine(''injected''))'",
    );
    expect(command).not.toContain('$env:LABTETHER_WS_URL="');
  });
});
