import type { ButtonHTMLAttributes, ReactNode } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
  children: ReactNode;
};

const variantStyles: Record<string, string> = {
  primary:
    "bg-[var(--accent)] text-[var(--accent-contrast)] shadow-[0_0_20px_var(--accent-glow),inset_0_1px_0_rgba(255,255,255,0.15)] hover:shadow-[0_0_30px_var(--accent-glow),0_0_60px_var(--accent-glow),inset_0_1px_0_rgba(255,255,255,0.15)]",
  secondary:
    "bg-transparent border border-[var(--control-border)] text-[var(--control-fg)] hover:bg-[var(--control-bg-hover)] hover:border-[var(--text-secondary)]",
  ghost:
    "bg-transparent text-[var(--control-fg)] hover:bg-[var(--hover)]",
  danger:
    "bg-transparent border border-[var(--bad)]/30 text-[var(--bad)] hover:bg-[var(--bad-glow)] hover:shadow-[0_0_12px_var(--bad-glow)]",
};

const sizeStyles: Record<string, string> = {
  sm: "px-2.5 py-1 text-xs",
  md: "px-3.5 py-1.5 text-sm",
  lg: "px-5 py-2 text-sm",
};

export function Button({
  variant = "secondary",
  size = "md",
  loading = false,
  className = "",
  children,
  disabled,
  ...rest
}: ButtonProps) {
  return (
    <button
      className={`inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-[color,background-color,border-color,box-shadow,opacity] duration-[var(--dur-fast)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
      disabled={disabled || loading}
      {...rest}
    >
      {loading ? (
        <>
          <svg
            className="animate-spin h-3.5 w-3.5 shrink-0"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
          Loading...
        </>
      ) : (
        children
      )}
    </button>
  );
}
