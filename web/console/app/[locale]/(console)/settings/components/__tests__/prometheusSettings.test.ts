import { describe, expect, it } from "vitest";

import {
  PROMETHEUS_SETTING_KEYS as KEYS,
  buildPrometheusPatch,
  buildPrometheusSettingsState,
  buildPrometheusTestRequest,
} from "../prometheusSettings";

describe("Prometheus secret settings", () => {
  it("never hydrates a password even if an older backend returns one", () => {
    const state = buildPrometheusSettingsState([
      {
        key: KEYS.remoteWritePassword,
        effective_value: "backend-must-not-send-this",
        sensitive: true,
        configured: true,
      },
      { key: KEYS.remoteWriteUsername, effective_value: "alice" },
    ]);

    expect(state.values[KEYS.remoteWritePassword]).toBe("");
    expect(JSON.stringify(state)).not.toContain("backend-must-not-send-this");
    expect(state.passwordConfigured).toBe(true);
  });

	it("treats an empty password as unchanged and preserves explicit password bytes", () => {
    const current = {
      [KEYS.remoteWriteURL]: "https://metrics.example.test/api/v1/write",
      [KEYS.remoteWriteUsername]: "alice",
      [KEYS.remoteWritePassword]: "",
    };
	const patch = buildPrometheusPatch(
	  { ...current, [KEYS.remoteWriteUsername]: "bob", [KEYS.remoteWritePassword]: "" },
      current,
    );

    expect(patch).toEqual({ [KEYS.remoteWriteUsername]: "bob" });
	expect(patch).not.toHaveProperty(KEYS.remoteWritePassword);

	const exact = buildPrometheusPatch(
	  { ...current, [KEYS.remoteWritePassword]: "  exact secret bytes  " },
	  current,
	);
	expect(exact[KEYS.remoteWritePassword]).toBe("  exact secret bytes  ");
  });

  it("requests scoped stored-password reuse without putting a secret in the body", () => {
    const request = buildPrometheusTestRequest(
      {
        [KEYS.remoteWriteURL]: "https://metrics.example.test/api/v1/write",
        [KEYS.remoteWriteUsername]: "alice",
        [KEYS.remoteWritePassword]: "",
      },
      true,
    );

    expect(request).toEqual({
      url: "https://metrics.example.test/api/v1/write",
      username: "alice",
      password: "",
      use_stored_password: true,
    });
  });

	it("uses an explicitly entered replacement instead of requesting stored reuse", () => {
    const request = buildPrometheusTestRequest(
      {
        [KEYS.remoteWriteURL]: "https://other.example.test/write",
        [KEYS.remoteWriteUsername]: "alice",
		[KEYS.remoteWritePassword]: "  replacement  ",
      },
      true,
    );

    expect(request.use_stored_password).toBe(false);
	expect(request.password).toBe("  replacement  ");
  });
});
