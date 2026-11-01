import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

type EmptyStateProps = {
  /** Kept for existing callers; 2.0 empty states show no icon. */
  icon?: LucideIcon;
  title: string;
  description: string;
  action?: ReactNode;
  className?: string;
};

/** 2.0 empty state (kit.css .kt-empty): what is missing, one plain line, and at most one button. */
export function EmptyState({ title, description, action, className = "" }: EmptyStateProps) {
  return (
    <div className={`kt-empty${className ? ` ${className}` : ""}`}>
      <p className="kt-empty-t">{title}</p>
      {description ? <p className="kt-empty-d">{description}</p> : null}
      {action ? <div className="kt-empty-a">{action}</div> : null}
    </div>
  );
}
