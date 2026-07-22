import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CredentialProfileSelect } from "../CredentialProfileSelect";

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

describe("CredentialProfileSelect", () => {
  it("renders human labels, filters incompatible kinds, and exposes load failures accessibly", async () => {
    await act(async () => {
      root.render(
        <CredentialProfileSelect
          id="credential-picker"
          label="Credential profile"
          value="missing-profile"
          onChange={vi.fn()}
          profiles={[
            { id: "cred_ssh", name: "Lab root", kind: "ssh_password", username: "root", created_at: "", updated_at: "" },
            { id: "cred_rdp", name: "Windows", kind: "rdp_password", created_at: "", updated_at: "" },
          ]}
          loading={false}
          error="Credential inventory unavailable."
          allowedKinds={["ssh_password"]}
        />,
      );
    });

    const options = [...container.querySelectorAll("option")].map((option) => option.textContent);
    expect(options).toContain("Lab root — SSH password (root)");
    expect(options).not.toContain("Windows — RDP password");
    expect(options).toContain("Previously selected profile (unavailable)");
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("inventory unavailable");
    expect(container.querySelector("select")?.getAttribute("aria-describedby")).toBe("credential-picker-status");
  });
});
