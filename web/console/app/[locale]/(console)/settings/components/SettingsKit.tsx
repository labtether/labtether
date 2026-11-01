"use client";

import { useState, type ReactNode } from "react";
import { Check, Copy } from "lucide-react";
import { Switch as KitSwitch } from "../../../../components/ui/Switch";

/*
 * The 2.0 Settings grammar shared by General, Notifications, Security and Agents: a section title on the canvas,
 * a sheet under it, rows inside (label left, control right), key-value facts, form grids and a sheet footer.
 * Styles live in app/styles/settings.css under the .st- prefix.
 */

export function Section({
  title,
  action,
  id,
  children,
}: {
  title: ReactNode;
  action?: ReactNode;
  id?: string;
  children: ReactNode;
}) {
  return (
    <section className="st-sec" id={id} aria-labelledby={id ? `${id}-h` : undefined}>
      <div className="st-sh">
        <h2 id={id ? `${id}-h` : undefined}>{title}</h2>
        {action ? <div className="st-sh-act">{action}</div> : null}
      </div>
      {children}
    </section>
  );
}

/** A raised sheet. pad: forms and mixed content (20 px); otherwise rows (4 px inset). */
export function Sheet({ pad = false, className = "", children }: { pad?: boolean; className?: string; children: ReactNode }) {
  return <div className={`st-sheet${pad ? " pad" : ""}${className ? ` ${className}` : ""}`}>{children}</div>;
}

/** One row: the name and its grey line on the left, the control on the right. two: the grey line sits under the name. */
export function Row({
  label,
  hint,
  two = false,
  className = "",
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  two?: boolean;
  className?: string;
  children?: ReactNode;
}) {
  return (
    <div className={`st-row${two ? " two" : ""}${className ? ` ${className}` : ""}`}>
      <div className="st-row-l">
        <span className="st-nm">{label}</span>
        {hint ? <span className="st-g">{hint}</span> : null}
      </div>
      {children ? <div className="st-row-r">{children}</div> : null}
    </div>
  );
}

/** Facts as key-value lines. */
export type Fact = readonly [ReactNode, ReactNode];

export function Facts({ items, className = "" }: { items: ReadonlyArray<Fact | null | false | undefined>; className?: string }) {
  return (
    <dl className={`st-kv${className ? ` ${className}` : ""}`}>
      {items.filter((item): item is Fact => Boolean(item)).map(([key, value], index) => {
        return (
          <div key={index}>
            <dt>{key}</dt>
            <dd>{value}</dd>
          </div>
        );
      })}
    </dl>
  );
}

/** A labelled form field. */
export function Field({
  label,
  htmlFor,
  hint,
  wide = false,
  children,
}: {
  label: ReactNode;
  htmlFor?: string;
  hint?: ReactNode;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={`st-field${wide ? " wide" : ""}`}>
      {htmlFor ? <label htmlFor={htmlFor}>{label}</label> : <span className="st-lbl">{label}</span>}
      {children}
      {hint ? <span className="st-hint">{hint}</span> : null}
    </div>
  );
}

/** The footer of a sheet: a message on the left, the sheet's buttons on the right. */
export function SheetFoot({ message, tone, children }: { message?: ReactNode; tone?: "bad"; children?: ReactNode }) {
  return (
    <div className="st-foot">
      {message ? (
        <p className={`st-msg${tone ? ` ${tone}` : ""}`} role={tone === "bad" ? "alert" : "status"}>
          {message}
        </p>
      ) : null}
      {children}
    </div>
  );
}

/** A plain one-line message under a control or a list. */
export function Message({ tone, children }: { tone?: "bad" | "warn"; children: ReactNode }) {
  return (
    <p className={`st-msg${tone ? ` ${tone}` : ""}`} role={tone === "bad" ? "alert" : "status"}>
      {children}
    </p>
  );
}

/** On/off switch: the kit's switch, named by label. */
export function Switch({
  checked,
  onChange,
  label,
  disabled = false,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  disabled?: boolean;
}) {
  return <KitSwitch checked={checked} onChange={onChange} aria-label={label} disabled={disabled} />;
}

/** Status: an 8 px dot and a word. Only shown when the state is not plain OK. */
export function Status({ tone, children }: { tone: "ok" | "warn" | "bad" | "off" | "busy"; children: ReactNode }) {
  return (
    <span className={`st-st ${tone}`}>
      <i aria-hidden="true" />
      {children}
    </span>
  );
}

/** Icon-only copy button; the aria-label becomes its tooltip. */
export function CopyButton({
  text,
  label,
  copied: copiedProp,
  onCopied,
  copiedLabel,
}: {
  text: string;
  label: string;
  copied?: boolean;
  onCopied?: (copied: boolean) => void;
  copiedLabel?: string;
}) {
  const [copiedLocal, setCopiedLocal] = useState(false);
  const copied = copiedProp ?? copiedLocal;
  const set = onCopied ?? setCopiedLocal;
  return (
    <button
      type="button"
      className="st-icon-btn"
      aria-label={copied && copiedLabel ? copiedLabel : label}
      onClick={() => {
        void navigator.clipboard
          ?.writeText(text)
          .then(() => {
            set(true);
            window.setTimeout(() => set(false), 2000);
          })
          .catch(() => {});
      }}
    >
      {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
    </button>
  );
}

/** A code well (config snippet, command) with a copy button. */
export function CodeBlock({ code, copyLabel }: { code: string; copyLabel: string }) {
  return (
    <div className="st-code">
      <pre>{code}</pre>
      <CopyButton text={code} label={copyLabel} />
    </div>
  );
}

/** A dialog on a scrim. The title element inside must carry the id given as labelledBy. */
export function SettingsDialog({
  labelledBy,
  onClose,
  wide = false,
  children,
}: {
  labelledBy: string;
  onClose: () => void;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className="st-scrim"
      onClick={onClose}
      onKeyDown={(event) => {
        if (event.key === "Escape") onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        className={`st-dlg${wide ? " wide" : ""}`}
        onClick={(event) => event.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}
