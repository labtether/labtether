"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, Copy, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type ReactNode } from "react";
import type { FileEntry } from "./fileOpsClient";
import { unifiedListFiles, type UnifiedFileEntry } from "./fileOpsClient";
import { parentPath } from "./remotePaths";
import { clickSelection } from "./filesListModel";
import { FileListView } from "./FileListView";
import { ToolMenu, type MenuItemSpec } from "./FileMenus";
import { FilesPathBar } from "./FilesPathBar";
import type { SplitChoice } from "./FilesToolbar";
import type { FileSource } from "./useFileTabsState";

// Two panes side by side (stacked on a phone): the tab's source on the left, a second source (or the same one in
// another folder) on the right. Picked items copy into the other pane's open folder with "Copy to …" or by dragging
// them across; every copy runs as a transfer.

const DRAG_TYPE = "application/x-labtether-files";

interface SplitViewProps {
  leftSource: FileSource;
  leftLabel: string;
  rightSource: FileSource;
  rightLabel: string;
  onTransfer: (sourceFiles: string[], sourceSource: FileSource, destSource: FileSource, destPath: string) => void;
  choices: SplitChoice[];
  onClose: () => void;
}

type PaneProps = {
  side: "left" | "right";
  source: FileSource;
  label: string;
  otherLabel: string;
  otherPath: string;
  onPath: (path: string) => void;
  onCopy: (paths: string[]) => void;
  head?: ReactNode;
};

const toEntry = (e: UnifiedFileEntry): FileEntry => ({ name: e.name, is_dir: e.is_dir, size: e.size, mod_time: e.mod_time ?? "", mode: e.mode ?? "" });

function SplitPane({ side, source, label, otherLabel, otherPath, onPath, onCopy, head }: PaneProps) {
  const t = useTranslations("files");
  const [path, setPath] = useState(source.type === "agent" ? "~" : "/");
  const [entries, setEntries] = useState<UnifiedFileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [dropping, setDropping] = useState(false);
  const anchor = useRef<string | null>(null);
  const fetchId = useRef(0);

  const load = useCallback(async (next: string) => {
    const id = ++fetchId.current;
    setLoading(true);
    setError(null);
    setPicked(new Set());
    try {
      const result = await unifiedListFiles(source, next);
      if (id !== fetchId.current) return;
      setPath(result.path ?? next);
      setEntries([...result.entries].sort((a, b) => (a.is_dir !== b.is_dir ? (a.is_dir ? -1 : 1) : a.name.localeCompare(b.name))));
    } catch (err) {
      if (id !== fetchId.current) return;
      setError(err instanceof Error ? err.message : String(err));
      setEntries([]);
    } finally {
      if (id === fetchId.current) setLoading(false);
    }
  }, [source]);

  useEffect(() => {
    void load(source.type === "agent" ? "~" : "/");
  }, [load, source]);

  useEffect(() => { onPath(path); }, [onPath, path]);

  const list = useMemo(() => entries.map(toEntry), [entries]);
  const order = useMemo(() => entries.map((e) => e.name), [entries]);
  const pathsOf = (names: string[]) => entries.filter((e) => names.includes(e.name)).map((e) => e.path);
  const copyTo = otherLabel === label ? otherPath : otherLabel;

  const accept = (e: DragEvent) => {
    if (!e.dataTransfer.types.includes(DRAG_TYPE)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    setDropping(true);
  };

  return (
    <section className="fx-pane" aria-label={`${label} ${path}`}>
      <div className="fx-pane-h">
        <span className="fx-pane-nm">{label}</span>
        <span className="fx-sp" />
        {head}
      </div>
      <div className="fx-pane-path">
        <FilesPathBar path={path} onGo={(p) => void load(p)} canUp={path !== "/" && path !== "~" && parentPath(path) !== path} onUp={() => void load(parentPath(path))} busy={loading} idSuffix={`-${side}`} />
      </div>
      {error ? <p className="fx-err" role="alert">{error}</p> : null}
      <FileListView
        entries={list}
        compact
        label={t("list.label", { path })}
        loading={loading}
        emptyText={t("list.empty")}
        picked={picked}
        dropText={dropping ? t("list.dropCopy") : null}
        drop={{
          onDragOver: accept,
          onDragLeave: (e) => { if (!(e.relatedTarget instanceof Node) || !e.currentTarget.contains(e.relatedTarget)) setDropping(false); },
          onDrop: (e) => {
            setDropping(false);
            const raw = e.dataTransfer.getData(DRAG_TYPE);
            if (!raw) return;
            e.preventDefault();
            try {
              const data = JSON.parse(raw) as { paths: string[]; side: string };
              if (data.side !== side && data.paths.length) onCopy(data.paths);
            } catch {
              // Not our drag data.
            }
          },
        }}
        handlers={{
          onPick: (name, mods) => {
            const next = clickSelection(picked, order, name, anchor.current, mods);
            anchor.current = next.anchor;
            setPicked(next.selected);
          },
          onToggle: (name) => setPicked((prev) => clickSelection(prev, order, name, null, { toggle: true, range: false }).selected),
          onOpen: (entry) => { if (entry.is_dir) void load(entries.find((e) => e.name === entry.name)?.path ?? path); },
          onContextMenu: () => {},
          onDragStart: (e, entry) => {
            const names = picked.has(entry.name) ? Array.from(picked) : [entry.name];
            e.dataTransfer.setData(DRAG_TYPE, JSON.stringify({ paths: pathsOf(names), side }));
            e.dataTransfer.effectAllowed = "copy";
          },
          menuItems: (entry): MenuItemSpec[] => [
            ...(entry.is_dir ? [{ key: "open", label: t("menu.open"), onSelect: () => void load(entries.find((x) => x.name === entry.name)?.path ?? path) }] : []),
            { key: "copy", label: t("split.copyTo", { name: copyTo }), onSelect: () => onCopy(pathsOf(picked.has(entry.name) ? Array.from(picked) : [entry.name])) },
          ],
          menuLabel: (name) => t("menu.actionsFor", { name }),
          selectLabel: (name) => t("list.select", { name }),
          renameLabel: t("list.rename"),
        }}
      />
      {picked.size ? (
        <div className="fx-status">
          <span className="fx-num">{t("status.picked", { count: picked.size })}</span>
          <span className="fx-sp" />
          <button type="button" className="fx-btn sm" onClick={() => { onCopy(pathsOf(Array.from(picked))); setPicked(new Set()); }}>
            <Copy aria-hidden="true" width={13} height={13} strokeWidth={2} />
            {t("split.copyTo", { name: copyTo })}
          </button>
          <button type="button" className="fx-btn sm quiet" onClick={() => setPicked(new Set())}>{t("status.clear")}</button>
        </div>
      ) : null}
    </section>
  );
}

export function SplitView({ leftSource, leftLabel, rightSource, rightLabel, onTransfer, choices, onClose }: SplitViewProps) {
  const t = useTranslations("files");
  const [leftPath, setLeftPath] = useState("");
  const [rightPath, setRightPath] = useState("");

  const swap = (
    <>
      <ToolMenu
        label={t("split.pick")}
        trigger={
          <button type="button" className="fx-ib sm" title={t("split.pick")}>
            <ChevronDown aria-hidden="true" width={14} height={14} strokeWidth={2} />
          </button>
        }
      >
        <div className="fx-menu-scroll">
          {choices.map((c) => <Menu.Item key={c.key} className="lt-menu-item" onSelect={c.onSelect}>{c.label}</Menu.Item>)}
        </div>
      </ToolMenu>
      <button type="button" className="fx-ib sm" onClick={onClose} aria-label={t("bar.closeSplit")} title={t("bar.closeSplit")}>
        <X aria-hidden="true" width={14} height={14} strokeWidth={2} />
      </button>
    </>
  );

  return (
    <div className="fx-split">
      <SplitPane
        side="left"
        source={leftSource}
        label={leftLabel}
        otherLabel={rightLabel}
        otherPath={rightPath}
        onPath={setLeftPath}
        onCopy={(paths) => onTransfer(paths, leftSource, rightSource, rightPath)}
      />
      <SplitPane
        key={`${rightSource.type}:${rightSource.type === "agent" ? rightSource.assetId : rightSource.connectionId}`}
        side="right"
        source={rightSource}
        label={rightLabel}
        otherLabel={leftLabel}
        otherPath={leftPath}
        onPath={setRightPath}
        onCopy={(paths) => onTransfer(paths, rightSource, leftSource, leftPath)}
        head={swap}
      />
    </div>
  );
}
