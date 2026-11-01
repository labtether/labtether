import type { ReactNode } from "react";

/** 2.0 page header: the title (24/32) with the page's actions on the right. A subtitle, when given, is one grey line. */
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
  return (
    <header className={`lt-ph${className ? ` ${className}` : ""}`}>
      <div className="lt-ph-row">
        <h1>{title}</h1>
        {action ?? null}
      </div>
      {subtitle ? <p className="lt-ph-sub">{subtitle}</p> : null}
    </header>
  );
}
