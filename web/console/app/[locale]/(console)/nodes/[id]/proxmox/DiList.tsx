"use client";

// 2.0 list grammar for the integration tabs on a device page (Proxmox, PBS, TrueNAS, Home Assistant, storage):
// section titles on the canvas, sheets under them, plain rows with a dot only when something is wrong, grey inline
// text, right-aligned numbers and one-hue usage meters. Styles live in app/styles/device-int.css (prefix .di-).

import type { CSSProperties, KeyboardEvent, ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../../i18n/navigation";
import { ofParts, usageTone } from "./diFormat";

export type DiTone = "ok" | "warn" | "bad" | "off" | "busy" | "";

/** The page of an integration tab: one container so its lists follow their own width. */
export function DiPage({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`di ${className}`}>{children}</div>;
}

/** A section: its title (and controls on the right) on the canvas, then its sheet. */
export function DiSection({ title, right, children, id }: { title?: ReactNode; right?: ReactNode; children: ReactNode; id?: string }) {
  return (
    <section className="di-sec" aria-labelledby={id}>
      {title || right ? (
        <div className="di-sh">
          {title ? <h2 id={id}>{title}</h2> : null}
          {right ? <div className="di-sh-r">{right}</div> : null}
        </div>
      ) : null}
      {children}
    </section>
  );
}

/** A raised sheet. "pad" for mixed content, "tight" for key-value blocks, "flush" for content that pads itself. */
export function DiSheet({ kind = "pad", className = "", children }: { kind?: "pad" | "tight" | "flush"; className?: string; children: ReactNode }) {
  return <div className={`di-sheet ${kind} ${className}`}>{children}</div>;
}

type Columns = { cols: string; colsM?: string; colsS?: string };

/**
 * A list sheet. cols is the grid of every row (CSS grid-template-columns); colsM applies under 760 px of list width
 * and colsS under 560 px, where cells marked "di-m" / "di-s" hide.
 */
export function DiList({ cols, colsM, colsS, hang = false, label, className = "", children }: Columns & {
  hang?: boolean;
  label?: string;
  className?: string;
  children: ReactNode;
}) {
  const style = { "--di-cols": cols, "--di-cols-m": colsM ?? cols, "--di-cols-s": colsS ?? "minmax(0, 1fr) auto" } as CSSProperties;
  return (
    <div className={`di-list${hang ? " hang" : ""} ${className}`} style={style} role="list" aria-label={label}>
      {children}
    </div>
  );
}

/** The column header: micro labels; give number columns className "r". */
export function DiCols({ children }: { children: ReactNode }) {
  return <div className="di-row di-cols" aria-hidden="true">{children}</div>;
}

/** One row. tone hangs a status dot in the left margin (only when not OK); onOpen makes the whole row a button. */
export function DiRow({ tone = "", onOpen, selected = false, label, className = "", children, after }: {
  tone?: DiTone;
  onOpen?: () => void;
  selected?: boolean;
  label?: string;
  className?: string;
  children: ReactNode;
  after?: ReactNode;
}) {
  const open = onOpen
    ? {
        onClick: (event: { target: EventTarget | null }) => {
          const target = event.target as HTMLElement | null;
          if (target?.closest("a, button, input, select, [role=menuitem]")) return;
          onOpen();
        },
        onKeyDown: (event: KeyboardEvent<HTMLDivElement>) => {
          if (event.target !== event.currentTarget) return;
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onOpen();
          }
        },
        tabIndex: 0,
        "aria-expanded": selected,
      }
    : {};
  return (
    <div role="listitem" className={`di-item${selected ? " sel" : ""}`}>
      <div className={`di-row${onOpen ? " click" : ""}${selected ? " sel" : ""} ${className}`} aria-label={label} {...open}>
        {tone && tone !== "ok" ? <span className="di-lead" aria-hidden="true"><DiDot tone={tone} /></span> : null}
        {children}
      </div>
      {after}
    </div>
  );
}

/** The row's name and its grey inline text. A problem replaces the grey text in its tone. */
export function DiMain({ name, href, grey, problem, mono = false, plain = false }: {
  name: ReactNode;
  href?: string;
  grey?: ReactNode;
  problem?: { tone: "bad" | "warn"; text: ReactNode } | null;
  mono?: boolean;
  plain?: boolean;
}) {
  const cls = `di-nm${mono ? " mono" : ""}${plain ? " plain" : ""}`;
  return (
    <span className="di-main">
      {href ? <Link href={href} className={cls}>{name}</Link> : <span className={cls}>{name}</span>}
      {problem ? <span className={`di-grey ${problem.tone}`}>{problem.text}</span> : grey ? <span className="di-grey">{grey}</span> : null}
    </span>
  );
}

/** Grey inline parts joined with " · ", skipping empty ones. */
export function DiDots({ parts }: { parts: ReactNode[] }) {
  const shown = parts.filter((part) => part !== null && part !== undefined && part !== false && part !== "");
  return (
    <>
      {shown.map((part, index) => (
        <span key={index}>{index > 0 ? " · " : ""}{part}</span>
      ))}
    </>
  );
}

/** A right-aligned number with its unit in grey; "—" when missing. hot turns it amber. */
export function DiNum({ value, unit, hot = false, className = "" }: { value?: ReactNode; unit?: string; hot?: boolean; className?: string }) {
  if (value === undefined || value === null || value === "") {
    return <span className={`di-num none ${className}`}>—</span>;
  }
  return (
    <span className={`di-num${hot ? " hot" : ""} ${className}`}>
      {value}
      {unit ? <span className="u">{unit === "%" ? unit : ` ${unit}`}</span> : null}
    </span>
  );
}

/** A right-aligned time or short quiet value. */
export function DiTime({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return <span className={`di-time ${className}`}>{children || "—"}</span>;
}

/** Three cells: a 4 px meter, the percent and "used of total"; amber at 85% or more. */
export function DiUsage({ pct, used, total, className = "" }: { pct: number | null; used?: number | null; total?: number | null; className?: string }) {
  const t = useTranslations("devices-proxmox.kit");
  const hot = usageTone(pct) === "warn";
  const of = ofParts(used, total);
  return (
    <>
      <span className={`di-meter di-m${hot ? " hi" : ""} ${className}`} aria-hidden="true">
        <i style={{ width: `${pct == null ? 0 : Math.max(0, Math.min(100, pct))}%` }} />
      </span>
      <DiNum value={pct == null ? undefined : Math.round(pct)} unit="%" hot={hot} />
      <span className="di-num di-s">{of ? t("of", of) : "—"}</span>
    </>
  );
}

/** A meter alone (dataset size against the pool, for example). */
export function DiMeter({ pct, hot = false, className = "" }: { pct: number | null; hot?: boolean; className?: string }) {
  return (
    <span className={`di-meter${hot ? " hi" : ""} ${className}`} aria-hidden="true">
      <i style={{ width: `${pct == null ? 0 : Math.max(0, Math.min(100, pct))}%` }} />
    </span>
  );
}

/** The 8 px status dot. */
export function DiDot({ tone }: { tone: DiTone }) {
  return <span className={`di-dot ${tone || "ok"}`} aria-hidden="true" />;
}

/** A status word with its dot; the word takes the tone colour only when bad or warn. */
export function DiStatus({ tone, word, time }: { tone: DiTone; word: ReactNode; time?: ReactNode }) {
  return (
    <span className={`di-st ${tone}`}>
      <DiDot tone={tone} />
      <span>{word}</span>
      {time ? <span className="di-st-t">{time}</span> : null}
    </span>
  );
}

/** Key-value lines in a tight sheet. Rows with an empty value are left out. */
export function DiKV({ items, columns = 1 }: { items: Array<[ReactNode, ReactNode] | null | false>; columns?: 1 | 2 }) {
  const rows = items.filter((item): item is [ReactNode, ReactNode] => !!item && item[1] !== null && item[1] !== undefined && item[1] !== "");
  if (rows.length === 0) return null;
  return (
    <dl className={`di-kv${columns === 2 ? " two" : ""}`}>
      {rows.map(([label, value], index) => (
        <div key={index}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** An empty or waiting state inside a sheet: a short bold line, an optional grey line and an optional button. */
export function DiEmpty({ title, line, action }: { title: ReactNode; line?: ReactNode; action?: ReactNode }) {
  return (
    <div className="di-empty">
      <b>{title}</b>
      {line ? <span>{line}</span> : null}
      {action}
    </div>
  );
}

/** A one-line message on the canvas above a list: what happened, in its tone. */
export function DiNote({ tone = "", children, action }: { tone?: "bad" | "warn" | "ok" | ""; children: ReactNode; action?: ReactNode }) {
  return (
    <div className={`di-note ${tone}`} role={tone === "bad" ? "alert" : "status"}>
      <span>{children}</span>
      {action}
    </div>
  );
}

/** The sheet footer: "25 of 645" on the left, a quiet button on the right. */
export function DiFoot({ left, right }: { left?: ReactNode; right?: ReactNode }) {
  return (
    <div className="di-foot">
      <span className="num">{left}</span>
      {right}
    </div>
  );
}

/** A log or output well inside a sheet. */
export function DiLog({ children, label }: { children: ReactNode; label?: string }) {
  return <pre className="di-log" aria-label={label} tabIndex={0}>{children}</pre>;
}
