"use client";

import { useMemo, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { usePathname } from "../../../i18n/navigation";
import { sectionFor } from "../../lib/navigation";
import { PageSectionContext } from "../pageSection";
import { SectionTabs } from "./SectionTabs";

/** Tells PageHeader which section the current page belongs to, so it can show the section's title and tabs. */
export function PageSectionProvider({ children }: { children: ReactNode }) {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const section = sectionFor(pathname);
  const value = useMemo(() => (section ? { title: t(section.translationKey), tabs: <SectionTabs /> } : null), [section, t]);
  return <PageSectionContext.Provider value={value}>{children}</PageSectionContext.Provider>;
}
