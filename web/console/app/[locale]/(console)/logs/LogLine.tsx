"use client";

import { Copy } from "lucide-react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import type { LogEvent } from "../../../console/models";
import { RowCheck } from "./LogControls";
import { levelWord } from "./logsModel";

export type LineCtx = {
  timeOf: (event: LogEvent) => string;
  deviceOf: (event: LogEvent) => string;
};

/** The level word only when it says something: error and warn in colour, debug grey. */
function Level({ level }: { level: string }) {
  const t = useTranslations("logs");
  const word = levelWord(level);
  return <span className={`lg-lv ${word}`}>{word === "info" ? "" : t(`levelWord.${word}`)}</span>;
}

function DeviceCell({ event, name }: { event: LogEvent; name: string }) {
  if (!event.asset_id) return <span className="lg-dev lg-dev-none">{name}</span>;
  return (
    <Link href={`/nodes/${encodeURIComponent(event.asset_id)}`} className="lg-dev">
      {name}
    </Link>
  );
}

/** A grey line around the open one, from the same device. */
function ContextLine({ event, ctx }: { event: LogEvent; ctx: LineCtx }) {
  return (
    <div className="lg-ln lg-ctx">
      <span className="lg-x" />
      <time className="lg-t">{ctx.timeOf(event)}</time>
      <Level level={event.level} />
      <span className="lg-x" />
      <span className="lg-src">{event.source}</span>
      <span className="lg-cm">{event.message}</span>
    </div>
  );
}

/** One log line: tick box, time, level, device, source and the message (two lines at most until opened). */
export function LogLine({
  event,
  ctx,
  open,
  selected,
  onOpen,
  onSelect,
}: {
  event: LogEvent;
  ctx: LineCtx;
  open: boolean;
  selected: boolean;
  onOpen: () => void;
  onSelect: () => void;
}) {
  const t = useTranslations("logs");
  const time = ctx.timeOf(event);
  const device = ctx.deviceOf(event);
  return (
    <div className={`lg-ln${open ? " open" : ""}${selected ? " sel" : ""}`} data-row={event.id}>
      <RowCheck checked={selected} label={t("selectLine", { time, device: device || event.source })} onChange={onSelect} />
      <time className="lg-t" dateTime={event.timestamp}>{time}</time>
      <Level level={event.level} />
      <DeviceCell event={event} name={device} />
      <span className="lg-src">{event.source}</span>
      <button type="button" className="lg-msg" aria-expanded={open} onClick={onOpen}>
        <span className="lg-mt">{event.message}</span>
      </button>
    </div>
  );
}

/** The open line with two lines either side from the same device, its fields, Copy line and Open device. */
export function OpenLogLine({
  event,
  ctx,
  newer,
  older,
  selected,
  onClose,
  onSelect,
  onCopy,
}: {
  event: LogEvent;
  ctx: LineCtx;
  newer: LogEvent[];
  older: LogEvent[];
  selected: boolean;
  onClose: () => void;
  onSelect: () => void;
  onCopy: () => void;
}) {
  const t = useTranslations("logs");
  const fields = Object.entries(event.fields ?? {}).filter(([, v]) => v !== "");
  const device = ctx.deviceOf(event);
  return (
    <div className="lg-grp">
      {newer.map((e) => <ContextLine key={e.id} event={e} ctx={ctx} />)}
      <LogLine event={event} ctx={ctx} open selected={selected} onOpen={onClose} onSelect={onSelect} />
      {older.map((e) => <ContextLine key={e.id} event={e} ctx={ctx} />)}
      {fields.length > 0 ? (
        <dl className="lg-fields" aria-label={t("fields")}>
          {fields.map(([k, v]) => (
            <div key={k}><dt>{k}</dt><dd>{v}</dd></div>
          ))}
        </dl>
      ) : null}
      <div className="lg-acts">
        <button type="button" className="lg-btn" onClick={onCopy}>
          <Copy aria-hidden="true" strokeWidth={1.75} />
          {t("copyLine")}
        </button>
        {event.asset_id ? (
          <Link href={`/nodes/${encodeURIComponent(event.asset_id)}`} className="lg-btn quiet">
            {t("openDevice", { device })}
          </Link>
        ) : null}
      </div>
    </div>
  );
}
