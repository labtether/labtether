"use client";

import { useTranslations } from "next-intl";

export type OperatorAccessFeature = "terminal" | "remoteView" | "files" | "recordings" | "hostData";

// Shown to viewers in place of features the Hub reserves for operators
// (see canAccessHostData in app/lib/roles.ts).
export function OperatorAccessRequired({ title, feature }: { title: string; feature: OperatorAccessFeature }) {
  const t = useTranslations("common.operatorAccess");
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--panel-glass)] p-6">
      <h1 className="text-base font-medium text-[var(--text)]">{title}</h1>
      <p className="mt-2 text-sm text-[var(--muted)]">{t(feature)}</p>
    </div>
  );
}
