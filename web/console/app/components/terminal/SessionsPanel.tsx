"use client";

import { ChevronRight } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { Button } from "../ui/Button";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Modal } from "../ui/Modal";
import SessionCard, { type SessionRowAction } from "./SessionCard";
import type { TerminalSessionLibrary } from "./useTerminalSessionLibrary";
import { useTerminalWhen } from "./useTerminalWhen";
import type { Bookmark, PersistentSession } from "./terminalSessionTypes";
export type { SessionsPanelProps } from "./terminalSessionTypes";

type SectionProps = { lib: TerminalSessionLibrary; nameOf: (id: string) => string };
type Ask = { title: string; body: string; confirm: string; run: () => void } | null;

/** The rename field that takes a row's name while renaming. */
function RenameField({ lib, label }: { lib: TerminalSessionLibrary; label: string }) {
  return (
    <input
      ref={lib.renameInputRef}
      className="tm-rename"
      aria-label={label}
      value={lib.renameValue}
      maxLength={80}
      onChange={(event) => lib.setRenameValue(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          void lib.handleRenameSubmit();
        } else if (event.key === "Escape") {
          lib.setRenameTarget(null);
        }
      }}
      onBlur={() => void lib.handleRenameSubmit()}
    />
  );
}

function AskDialog({ ask, onClose }: { ask: Ask; onClose: () => void }) {
  return (
    <ConfirmDialog
      open={ask !== null}
      onClose={onClose}
      onConfirm={() => {
        ask?.run();
        onClose();
      }}
      title={ask?.title ?? ""}
      message={ask?.body ?? ""}
      confirmLabel={ask?.confirm}
      danger
    />
  );
}

/** Sessions that keep running on the hub after their tab closed (or are open elsewhere). Rows reattach. */
export function RunningSessions({ lib, nameOf }: SectionProps) {
  const t = useTranslations("terminal.sessions");
  const when = useTerminalWhen();
  const [ask, setAsk] = useState<Ask>(null);
  const list = [...lib.detachedSessions, ...lib.activeSessions];
  if (list.length === 0) return null;
  const row = (s: PersistentSession) => {
    const device = nameOf(s.target) || s.target;
    const name = s.title || device;
    const renaming = lib.renameTarget?.type === "session" && lib.renameTarget.id === s.id;
    const meta = s.status === "attached" ? t("attached") : t("detachedAt", { time: when(s.last_detached_at || s.updated_at) });
    const save: SessionRowAction[] = s.bookmark_id ? [] : [{ label: t("saveConnection"), onSelect: () => void lib.saveAsBookmark(s) }];
    return (
      <SessionCard
        key={s.id}
        name={name}
        grey={name === device ? undefined : device}
        meta={meta}
        tone={s.status === "attached" ? "ok" : undefined}
        onOpen={(newTab) => void lib.reattach(s, newTab)}
        openLabel={t("reattachTo", { name })}
        moreLabel={t("more", { name })}
        editor={renaming ? <RenameField lib={lib} label={t("renameLabel", { name })} /> : undefined}
        actions={[
          { label: t("reattach"), onSelect: () => void lib.reattach(s) },
          { label: t("openNewTab"), onSelect: () => void lib.reattach(s, true) },
          "sep",
          { label: t("rename"), onSelect: () => lib.startRename({ type: "session", id: s.id, currentTitle: s.title }) },
          ...save,
          "sep",
          {
            label: t("end"),
            danger: true,
            onSelect: () => setAsk({ title: t("endTitle", { name: device }), body: t("endBody", { name: device }), confirm: t("endConfirm"), run: () => void lib.endSession(s.id) }),
          },
        ]}
      />
    );
  };
  return (
    <section className="tm-sec" aria-labelledby="tm-running-h">
      <h2 className="tm-h" id="tm-running-h">{t("running")}</h2>
      <div className="tm-list">{list.map(row)}</div>
      <AskDialog ask={ask} onClose={() => setAsk(null)} />
    </section>
  );
}

const addressOf = (b: Bookmark) =>
  b.host ? `${b.username ? `${b.username}@` : ""}${b.host}${b.port && b.port !== 22 ? `:${b.port}` : ""}` : "";

/** Saved SSH connections (terminal bookmarks). Rows connect. */
export function SavedConnections({ lib, nameOf }: SectionProps) {
  const t = useTranslations("terminal.sessions");
  const when = useTerminalWhen();
  const [ask, setAsk] = useState<Ask>(null);
  if (lib.savedBookmarks.length === 0) return null;
  const row = (b: Bookmark) => {
    const renaming = lib.renameTarget?.type === "bookmark" && lib.renameTarget.id === b.id;
    const grey = b.asset_id ? nameOf(b.asset_id) || b.asset_id : addressOf(b);
    return (
      <SessionCard
        key={b.id}
        name={b.title}
        grey={grey}
        meta={b.last_used_at ? t("usedAt", { time: when(b.last_used_at) }) : undefined}
        onOpen={(newTab) => void lib.connectBookmark(b, newTab)}
        openLabel={t("connectTo", { name: b.title })}
        moreLabel={t("more", { name: b.title })}
        editor={renaming ? <RenameField lib={lib} label={t("renameLabel", { name: b.title })} /> : undefined}
        actions={[
          { label: t("connect"), onSelect: () => void lib.connectBookmark(b) },
          { label: t("openNewTab"), onSelect: () => void lib.connectBookmark(b, true) },
          "sep",
          { label: t("rename"), onSelect: () => lib.startRename({ type: "bookmark", id: b.id, currentTitle: b.title }) },
          {
            label: t("delete"),
            danger: true,
            onSelect: () => setAsk({ title: t("deleteTitle", { name: b.title }), body: t("deleteBody"), confirm: t("deleteConfirm"), run: () => void lib.deleteBookmark(b.id) }),
          },
        ]}
      />
    );
  };
  return (
    <section className="tm-sec" aria-labelledby="tm-saved-h">
      <h2 className="tm-h" id="tm-saved-h">{t("saved")}</h2>
      <div className="tm-list">{lib.savedBookmarks.map(row)}</div>
      <AskDialog ask={ask} onClose={() => setAsk(null)} />
    </section>
  );
}

/** Sessions that ended, folded until asked for (or while filtering). Rows show what they printed. */
export function EndedSessions({ lib, nameOf }: SectionProps) {
  const t = useTranslations("terminal.sessions");
  const when = useTerminalWhen();
  const [open, setOpen] = useState(false);
  if (lib.archivedSessions.length === 0) return null;
  const shown = open || lib.query !== "";
  return (
    <section className="tm-sec">
      <button type="button" className="tm-h tm-fold" aria-expanded={shown} onClick={() => setOpen((v) => !v)}>
        <ChevronRight aria-hidden="true" className={shown ? "open" : undefined} />
        {t("ended")}
      </button>
      {shown ? (
        <div className="tm-list">
          {lib.archivedSessions.map((s) => {
            const name = s.title || nameOf(s.target) || s.target;
            return (
              <SessionCard
                key={s.id}
                name={name}
                meta={t("endedAt", { time: when(s.archived_at) })}
                onOpen={() => void lib.reattach(s)}
                openLabel={t("viewOutput", { name })}
                moreLabel={t("more", { name })}
                actions={[{ label: t("view"), onSelect: () => void lib.reattach(s) }]}
              />
            );
          })}
        </div>
      ) : null}
    </section>
  );
}

/** Every session and saved connection, from the workspace's "+" menu. */
export default function SessionsPanel({
  open,
  onClose,
  lib,
  nameOf,
  onAddConnection,
}: SectionProps & { open: boolean; onClose: () => void; onAddConnection: () => void }) {
  const t = useTranslations("terminal.sessions");
  const empty = lib.activeSessions.length + lib.detachedSessions.length + lib.savedBookmarks.length + lib.archivedSessions.length === 0;
  return (
    <Modal open={open} onClose={onClose} title={t("title")} className="tm-dlg-wide">
      <div className="tm-dlg">
        <div className="tm-dlg-tools">
          <label className="tm-q tm-q-sm">
            <span className="sr-only">{t("filter")}</span>
            <input type="search" value={lib.searchQuery} placeholder={t("filter")} onChange={(e) => lib.setSearchQuery(e.target.value)} />
          </label>
          <Button onClick={onAddConnection}>{t("add")}</Button>
        </div>
        <div className="tm-dlg-scroll">
          <RunningSessions lib={lib} nameOf={nameOf} />
          <SavedConnections lib={lib} nameOf={nameOf} />
          <EndedSessions lib={lib} nameOf={nameOf} />
          {empty ? <p className="tm-none">{lib.query ? t("noMatch") : t("none")}</p> : null}
        </div>
      </div>
    </Modal>
  );
}
