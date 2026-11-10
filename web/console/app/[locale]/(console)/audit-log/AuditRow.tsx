"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Copy, MoreHorizontal, PanelTopOpen } from "lucide-react";
import { useTranslations } from "next-intl";
import { alertDevices, detailRows, ID_KEYS, outcome, targetText, type AuditEntry } from "./auditModel";

export type AuditText = {
  time: (e: AuditEntry) => string;
  who: (e: AuditEntry) => string;
  action: (e: AuditEntry) => string;
  /** Where it happened, after the object's name: "on nas01", "on 3 devices"; "" for most rows. */
  where: (e: AuditEntry) => string;
  /** The IDs the row left out (a person or object without a name), for its details. */
  ids: (e: AuditEntry) => Array<[string, string]>;
};

const ID_LABELS: Record<string, string> = { [ID_KEYS.actor]: "ids.actor", [ID_KEYS.apiKey]: "ids.apiKey", [ID_KEYS.target]: "ids.target" };

/** Time, person and what they did; a refused attempt leads with "Denied". Opens to its reason and details. */
export function AuditRow({
  entry,
  text,
  open,
  onToggle,
  onCopy,
}: {
  entry: AuditEntry;
  text: AuditText;
  open: boolean;
  onToggle: () => void;
  onCopy: (line: string) => void;
}) {
  const t = useTranslations("audit-log");
  const result = outcome(entry.decision);
  const details = detailRows(entry, text.ids(entry));
  const time = text.time(entry);
  const who = text.who(entry);
  const action = text.action(entry);
  const target = targetText(entry);
  const where = text.where(entry);
  // Several devices show as a count; their names are in the row's details and on hover.
  const devices = alertDevices(entry);
  const deviceNames = devices && "names" in devices ? devices.names.join(", ") : "";
  const line = [time, who, result ? `${t(`outcome.${result}`)}:` : "", action, target, where].filter(Boolean).join(" ");
  const what = (
    <>
      {result ? <span className={`lg-au-res ${result}`}>{t(`outcome.${result}`)}</span> : null}
      <span className="lg-au-act">{action}</span>
      {target ? <span className="lg-au-tg">{target}</span> : null}
      {where ? <span className="lg-au-wh" title={deviceNames || undefined}>{where}</span> : null}
    </>
  );
  return (
    <div className={`lg-au-g${open ? " open" : ""}`}>
      <div className="lg-au lg-au-r" data-row={entry.id}>
        <time className="lg-t" dateTime={entry.timestamp}>{time}</time>
        <span className="lg-au-who">{who}</span>
        {details.length > 0 ? (
          <button type="button" className="lg-au-what lg-au-btn" aria-expanded={open} onClick={onToggle}>{what}</button>
        ) : (
          <span className="lg-au-what">{what}</span>
        )}
        <Menu.Root>
          <Menu.Trigger className="lg-more" aria-label={t("rowActions")}>
            <MoreHorizontal aria-hidden="true" />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="lt-menu" align="end" sideOffset={4}>
              <Menu.Item className="lt-menu-item" onSelect={() => onCopy(line)}>
                <Copy aria-hidden="true" />
                {t("copyLine")}
              </Menu.Item>
              {details.length > 0 ? (
                <Menu.Item className="lt-menu-item" onSelect={onToggle}>
                  <PanelTopOpen aria-hidden="true" />
                  {open ? t("hideDetails") : t("showDetails")}
                </Menu.Item>
              ) : null}
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      </div>
      {open && details.length > 0 ? (
        <dl className="lg-fields lg-au-d">
          {details.map(([k, v]) => (
            <div key={k}><dt>{k === "reason" ? t("reason") : ID_LABELS[k] ? t(ID_LABELS[k]) : k}</dt><dd>{v}</dd></div>
          ))}
        </dl>
      ) : null}
    </div>
  );
}
