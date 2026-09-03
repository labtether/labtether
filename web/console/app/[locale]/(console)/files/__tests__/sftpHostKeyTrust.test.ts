import { describe, expect, it } from "vitest";

import {
  sftpEndpointKey,
  trustedSFTPHostKeyForRequest,
  type FileConnection,
  type TestedSFTPHostKey,
} from "../fileConnectionsClient";

const existing: FileConnection = {
  id: "fconn-1",
  name: "Primary SFTP",
  protocol: "sftp",
  host: "files.example.test",
  port: 22,
  initial_path: "/",
  extra_config: { host_key: "ssh-ed25519 saved-key" },
  created_at: "2026-09-03T00:00:00Z",
  updated_at: "2026-09-03T00:00:00Z",
};

describe("SFTP host key trust", () => {
  it("keeps the saved key only for the same endpoint", () => {
    expect(trustedSFTPHostKeyForRequest("sftp", "FILES.EXAMPLE.TEST", 22, existing, null))
      .toBe("ssh-ed25519 saved-key");
    expect(trustedSFTPHostKeyForRequest("sftp", "new.example.test", 22, existing, null))
      .toBeUndefined();
    expect(trustedSFTPHostKeyForRequest("sftp", existing.host, 2222, existing, null))
      .toBeUndefined();
  });

  it("uses a tested key only for the endpoint that produced it", () => {
    const tested: TestedSFTPHostKey = {
      endpoint: sftpEndpointKey("new.example.test", 2222),
      hostKey: "ssh-ed25519 tested-key",
      fingerprint: "SHA256:test",
    };
    expect(trustedSFTPHostKeyForRequest("sftp", "new.example.test", 2222, existing, tested))
      .toBe("ssh-ed25519 tested-key");
    expect(trustedSFTPHostKeyForRequest("sftp", "other.example.test", 2222, existing, tested))
      .toBeUndefined();
  });

  it("never adds an SSH key to another protocol", () => {
    expect(trustedSFTPHostKeyForRequest("ftp", existing.host, 22, existing, null))
      .toBeUndefined();
  });
});
