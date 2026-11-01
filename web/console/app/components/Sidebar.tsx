"use client";

import { memo } from "react";
import { useTranslations } from "next-intl";
import { SidebarNav } from "./shell/SidebarNav";

/** The desktop sidebar: a floating panel on the canvas. Under 900 px it folds into the drawer (MobileNav). */
export const Sidebar = memo(function Sidebar() {
  const t = useTranslations("nav");
  return (
    <nav aria-label={t("primaryNav")} className="lt-side">
      <SidebarNav />
    </nav>
  );
});
