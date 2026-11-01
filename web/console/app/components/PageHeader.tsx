"use client";

import { useContext, type ReactNode } from "react";
import { PageSectionContext } from "./pageSection";

/**
 * 2.0 page header: the title (24/32) with the page's actions on the right. A subtitle, when given, is one grey line.
 * On a section's pages (Automations, Settings) the title is the section's name and its tab row sits under it, as in
 * the mockup; the page's own title is then the current tab.
 */
export function PageHeader({
  title,
  subtitle,
  action,
  className,
}: {
  title: string;
  subtitle?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  const section = useContext(PageSectionContext);
  return (
    <>
      <header className={`lt-ph${className ? ` ${className}` : ""}`}>
        <div className="lt-ph-row">
          <h1>{section?.title ?? title}</h1>
          {action ?? null}
        </div>
        {subtitle ? <p className="lt-ph-sub">{subtitle}</p> : null}
      </header>
      {section?.tabs ?? null}
    </>
  );
}
