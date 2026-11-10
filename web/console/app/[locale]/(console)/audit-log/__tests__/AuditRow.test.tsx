import { NextIntlClientProvider, useTranslations } from "next-intl";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import en from "../../../../../messages/en/audit-log.json";
import { AuditRow, type AuditText } from "../AuditRow";
import { actionSentence, alertDevices, auditActor, hiddenIDs, type AuditEntry } from "../auditModel";

// Risk: an alert row in the Audit log does not say which device it was about, or the names of several devices
// cannot be read anywhere (hover on the count, or the row's details).
let root: Root;
let container: HTMLDivElement;

function Row({ entry, open = false }: { entry: AuditEntry; open?: boolean }) {
  const t = useTranslations("audit-log");
  const text: AuditText = {
    time: () => "22:41",
    who: () => "alex",
    action: (e) => t(`actions.${actionSentence(e).key}`, actionSentence(e).values),
    where: (e) => {
      const devices = alertDevices(e);
      if (!devices) return "";
      return "name" in devices ? t("where.device", { name: devices.name }) : t("where.devices", { count: devices.count });
    },
    ids: (e) => hiddenIDs(e, auditActor(e.actor_id, new Map())),
  };
  return <AuditRow entry={entry} text={text} open={open} onToggle={() => {}} onCopy={() => {}} />;
}

const alert = (details: Record<string, unknown>): AuditEntry => ({
  id: "a1",
  type: "alert.acknowledged",
  actor_id: "usr-1",
  target: "rule-1",
  timestamp: "2026-10-27T22:41:00Z",
  details: { target_name: "Disk above 85%", ...details },
});

async function show(entry: AuditEntry, open = false) {
  await act(async () => {
    root.render(
      <NextIntlClientProvider locale="en" messages={{ "audit-log": en }}>
        <Row entry={entry} open={open} />
      </NextIntlClientProvider>,
    );
  });
  return container.querySelector(".lg-au-what")?.textContent ?? "";
}

beforeEach(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("AuditRow, IDs", () => {
  it("leaves a Hub-made ID out of the row and shows it in the details under a word", async () => {
    const entry: AuditEntry = {
      id: "s1", type: "schedule.deleted", actor_id: "usr-1", target: "sched_1000000000000000004_4", timestamp: "2026-10-27T22:41:00Z",
    };
    expect(await show(entry, true)).toBe("Deleted a schedule");
    expect(container.querySelector(".lg-au-d")?.textContent).toBe("Object IDsched_1000000000000000004_4");
  });
});

describe("AuditRow, alert rows", () => {
  it("names the one device after the rule", async () => {
    expect(await show(alert({ device_name: "nas01", device_id: "dev-1" }))).toBe("Acknowledged an alertDisk above 85%on nas01");
  });

  it("counts several devices and keeps their names on hover and in the details", async () => {
    const entry = alert({ device_count: 3, device_ids: ["a", "b", "c"], device_names: ["nas01", "nas02", "pve01"] });
    expect(await show(entry, true)).toBe("Acknowledged an alertDisk above 85%on 3 devices");
    expect(container.querySelector(".lg-au-wh")?.getAttribute("title")).toBe("nas01, nas02, pve01");
    expect(container.querySelector(".lg-au-d")?.textContent).toContain("nas01, nas02, pve01");
  });

  it("adds nothing for a rule that is not about a device", async () => {
    expect(await show(alert({}))).toBe("Acknowledged an alertDisk above 85%");
    expect(container.querySelector(".lg-au-wh")).toBeNull();
  });
});
