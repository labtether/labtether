"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Link } from "../../../i18n/navigation";
import { Button } from "../ui/Button";

/** A tab's control row: the view switch on the left, the tab's own buttons on the right. */
export function Bar({ left, right }: { left?: ReactNode; right?: ReactNode }) {
  if (!left && !right) return null;
  return (
    <div className="ct-bar">
      {left}
      <span className="lt-sp" />
      {right}
    </div>
  );
}

/** A button-styled link (secondary look), for actions that open another page. */
export function LinkButton({ href, children }: { href: string; children: ReactNode }) {
  return <Link href={href} className="ct-btn">{children}</Link>;
}

/** Grey placeholder rows while a list loads. */
export function ListSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="ct-list" aria-busy="true">
      {Array.from({ length: rows }, (_, i) => <div key={i} className="ct-row ct-skel"><span className="ct-lead" /><i /><i /></div>)}
    </div>
  );
}

/** A list that could not load: what happened and Try again. */
export function ListError({ text, onRetry }: { text: string; onRetry?: () => void }) {
  const t = useTranslations("containers");
  return (
    <div className="ct-list ct-none" role="alert">
      <p>{text}</p>
      {onRetry ? <Button onClick={onRetry}>{t("retry")}</Button> : null}
    </div>
  );
}

/** An empty list: one line, and an optional action under it. */
export function ListEmpty({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <div className="ct-list ct-none">
      <p>{text}</p>
      {action}
    </div>
  );
}

/** A key–value sheet row. */
export function KV({ k, children, mono = false }: { k: string; children: ReactNode; mono?: boolean }) {
  return (
    <div className="ct-kv">
      <dt>{k}</dt>
      <dd className={mono ? "mono" : undefined}>{children}</dd>
    </div>
  );
}

/** A section title on the canvas with optional buttons on its right. */
export function SectionHead({ id, title, children }: { id?: string; title: string; children?: ReactNode }) {
  return (
    <div className="ct-sh">
      <h2 id={id}>{title}</h2>
      {children ? <div className="ct-sh-acts">{children}</div> : null}
    </div>
  );
}
