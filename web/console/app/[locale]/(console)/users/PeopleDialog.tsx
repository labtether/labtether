"use client";

import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Modal } from "../../../components/ui/Modal";
import { Button } from "../../../components/ui/Button";

/**
 * The one dialog shape for Users, Groups and Health: a title, fields or a sentence, one error line, then Cancel and the
 * action. Enter submits. While busy, the dialog stays open and the buttons wait.
 */
export function PeopleDialog({
  open,
  title,
  onClose,
  onSubmit,
  submitLabel,
  busy = false,
  error,
  tone = "plain",
  wide = false,
  disabled = false,
  children,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  onSubmit: () => void;
  submitLabel: string;
  busy?: boolean;
  error?: string;
  tone?: "plain" | "main" | "danger";
  wide?: boolean;
  disabled?: boolean;
  children?: ReactNode;
}) {
  const tc = useTranslations("common");
  const variant = tone === "danger" ? "danger" : tone === "main" ? "primary" : "secondary";
  return (
    <Modal open={open} onClose={() => { if (!busy) onClose(); }} title={title} className={wide ? "pp-wide" : ""}>
      <form className="pp-dlg" noValidate onSubmit={(event) => { event.preventDefault(); if (!busy && !disabled) onSubmit(); }}>
        {children}
        {error ? <p className="pp-err" role="alert">{error}</p> : null}
        <div className="pp-acts">
          <Button type="button" variant="ghost" onClick={onClose} disabled={busy}>{tc("cancel")}</Button>
          <Button type="submit" variant={variant} loading={busy} disabled={disabled}>{submitLabel}</Button>
        </div>
      </form>
    </Modal>
  );
}

/** A labelled control with its own error line under it. */
export function Field({ id, label, error, children }: { id: string; label: string; error?: string; children: ReactNode }) {
  return (
    <div className="pp-field">
      <label htmlFor={id}>{label}</label>
      {children}
      {error ? <p className="pp-err" id={`${id}-err`}>{error}</p> : null}
    </div>
  );
}

/**
 * What to show when a hub call fails: the hub's own sentence, or the fallback when the hub sent none (the hooks then
 * carry an internal "failed to … (500)" line that must not reach the screen).
 */
export function errorText(err: unknown, fallback: string): string {
  const message = err instanceof Error ? err.message.trim() : "";
  if (!message || /\(\d{3}\)$/.test(message) || /^failed to /i.test(message)) return fallback;
  return message;
}
