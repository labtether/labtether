import type { InputHTMLAttributes, ReactNode } from "react";
import { Input } from "../../components/ui/Input";

/**
 * The quiet single column of sign-in and first-run setup, a little above the middle of the canvas: the mascot,
 * an optional step line ("1 of 2"), the heading, then the form.
 */
export function SignInShell({ title, step, wide, children }: { title: string; step?: string; wide?: boolean; children: ReactNode }) {
  return (
    <main className="si">
      <div className={`si-col${wide ? " wide" : ""}`}>
        <img src="/logo.svg" alt="LabTether" width={40} height={40} className="si-logo" />
        {step ? <p className="si-step">{step}</p> : null}
        <h1 className="si-h">{title}</h1>
        {children}
      </div>
    </main>
  );
}

/** A label over its input, and the input's own error under it. */
export function SignInField({
  id,
  label,
  error,
  hint,
  className = "",
  ...input
}: InputHTMLAttributes<HTMLInputElement> & { id: string; label: string; error?: string; hint?: ReactNode }) {
  const describedBy = [error ? `${id}-err` : "", hint ? `${id}-hint` : ""].filter(Boolean).join(" ") || undefined;
  return (
    <div className="si-field">
      <label htmlFor={id}>{label}</label>
      <Input id={id} className={className} error={Boolean(error)} aria-invalid={error ? true : undefined} aria-describedby={describedBy} {...input} />
      {hint ? <div className="si-hint" id={`${id}-hint`}>{hint}</div> : null}
      {error ? <p className="si-err" id={`${id}-err`}>{error}</p> : null}
    </div>
  );
}
