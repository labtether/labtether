import type { ButtonHTMLAttributes, ReactNode } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
  children: ReactNode;
};

// 2.0 controls: a raised surface with a hairline edge; rose only for the page's main button; no glow.
const variantStyles: Record<string, string> = {
  primary:
    "bg-[var(--accent)] text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)]",
  secondary:
    "bg-[var(--ctl-bg)] text-[var(--text)] shadow-[var(--ctl-elev)] hover:shadow-[var(--ctl-elev-hover)]",
  ghost:
    "bg-transparent text-[var(--text-2)] hover:bg-[var(--hover)] hover:text-[var(--text)]",
  danger:
    "bg-[var(--ctl-bg)] text-[var(--bad-text)] shadow-[var(--ctl-elev)] hover:bg-[var(--bad-glow)]",
};

const sizeStyles: Record<string, string> = {
  sm: "h-[var(--ctl-sm)] px-2.5 text-[13px]",
  md: "h-[var(--ctl)] px-3 text-[13px]",
  lg: "h-9 px-4 text-sm",
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
      className={`inline-flex items-center justify-center gap-1.5 rounded-[var(--r-3)] whitespace-nowrap font-medium transition-[color,background-color,border-color,box-shadow,opacity] duration-[var(--dur-fast)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
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
