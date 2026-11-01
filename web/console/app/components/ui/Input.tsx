import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";

/** 2.0 fields (kit.css .kt-input): 32 px, a 1 px edge, a 3 px halo on focus, red edge when invalid. */

type FieldState = {
  className?: string;
  error?: boolean;
  success?: boolean;
};

function fieldClass(base: string, { className = "", success }: FieldState): string {
  return [base, success ? "ok" : "", className].filter(Boolean).join(" ");
}

type InputProps = InputHTMLAttributes<HTMLInputElement> & FieldState;

export function Input({ className = "", error, success, ...rest }: InputProps) {
  return (
    <input
      aria-invalid={error ? true : undefined}
      {...rest}
      className={fieldClass("kt-input", { className, success })}
    />
  );
}

type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & FieldState & {
  children: ReactNode;
};

export function Select({ className = "", children, error, success, ...rest }: SelectProps) {
  return (
    <select
      aria-invalid={error ? true : undefined}
      {...rest}
      className={fieldClass("kt-input select-chevron", { className, success })}
    >
      {children}
    </select>
  );
}

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & FieldState;

export function Textarea({ className = "", error, success, ...rest }: TextareaProps) {
  return (
    <textarea
      aria-invalid={error ? true : undefined}
      {...rest}
      className={fieldClass("kt-input", { className, success })}
    />
  );
}
