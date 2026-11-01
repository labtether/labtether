"use client";

import { Box, ChevronDown, ChevronUp } from "lucide-react";
import { useTranslations } from "next-intl";
import type { MouseEvent, ReactNode } from "react";
import { Link, useRouter } from "../../../i18n/navigation";
import { iconKeyFor, useIconIndex } from "./containerIcons";
import { formatMem, shortImage, type CtRow, type Section, type SectionRow, type Sort, type SortKey } from "./containerModel";
import { ContainerMenu } from "./ContainerMenu";
import type { MenuEntry } from "./RowMenu";
import { useStateWord, useStoredSet } from "./useContainerWords";

/** The container's app icon from the service icon set, or a plain box. */
export function ContainerGlyph({ image, name, size = 16 }: { image: string; name: string; size?: number }) {
  const index = useIconIndex();
  const key = iconKeyFor(image, name, index);
  return (
    <span className="ct-g" aria-hidden="true">
      {key ? <img src={`/service-icons/${key}.svg`} alt="" width={size} height={size} loading="lazy" decoding="async" draggable={false} /> : <Box strokeWidth={1.75} />}
    </span>
  );
}

/** A plain number with its unit in grey; "—" when there is none. */
function Num({ text, hot = false }: { text: string | null; hot?: boolean }) {
  if (text == null) return <span className="ct-n none">—</span>;
  const m = /^([\d.,]+)\s*(.*)$/.exec(text);
  return (
    <span className={`ct-n${hot ? " hot" : ""}`}>
      {m ? m[1] : text}
      {m && m[2] ? <span className="u">{m[2] === "%" ? "%" : ` ${m[2]}`}</span> : null}
    </span>
  );
}

function Ports({ row }: { row: CtRow }) {
  if (!row.ports.length) return <span className="ct-ports none" />;
  const list = row.ports.map((p) => (p.proto === "tcp" ? String(p.host) : `${p.host}/${p.proto}`));
  const shown = list.slice(0, 2).join(", ");
  return <span className="ct-ports" title={list.join(", ")}>{list.length > 2 ? `${shown} +${list.length - 2}` : shown}</span>;
}

/** One container: dot (only when not plainly running), icon, name, grey image · stack (or the state word), ports, CPU, memory, ⋯. */
export function ContainerRow({ item, onChanged, extra }: { item: SectionRow; onChanged?: () => void; extra?: (row: CtRow) => MenuEntry[] }) {
  const { row, rail, showStack, showHost } = item;
  const router = useRouter();
  const word = useStateWord()(row.state);
  const grey = word || [shortImage(row.image), showStack && row.stack !== row.name ? row.stack : "", showHost ? row.host.label : ""].filter(Boolean).join(" · ");
  const open = (event: MouseEvent<HTMLDivElement>) => {
    // Clicks in the row's menu or confirm (portalled elsewhere in the page) and on its links stay theirs.
    if (!event.currentTarget.contains(event.target as Node) || (event.target as HTMLElement).closest("a, button")) return;
    if (event.metaKey || event.ctrlKey) window.open(row.href, "_blank", "noopener,noreferrer");
    else router.push(row.href);
  };
  const target = row.host.source === "portainer"
    ? { source: "portainer" as const, endpointAssetId: row.host.key, containerId: row.id }
    : { source: "docker" as const, assetId: row.assetId };
  return (
    <div className={`ct-row${rail ? ` stk stk-${rail}` : ""}`} role="row" onClick={open}>
      <span className="ct-lead" aria-hidden="true">{row.tone ? <i className={`ct-dot ${row.tone}`} /> : null}</span>
      <ContainerGlyph image={row.image} name={row.name} />
      <span className="ct-main" role="cell">
        <Link href={row.href} className="ct-nm">{row.name}</Link>
        <span className={`ct-grey${word ? ` w-${row.tone || "off"}` : ""}`} title={word ? undefined : row.image}>{grey}</span>
      </span>
      <span role="cell" className="ct-pc"><Ports row={row} /></span>
      <span role="cell" className="ct-r"><Num text={row.cpu == null ? null : `${row.cpu.toFixed(1)}%`} hot={(row.cpu ?? 0) >= 85} /></span>
      <span role="cell" className="ct-r"><Num text={row.mem == null ? null : formatMem(row.mem)} /></span>
      <span className="ct-act" role="cell">
        <ContainerMenu target={target} name={row.name} image={row.image} href={row.href} running={row.state.kind === "running"} busy={row.tone === "busy" && row.state.kind !== "running"} extra={extra?.(row)} onChanged={onChanged} />
      </span>
    </div>
  );
}

function SortHead({ k, label, sort, onSort, right = false }: { k: SortKey; label: string; sort?: Sort; onSort?: (k: SortKey) => void; right?: boolean }) {
  const t = useTranslations("containers.columns");
  if (!sort || !onSort) return <span className={right ? "r" : ""}>{label}</span>;
  const on = sort.key === k;
  const Icon = on && sort.dir === "desc" ? ChevronDown : ChevronUp;
  return (
    <button type="button" className={`ct-sort${on ? " on" : ""}${right ? " r" : ""}`} onClick={() => onSort(k)} aria-label={t("sortBy", { column: label })} aria-pressed={on}>
      {label}
      {on ? <Icon aria-hidden="true" strokeWidth={2.2} /> : null}
    </button>
  );
}

/**
 * Containers in sections (hosts or stacks) under one column header. A host's stack members sit together on a thin
 * rail. Sections fold; the folded ones are kept per browser. storeKey: where folded sections are kept.
 */
export function ContainerList({
  sections,
  sort,
  onSort,
  onChanged,
  storeKey = "labtether.containers.closed",
  empty,
  foot,
  label,
  extra,
}: {
  sections: Section[];
  sort?: Sort;
  onSort?: (k: SortKey) => void;
  onChanged?: () => void;
  storeKey?: string;
  empty?: ReactNode;
  foot?: ReactNode;
  label: string;
  /** More ⋯ items for a row. */
  extra?: (row: CtRow) => MenuEntry[];
}) {
  const t = useTranslations("containers");
  const [closed, flip] = useStoredSet(storeKey);
  return (
    <div className="ct-list" role="table" aria-label={label}>
      <div className="ct-row ct-cols" role="row">
        <span className="ct-lead" />
        <span />
        <SortHead k="name" label={t("columns.container")} sort={sort} onSort={onSort} />
        <span className="ct-pc">{t("columns.ports")}</span>
        <SortHead k="cpu" label={t("columns.cpu")} sort={sort} onSort={onSort} right />
        <SortHead k="mem" label={t("columns.memory")} sort={sort} onSort={onSort} right />
        <span />
      </div>
      {sections.map((s) => {
        const isClosed = closed.has(s.key);
        const title = s.kind === "nostack" ? t("noStack") : s.title;
        return (
          <div key={s.key} role="rowgroup" aria-label={title || undefined}>
            {s.kind !== "all" && (
              <div className={`ct-gh${isClosed ? " closed" : ""}`} role="row">
                <span className="ct-gh-c" role="cell">
                  <button type="button" className="ct-gh-t" aria-expanded={!isClosed} aria-label={s.href ? title : undefined} onClick={() => flip(s.key)}>
                    <ChevronDown aria-hidden="true" strokeWidth={2.4} />
                    {s.href ? null : title}
                  </button>
                  {s.href ? <Link href={s.href} className="ct-gh-l">{title}</Link> : null}
                </span>
              </div>
            )}
            {!isClosed && s.rows.map((item) => <ContainerRow key={item.row.key} item={item} onChanged={onChanged} extra={extra} />)}
          </div>
        );
      })}
      {!sections.length && empty ? <div className="ct-empty">{empty}</div> : null}
      {foot ? <div className="ct-foot">{foot}</div> : null}
    </div>
  );
}
