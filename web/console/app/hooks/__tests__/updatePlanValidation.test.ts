import { describe, expect, it } from "vitest";

import { EXECUTABLE_UPDATE_SCOPES, parseUpdatePlanInput } from "../updatePlanValidation";

describe("parseUpdatePlanInput", () => {
  it("normalizes and deduplicates asset-bound package updates", () => {
    expect(parseUpdatePlanInput(" asset-a,asset-b,asset-a ", " OS_PACKAGES,os_packages ")).toEqual({
      targets: ["asset-a", "asset-b"],
      scopes: ["os_packages"],
    });
  });

  it("defaults an empty scope field to the executable scope", () => {
    expect(parseUpdatePlanInput("asset-a", "").scopes).toEqual([...EXECUTABLE_UPDATE_SCOPES]);
  });

  it("rejects targetless plans that could only fail at runtime", () => {
    expect(() => parseUpdatePlanInput(" ", "os_packages")).toThrow("At least one device");
  });

  it("rejects roadmap scopes that the hub cannot execute", () => {
    expect(() => parseUpdatePlanInput("asset-a", "docker_images")).toThrow("Unsupported update scope");
  });
});
