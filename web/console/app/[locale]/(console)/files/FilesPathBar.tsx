"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ArrowLeft, ArrowUp, ChevronDown, Folder } from "lucide-react";
import { useTranslations } from "next-intl";
import { Fragment, useState } from "react";
import { pathCrumbs } from "./filesListModel";
import { ToolMenu } from "./FileMenus";

// Back, Up and the path: crumbs to click, or a field to type a path in (click beside the crumbs). The folder menu
// on the left lists the places to jump to (home and the filesystem root).

export type Place = { label: string; path: string };

type FilesPathBarProps = {
  path: string;
  onGo: (path: string) => void;
  canUp: boolean;
  onUp: () => void;
  canBack?: boolean;
  onBack?: () => void;
  places?: Place[];
  busy?: boolean;
  /** A controlled draft for the typed path (a device's Files tab keeps it across folder reads). */
  draft?: string;
  onDraft?: (value: string) => void;
  idSuffix?: string;
};

export function FilesPathBar({ path, onGo, canUp, onUp, canBack, onBack, places = [], busy, draft, onDraft, idSuffix = "" }: FilesPathBarProps) {
  const t = useTranslations("files.bar");
  const [editing, setEditing] = useState(false);
  const [localDraft, setLocalDraft] = useState("");
  const value = draft ?? localDraft;
  const setValue = onDraft ?? setLocalDraft;
  const crumbs = pathCrumbs(path);
  const inputId = `fx-path-in${idSuffix}`;

  const startEdit = () => {
    setValue(path);
    setEditing(true);
  };
  const submit = () => {
    const next = value.trim();
    setEditing(false);
    if (next && next !== path) onGo(next);
  };

  const placesMenu = places.length ? (
    <ToolMenu
      label={t("places")}
      align="start"
      trigger={
        <button type="button" className="fx-pl" title={t("places")}>
          <Folder aria-hidden="true" width={15} height={15} strokeWidth={1.75} />
          <ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.2} />
        </button>
      }
    >
      {places.map((p) => (
        <Menu.Item key={p.path} className="lt-menu-item fx-place" onSelect={() => onGo(p.path)}>
          <span>{p.label}</span>
          <span className="lt-menu-hint fx-mono">{p.path}</span>
        </Menu.Item>
      ))}
    </ToolMenu>
  ) : null;

  return (
    <div className="fx-path-w">
      {onBack ? (
        <button type="button" className="fx-ib" onClick={onBack} disabled={!canBack || busy} aria-label={t("back")} title={t("back")}>
          <ArrowLeft aria-hidden="true" width={15} height={15} strokeWidth={2} />
        </button>
      ) : null}
      <button type="button" className="fx-ib" onClick={onUp} disabled={!canUp || busy} aria-label={t("up")} title={t("up")}>
        <ArrowUp aria-hidden="true" width={15} height={15} strokeWidth={2} />
      </button>
      {editing ? (
        <div className="fx-path ed">
          <label className="sr-only" htmlFor={inputId}>{t("path")}</label>
          <input
            id={inputId}
            className="fx-pin"
            value={value}
            autoFocus
            spellCheck={false}
            autoComplete="off"
            onFocus={(e) => e.target.select()}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") { e.preventDefault(); submit(); }
              if (e.key === "Escape") { e.preventDefault(); setEditing(false); }
            }}
            onBlur={() => setEditing(false)}
          />
        </div>
      ) : (
        <nav className="fx-path" aria-label={t("path")}>
          {placesMenu}
          <span className="fx-crumbs">
            {crumbs.map((c, i) => {
              const last = i === crumbs.length - 1;
              const sep = i > 1 || (i === 1 && crumbs[0].label !== "/") ? <span className="fx-sep" aria-hidden="true">/</span> : null;
              return (
                <Fragment key={c.path}>
                  {sep}
                  {last ? (
                    <span className="fx-here" aria-current="page">{c.label}</span>
                  ) : (
                    <button type="button" onClick={() => onGo(c.path)} disabled={busy}>{c.label}</button>
                  )}
                </Fragment>
              );
            })}
          </span>
          <button type="button" className="fx-pedit" onClick={startEdit} aria-label={t("typePath")} title={t("typePath")} />
        </nav>
      )}
    </div>
  );
}
