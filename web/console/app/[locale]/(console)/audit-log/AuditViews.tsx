"use client";

import { useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import { useAuth } from "../../../contexts/AuthContext";
import { canAccessHostData } from "../../../lib/roles";

/** Log · Recordings: the Audit log's two views, one route each. Recordings only for roles that may open them. */
export function AuditViews({ current }: { current: "log" | "recordings" }) {
  const t = useTranslations("audit-log");
  const { user } = useAuth();
  if (!canAccessHostData(user?.role)) return null;
  const views = [
    { id: "log", href: "/audit-log", label: t("views.log") },
    { id: "recordings", href: "/recordings", label: t("views.recordings") },
  ] as const;
  return (
    <nav className="lg-views" aria-label={t("title")}>
      {views.map((v) => (
        <Link key={v.id} href={v.href} className={v.id === current ? "on" : ""} aria-current={v.id === current ? "page" : undefined}>
          {v.label}
        </Link>
      ))}
    </nav>
  );
}
