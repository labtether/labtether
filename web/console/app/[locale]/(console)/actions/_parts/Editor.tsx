"use client";

import { ChevronDown, ChevronUp } from "lucide-react";
import { type FormEvent, type ReactNode, useEffect, useRef } from "react";
import { Button } from "../../../../components/ui/Button";

/**
 * The create and edit form of an automations tab: its title on the canvas, the fields on one sheet with Cancel and the
 * main button in the sheet's footer. Escape cancels. The first field takes focus when it opens.
 */
export function EditorSheet({
  title,
  submitLabel,
  cancelLabel,
  onSubmit,
  onCancel,
  pending = false,
  error,
  footStart,
  children,
}: {
  title: string;
  submitLabel: string;
  cancelLabel: string;
  onSubmit: () => void;
  onCancel: () => void;
  pending?: boolean;
  error?: string | null;
  footStart?: ReactNode;
  children: ReactNode;
}) {
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    formRef.current?.querySelector<HTMLElement>("input, select, textarea")?.focus();
  }, []);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!pending) onSubmit();
  };

  return (
    <section className="au-fsec" aria-label={title}>
      <h2>{title}</h2>
      <form
        ref={formRef}
        className="au-sheet"
        aria-label={title}
        noValidate
        onSubmit={submit}
        onKeyDown={(event) => {
          if (event.key === "Escape" && !pending) {
            event.preventDefault();
            onCancel();
          }
        }}
      >
        {children}
        {error ? <p role="alert" className="au-err au-ferr">{error}</p> : null}
        <div className="au-foot">
          {footStart ? <span className="au-foot-note">{footStart}</span> : null}
          <Button type="button" variant="ghost" onClick={onCancel} disabled={pending}>{cancelLabel}</Button>
          <Button type="submit" variant="primary" disabled={pending} aria-busy={pending || undefined}>{submitLabel}</Button>
        </div>
      </form>
    </section>
  );
}

/** A labelled field with its error under it. */
export function Field({
  id,
  label,
  error,
  hint,
  wide = false,
  children,
}: {
  id: string;
  label: string;
  error?: string;
  hint?: ReactNode;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={`au-field${wide ? " au-wide" : ""}`}>
      <label htmlFor={id}>{label}</label>
      {children}
      {error ? <p className="au-err" id={`${id}-err`}>{error}</p> : hint ? <p className="au-hint" id={`${id}-hint`}>{hint}</p> : null}
    </div>
  );
}

/** aria props for a field's control: invalid and described by its error or hint. */
export function fieldAria(id: string, error?: string, hint?: boolean) {
  if (error) return { "aria-invalid": true as const, "aria-describedby": `${id}-err` };
  return hint ? { "aria-describedby": `${id}-hint` } : {};
}

export type SortState<K extends string> = { key: K; dir: "asc" | "desc" };

/** A sortable column header: the label, and an arrow on the column the list is sorted by. */
export function SortHead<K extends string>({
  label,
  column,
  sort,
  onSort,
}: {
  label: string;
  column: K;
  sort: SortState<K>;
  onSort: (next: SortState<K>) => void;
}) {
  const on = sort.key === column;
  const Arrow = sort.dir === "asc" ? ChevronUp : ChevronDown;
  return (
    <button
      type="button"
      className={`au-sort${on ? " on" : ""}`}
      aria-pressed={on}
      onClick={() => onSort({ key: column, dir: on && sort.dir === "asc" ? "desc" : "asc" })}
    >
      {label}
      {on ? <Arrow aria-hidden="true" strokeWidth={2.4} /> : null}
    </button>
  );
}
