import { describe, expect, it } from "vitest";
import { validatePublicContentLength } from "../publicHubProxy";

describe("validatePublicContentLength", () => {
  it("allows missing and in-limit content lengths", () => {
    expect(validatePublicContentLength(null)).toEqual({ ok: true });
    expect(validatePublicContentLength("")).toEqual({ ok: true });
    expect(validatePublicContentLength("1048576")).toEqual({ ok: true });
  });

  it("rejects malformed decimal lengths", () => {
    expect(validatePublicContentLength("1e3")).toEqual({
      ok: false,
      status: 400,
      message: "invalid content-length",
    });
    expect(validatePublicContentLength("30abc")).toEqual({
      ok: false,
      status: 400,
      message: "invalid content-length",
    });
    expect(validatePublicContentLength("-1")).toEqual({
      ok: false,
      status: 400,
      message: "invalid content-length",
    });
  });

  it("rejects oversized declared bodies before reading", () => {
    expect(validatePublicContentLength("1048577")).toEqual({
      ok: false,
      status: 413,
      message: "request body too large",
    });
    expect(validatePublicContentLength("999999999999999999999999")).toEqual({
      ok: false,
      status: 413,
      message: "request body too large",
    });
  });
});
