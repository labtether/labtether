"use client";

import { useMemo, type ReactNode } from "react";
import { PageHeader } from "../../../components/PageHeader";
import { FileListView } from "./FileListView";
import { PointMenu } from "./FileMenus";
import { FileTabBar } from "./FileTabBar";
import { DeleteConfirmDialog, NameDialog, TextPreviewOverlay, UploadConfirmDialog } from "./FilesOverlays";
import { FilesStatusLine } from "./FilesStatusLine";
import { FilesMoreMenu, FilesToolbar, type SplitChoice } from "./FilesToolbar";
import { pickedBytes } from "./filesListModel";
import { NewTabPage } from "./NewTabPage";
import { SplitView } from "./SplitView";
import { TransferProgressBar } from "./TransferProgressBar";
import { useFileSources } from "./useFileSources";
import { useFilesWorkspace } from "./useFilesWorkspace";

// The Files page: tabs on devices and saved connections. A new tab shows the source picker; a browsing tab shows the
// toolbar, the folder and a one-line status; the second pane, the transfers and the dialogs show only when used.
// page.tsx limits it to roles that may read host data.
export function FilesWorkspace() {
  const w = useFilesWorkspace();
  const { t, tabs, src, ws, transfers } = w;
  const sources = useFileSources();
  const tab = tabs.activeTab;
  const browsing = tab !== null && tab.type !== "new";
  const label = tab?.label ?? "";

  const unavailable = useMemo(
    () => new Set(tabs.tabs.filter((x) => x.type === "agent" && x.sourceId && !sources.connectedAgentIds.has(x.sourceId)).map((x) => x.id)),
    [tabs.tabs, sources.connectedAgentIds],
  );

  const splitChoices: SplitChoice[] = [
    ...sources.ready.map((a) => ({
      key: `a:${a.id}`,
      label: tab?.sourceId === a.id ? t("bar.thisTab", { name: a.name }) : a.name,
      onSelect: () => tabs.setSplit({ type: "agent", sourceId: a.id, protocol: "agent", label: a.name }),
    })),
    ...sources.connections.map((c) => ({
      key: `c:${c.id}`,
      label: tab?.sourceId === c.id ? t("bar.thisTab", { name: c.name }) : c.name,
      onSelect: () => tabs.setSplit({ type: "connection", sourceId: c.id, protocol: c.protocol, label: c.name }),
    })),
  ];

  const openAgent = (id: string, name: string) => tabs.openInTab({ type: "agent", sourceId: id, label: name, protocol: "agent" });
  const openConn = (id: string, name: string, protocol: string) => tabs.openInTab({ type: "connection", sourceId: id, label: name, protocol });

  const places = src.source?.type === "agent"
    ? [{ label: t("bar.home"), path: "~" }, { label: t("bar.root"), path: "/" }]
    : [{ label: t("bar.root"), path: "/" }];
  const picked = ws.selectedEntries;
  const where = t("dialog.where", { path: src.currentPath, source: label });

  const browser = (
    <>
      <FilesToolbar
        path={src.currentPath}
        places={places}
        busy={src.loading}
        canUp={src.canUp}
        onUp={src.goUp}
        canBack={w.history.canBack}
        onBack={w.history.back}
        onGo={src.goTo}
        query={w.query}
        onQuery={w.setQuery}
        clipboard={w.clipboard.value}
        canPaste={w.clipboard.canPaste}
        onPaste={w.clipboard.paste}
        onClearClipboard={w.clipboard.clear}
        view={w.view}
        onView={w.setView}
        showHidden={src.showHidden}
        onShowHidden={src.setShowHidden}
        onNewFolder={w.newFolder.show}
        onRefresh={src.refresh}
        onUpload={() => w.upload.pick(src.currentPath)}
        split={{ open: false, choices: splitChoices, onClose: () => tabs.setSplit(null) }}
      />
      {src.error ? <p className="fx-err" role="alert">{src.error}</p> : null}
      <div className="fx-sheet">
        <FileListView
          entries={w.visible}
          view={w.view}
          label={t("list.label", { path: src.currentPath })}
          loading={src.loading}
          emptyText={w.query ? t("list.noMatch", { query: w.query }) : t("list.empty")}
          picked={picked}
          sort={{ field: src.sortField, dir: src.sortDir, onSort: src.toggleSort }}
          rename={w.rename}
          dropText={w.dragOver ? t("list.drop") : null}
          onSelectAll={() => ws.toggleSelectAll(w.order)}
          drop={w.drop}
          handlers={{
            onPick: (name, mods) => ws.pickClick(name, w.order, mods),
            onToggle: ws.toggleSelected,
            onOpen: src.openFile,
            onContextMenu: w.onContextMenu,
            onKeyAction: (action, entry) => (action === "rename" ? w.startRename(entry.name) : w.del.ask(w.targetsFor(entry))),
            menuItems: w.rowMenu,
            menuLabel: (name) => t("menu.actionsFor", { name }),
            selectLabel: (name) => t("list.select", { name }),
            renameLabel: t("list.rename"),
          }}
        />
        <FilesStatusLine
          shown={w.visible.length}
          total={src.entries.length}
          searching={w.query.trim() !== ""}
          picked={picked.size}
          pickedBytes={pickedBytes(src.entries, picked)}
          onDownload={src.entries.some((e) => picked.has(e.name) && !e.is_dir) ? () => src.download(Array.from(picked)) : undefined}
          onDelete={() => w.del.ask(Array.from(picked))}
          onClear={ws.clearSelection}
        />
      </div>
      <input
        ref={w.upload.inputRef}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          w.upload.onInput(Array.from(e.target.files ?? []));
          e.target.value = "";
        }}
      />
    </>
  );

  let body: ReactNode;
  if (!browsing) {
    body = <NewTabPage sources={sources} onOpenAgent={openAgent} onOpenConnection={openConn} />;
  } else if (tabs.splitMode && src.source && tabs.splitSource && tab?.split) {
    body = (
      <>
        <div className="fx-bar">
          <span className="fx-sp" />
          <FilesMoreMenu split={{ open: true, choices: splitChoices, onClose: () => tabs.setSplit(null) }} />
        </div>
        <SplitView
          leftSource={src.source}
          leftLabel={label}
          rightSource={tabs.splitSource}
          rightLabel={tab.split.label}
          onTransfer={(files, from, to, dir) => void transfers.startTransfer(files, from, to, dir)}
          choices={splitChoices}
          onClose={() => tabs.setSplit(null)}
        />
      </>
    );
  } else if (src.waiting) {
    body = (
      <div className="fx-state">
        <p><span className="fx-dot bad" aria-hidden="true" />{t("state.noAgent", { name: label })}</p>
      </div>
    );
  } else {
    body = browser;
  }

  return (
    <div className="fx-page">
      {browsing ? (
        <h1 className="fx-crumb">
          <span className="fx-crumb-root">{t("title")}</span>
          <span className="fx-crumb-sep" aria-hidden="true">/</span>
          <span>{label}</span>
        </h1>
      ) : (
        <PageHeader title={t("title")} />
      )}
      {tabs.tabs.length ? (
        <FileTabBar
          tabs={tabs.tabs}
          activeTabId={tabs.activeTabId}
          onAddTab={() => tabs.addTab({ type: "new" })}
          onRemoveTab={tabs.removeTab}
          onSetActiveTab={tabs.setActiveTab}
          unavailable={unavailable}
        />
      ) : null}
      {body}

      <TransferProgressBar
        transfers={transfers.transfers}
        onCancel={transfers.cancelTransfer}
        onClearCompleted={transfers.clearCompleted}
        upload={src.uploadProgress}
        onCancelUpload={src.cancelUpload}
        download={src.downloadProgress}
        onCancelDownload={src.cancelDownload}
      />

      <PointMenu
        at={ws.contextMenu ? { x: ws.contextMenu.x, y: ws.contextMenu.y } : null}
        label={t("menu.label")}
        items={w.contextItems}
        onClose={ws.closeContextMenu}
      />
      <DeleteConfirmDialog names={w.del.names} where={where} onCancel={w.del.cancel} onConfirm={w.del.confirm} />
      <UploadConfirmDialog files={w.upload.pending?.files ?? null} where={t("dialog.where", { path: w.upload.pending?.dir ?? "", source: label })} onCancel={w.upload.cancel} onConfirm={w.upload.confirm} />
      <NameDialog open={w.newFolder.open} title={t("dialog.newFolder")} confirmLabel={t("dialog.create")} onCancel={w.newFolder.cancel} onSubmit={w.newFolder.create} />
      <TextPreviewOverlay
        previewContent={src.preview}
        onClose={src.closePreview}
        onDownload={() => {
          if (src.preview) src.download([src.preview.name]);
        }}
      />
    </div>
  );
}

