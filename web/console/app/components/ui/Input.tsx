import type { InputHTMLAttributes, SelectHTMLAttributes, ReactNode } from "react";

type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  className?: string;
  error?: boolean;
  success?: boolean;
};

export function Input({ className = "", error, success, ...rest }: InputProps) {
  const stateClasses = error
    ? "border-[var(--border-error)] shadow-[0_0_0_3px_var(--bg-error)]"
    : success
      ? "border-[var(--border-success)] shadow-[0_0_0_3px_var(--bg-success)]"
      : "border-[var(--line)] focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_var(--accent-subtle)]";

  return (
    <input
      className={`w-full bg-transparent border rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] transition-[border-color,box-shadow] duration-[var(--dur-fast)] outline-none disabled:bg-[var(--surface)] disabled:text-[var(--text-disabled)] disabled:cursor-not-allowed ${stateClasses} ${className}`}
      {...rest}
    />
  );
}

type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & {
  className?: string;
  children: ReactNode;
  error?: boolean;
  success?: boolean;
};

export function Select({ className = "", children, error, success, ...rest }: SelectProps) {
  const stateClasses = error
    ? "border-[var(--border-error)] shadow-[0_0_0_3px_var(--bg-error)]"
    : success
      ? "border-[var(--border-success)] shadow-[0_0_0_3px_var(--bg-success)]"
      : "border-[var(--line)] focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_var(--accent-subtle)]";

  return (
    <select
      className={`appearance-none bg-[var(--surface)] border rounded-lg px-3 py-2 pr-8 text-sm text-[var(--text)] transition-[border-color,box-shadow] duration-[var(--dur-fast)] outline-none cursor-pointer disabled:text-[var(--text-disabled)] disabled:cursor-not-allowed select-chevron ${stateClasses} ${className}`}
      style={{ backdropFilter: "blur(8px) saturate(1.4)" }}
      {...rest}
    >
      {children}
    </select>
  );
}
