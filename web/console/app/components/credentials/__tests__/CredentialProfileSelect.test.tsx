import { act } from "react";
import { NextIntlClientProvider } from "next-intl";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import security from "../../../../messages/de/security.json";
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
  it("renders labels in the person's language, filters incompatible kinds, and exposes load failures accessibly", async () => {
    await act(async () => {
      root.render(
        <NextIntlClientProvider locale="de" messages={{ security }}>
          <CredentialProfileSelect
            id="credential-picker"
            label="Gespeicherte Anmeldung"
            value="missing-profile"
            onChange={vi.fn()}
            profiles={[
              { id: "cred_ssh", name: "Lab root", kind: "ssh_password", username: "root", created_at: "", updated_at: "" },
              { id: "cred_rdp", name: "Windows", kind: "rdp_password", created_at: "", updated_at: "" },
            ]}
            loading={false}
            error="Failed to load credential profiles (500)."
            allowedKinds={["ssh_password"]}
          />
        </NextIntlClientProvider>,
      );
    });

    const options = [...container.querySelectorAll("option")].map((option) => option.textContent);
    expect(options).toEqual(["Keine gespeicherte Anmeldung", "Bisherige Anmeldung (nicht verfügbar)", "Lab root · SSH-Passwort · root"]);
    expect(container.querySelector('[role="alert"]')?.textContent).toBe("Gespeicherte Anmeldungen konnten nicht geladen werden");
    expect(container.querySelector("select")?.getAttribute("aria-describedby")).toBe("credential-picker-status");
  });
});
