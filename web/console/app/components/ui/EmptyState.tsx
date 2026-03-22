import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

type EmptyStateProps = {
  icon?: LucideIcon;
  title: string;
  description: string;
  action?: ReactNode;
  className?: string;
};

export function EmptyState({ icon: Icon, title, description, action, className = "" }: EmptyStateProps) {
  return (
    <div className={`flex flex-col items-center justify-center gap-2 py-12 px-6 text-center ${className}`}>
      {Icon ? <Icon className="w-11 h-11 text-[var(--muted)] mb-1" strokeWidth={1.5} /> : null}
      <p className="text-sm font-semibold text-[var(--text)]">{title}</p>
      <p className="text-[0.8125rem] text-[var(--muted)] max-w-[280px] leading-relaxed">{description}</p>
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}
