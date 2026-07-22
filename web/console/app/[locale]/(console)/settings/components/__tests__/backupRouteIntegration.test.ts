import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

describe("backup export production routes", () => {
  it("uses Next API routes for every browser request", () => {
    const source = readFileSync(join(
      process.cwd(),
      "app/[locale]/(console)/settings/components/BackupExportCard.tsx",
    ), "utf8");

    expect(source).toContain('apiFetch("/api/v2/assets")');
    expect(source).toContain("/api/v2/actions?limit=");
    expect(source).toContain('apiFetch("/api/alerts/rules")');
    expect(source).toContain('apiFetch("/api/notifications/channels")');
    expect(source).not.toContain('apiFetch("/alerts/rules")');
    expect(source).not.toContain('apiFetch("/notifications/channels")');
  });
});
