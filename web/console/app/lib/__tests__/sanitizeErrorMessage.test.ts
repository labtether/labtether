import { describe, it, expect } from "vitest";
import { sanitizeErrorMessage } from "../sanitizeErrorMessage";

describe("sanitizeErrorMessage", () => {
  it("returns fallback for empty input", () => {
    expect(sanitizeErrorMessage("", "fallback")).toBe("fallback");
    expect(sanitizeErrorMessage("   ", "fallback")).toBe("fallback");
  });

  it("passes through clean messages", () => {
    expect(sanitizeErrorMessage("connection refused", "err")).toBe(
      "connection refused",
    );
  });

  it("redacts secret key-value pairs", () => {
    const msg = 'token_secret: "abc123", password: "hunter2"';
    const result = sanitizeErrorMessage(msg, "err");
    expect(result).not.toContain("abc123");
    expect(result).not.toContain("hunter2");
    expect(result).toContain("[redacted]");
  });

  it("redacts authorization headers", () => {
    const msg = "authorization: Bearer sk-secret-token";
    const result = sanitizeErrorMessage(msg, "err");
    expect(result).not.toContain("sk-secret-token");
    expect(result).toContain("[redacted]");
  });

  it("redacts PVE API tokens", () => {
    const msg = "pveapitoken=user@pam!token=secret-value";
    const result = sanitizeErrorMessage(msg, "err");
    expect(result).not.toContain("secret-value");
  });

  it("redacts URL credentials", () => {
    const msg = "connecting to https://admin:s3cret@host.local/api";
    const result = sanitizeErrorMessage(msg, "err");
    expect(result).not.toContain("s3cret");
  });

  it("redacts custom secrets", () => {
    const msg = "error using key CUSTOM_KEY_123 for auth";
    const result = sanitizeErrorMessage(msg, "err", ["CUSTOM_KEY_123"]);
    expect(result).not.toContain("CUSTOM_KEY_123");
    expect(result).toContain("[redacted]");
  });

  it("ignores empty strings in secrets array", () => {
    const msg = "normal message";
    const result = sanitizeErrorMessage(msg, "err", ["", "  "]);
    expect(result).toBe("normal message");
  });
});
