import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AgentSettingInputControl } from "../AgentSettingInputControl";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

describe("AgentSettingInputControl", () => {
  it.each([
    ["bool", "select"],
    ["enum", "select"],
    ["int", "input"],
    ["string", "input"],
  ] as const)("names an editable %s control with the visible setting label", async (type, selector) => {
    const onChange = vi.fn();
    await act(async () => {
      root.render(
        <AgentSettingInputControl
          setting={{
            key: "collect_interval_sec",
            label: "Collect Interval",
            type,
            min_int: type === "int" ? 1 : undefined,
            max_int: type === "int" ? 300 : undefined,
            allowed_values: type === "enum" ? ["info", "debug"] : undefined,
            sensitive: false,
            configured: false,
          }}
          currentValue={type === "bool" ? "true" : type === "enum" ? "info" : type === "int" ? "10" : "value"}
          editable
          onChange={onChange}
        />,
      );
    });

    expect(container.querySelector(`${selector}[aria-label="Collect Interval"]`)).not.toBeNull();
  });

  it("names a read-only sensitive setting without exposing it as plain text", async () => {
    await act(async () => {
      root.render(
        <AgentSettingInputControl
          setting={{
            key: "webrtc_turn_pass",
            label: "WebRTC TURN Password",
            type: "string",
            sensitive: true,
            configured: true,
          }}
          currentValue=""
          editable={false}
          onChange={vi.fn()}
        />,
      );
    });

    const input = container.querySelector('input[aria-label="WebRTC TURN Password"]');
    expect(input?.getAttribute("type")).toBe("password");
  });
});
