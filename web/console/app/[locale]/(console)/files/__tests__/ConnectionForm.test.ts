import { describe, expect, it } from "vitest";

import { ftpTLSFromExtraConfig } from "../ConnectionForm";

describe("ConnectionForm FTP defaults", () => {
  it("defaults new and malformed legacy FTP settings to TLS", () => {
    expect(ftpTLSFromExtraConfig({})).toBe(true);
    expect(ftpTLSFromExtraConfig({ ftp_tls: "false" })).toBe(true);
  });

  it("reads explicit current and legacy TLS choices", () => {
    expect(ftpTLSFromExtraConfig({ ftp_tls: false })).toBe(false);
    expect(ftpTLSFromExtraConfig({ use_tls: false })).toBe(false);
    expect(ftpTLSFromExtraConfig({ ftp_tls: true, use_tls: false })).toBe(true);
  });
});
