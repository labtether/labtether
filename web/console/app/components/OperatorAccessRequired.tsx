"use client";

import { useTranslations } from "next-intl";

export type OperatorAccessFeature = "terminal" | "remoteView" | "files" | "recordings" | "hostData";

// Shown to viewers in place of features the Hub reserves for operators
// (see canAccessHostData in app/lib/roles.ts). 2.0: an empty-state sheet, the name and one plain line.
export function OperatorAccessRequired({ title, feature }: { title: string; feature: OperatorAccessFeature }) {
  const t = useTranslations("common.operatorAccess");
  return (
    <div className="kt-sheet kt-empty">
      <h1 className="kt-empty-t">{title}</h1>
      <p className="kt-empty-d">{t(feature)}</p>
    </div>
  );
}
