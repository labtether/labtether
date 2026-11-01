"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Plus, Search } from "lucide-react";
import { useFastStatus } from "../../../contexts/StatusContext";
import type { Asset } from "../../../console/models";
import { formatAge } from "../../../console/formatters";
import { PageHeader } from "../../../components/PageHeader";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import QuickConnectForm from "./QuickConnectForm";
import { BookmarkForm } from "./BookmarkForm";
import { BookmarkRow } from "./BookmarkRow";
import { deleteBookmark, listBookmarks, type RemoteBookmark } from "./remoteBookmarksClient";
import { filterBookmarks, filterDevices, quickConnectCandidate, remoteDevices, type RemoteDevice } from "./newTabModel";
import type { AdhocConnectParams } from "./quickConnectModel";
import type { RemoteViewProtocol } from "./types";

interface NewTabPageProps {
  /** The tab row, shown under the header when more than one tab is open. */
  tabBar?: ReactNode;
  onConnectDevice: (assetId: string, name: string, protocol: RemoteViewProtocol) => void;
  onConnectBookmark: (bookmark: RemoteBookmark) => void;
  onConnectAdhoc: (params: AdhocConnectParams) => void;
}

/**
 * Remote View's new tab page: one "Connect to…" search over devices and bookmarks, then Devices and Bookmarks as plain
 * rows. Quick connect opens a host that is neither. A search that reads like an address offers Quick connect to it.
 */
export default function NewTabPage({ tabBar, onConnectDevice, onConnectBookmark, onConnectAdhoc }: NewTabPageProps) {
  const t = useTranslations("desktop.page");
  const tNav = useTranslations("nav");
  const [query, setQuery] = useState("");
  const [bookmarks, setBookmarks] = useState<RemoteBookmark[]>([]);
  const [bookmarksLoading, setBookmarksLoading] = useState(true);
  const [quickConnect, setQuickConnect] = useState<{ host: string } | null>(null);
  const [bookmarkForm, setBookmarkForm] = useState<RemoteBookmark | "new" | null>(null);
  const [removing, setRemoving] = useState<RemoteBookmark | null>(null);

  useEffect(() => {
    listBookmarks()
      .then(setBookmarks)
      .catch(() => setBookmarks([]))
      .finally(() => setBookmarksLoading(false));
  }, []);

  const status = useFastStatus();
  const devices = useMemo(() => remoteDevices(status?.assets as Asset[] | undefined), [status?.assets]);
  const shownDevices = filterDevices(devices, query);
  const shownBookmarks = filterBookmarks(bookmarks, query);
  const searching = !!query.trim();
  const candidate = searching && !shownDevices.length && !shownBookmarks.length ? quickConnectCandidate(query) : null;

  const removeBookmark = async (bookmark: RemoteBookmark) => {
    try {
      await deleteBookmark(bookmark.id);
      setBookmarks((prev) => prev.filter((b) => b.id !== bookmark.id));
    } catch (err) {
      console.error("Failed to delete bookmark:", err);
    } finally {
      setRemoving(null);
    }
  };

  return (
    <div className="rv-new">
      <PageHeader
        title={tNav("remoteView")}
        action={
          quickConnect ? null : (
            <button type="button" className="rv-btn primary" onClick={() => setQuickConnect({ host: "" })}>
              {t("quickConnect")}
            </button>
          )
        }
      />
      {tabBar ? <div className="rv-new-tabs">{tabBar}</div> : null}
      {quickConnect ? (
        <QuickConnectForm initialHost={quickConnect.host} onClose={() => setQuickConnect(null)} onConnect={onConnectAdhoc} />
      ) : null}

      <label className="rv-find">
        <Search aria-hidden="true" />
        <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("connectTo")} aria-label={t("connectTo")} />
      </label>

      {shownDevices.length || !searching ? (
        <section className="rv-sec" aria-labelledby="rv-sec-devices">
          <div className="rv-sh">
            <h2 id="rv-sec-devices">{t("devices")}</h2>
          </div>
          {shownDevices.length ? (
            <div className="rv-list">
              {shownDevices.map((device) => (
                <DeviceRow key={device.id} device={device} onOpen={() => onConnectDevice(device.id, device.name, "vnc")} />
              ))}
            </div>
          ) : (
            <p className="rv-none">{t("noDevices")}</p>
          )}
        </section>
      ) : null}

      {shownBookmarks.length || !searching ? (
        <section className="rv-sec" aria-labelledby="rv-sec-bookmarks">
          <div className="rv-sh">
            <h2 id="rv-sec-bookmarks">{t("bookmarks")}</h2>
            {bookmarkForm ? null : (
              <button type="button" className="rv-link" onClick={() => setBookmarkForm("new")}>
                <Plus aria-hidden="true" />
                {t("add")}
              </button>
            )}
          </div>
          {bookmarkForm ? (
            <BookmarkForm
              key={bookmarkForm === "new" ? "new" : bookmarkForm.id}
              editing={bookmarkForm === "new" ? null : bookmarkForm}
              onCancel={() => setBookmarkForm(null)}
              onSaved={(saved) => {
                setBookmarks((prev) => (prev.some((b) => b.id === saved.id) ? prev.map((b) => (b.id === saved.id ? saved : b)) : [...prev, saved]));
                setBookmarkForm(null);
              }}
            />
          ) : null}
          {bookmarksLoading ? (
            <p className="rv-none">{t("loadingBookmarks")}</p>
          ) : shownBookmarks.length ? (
            <div className="rv-list">
              {shownBookmarks.map((bookmark) => (
                <BookmarkRow
                  key={bookmark.id}
                  bookmark={bookmark}
                  onOpen={() => onConnectBookmark(bookmark)}
                  onEdit={() => setBookmarkForm(bookmark)}
                  onRemove={() => setRemoving(bookmark)}
                />
              ))}
            </div>
          ) : bookmarkForm ? null : (
            <p className="rv-none">{t("noBookmarks")}</p>
          )}
        </section>
      ) : null}

      {searching && !shownDevices.length && !shownBookmarks.length ? (
        <section className="rv-sec">
          {candidate ? (
            <div className="rv-list">
              <div className="rv-row">
                <span className="rv-lead" />
                <span className="rv-main">
                  <button type="button" className="rv-open" onClick={() => setQuickConnect({ host: candidate })}>
                    {t("quickConnectTo", { host: candidate })}
                  </button>
                </span>
              </div>
            </div>
          ) : (
            <p className="rv-none">{t("noMatch", { query: query.trim() })}</p>
          )}
        </section>
      ) : null}

      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        onConfirm={() => removing && void removeBookmark(removing)}
        title={t("removeTitle", { name: removing?.label ?? "" })}
        message={t(removing?.has_credentials ? "removeBodyLogin" : "removeBody")}
        confirmLabel={t("remove")}
        danger
      />
    </div>
  );
}

/** A device: its name opens it; the grey text is what it runs and its address, or its problem in the tone colour. */
function DeviceRow({ device, onOpen }: { device: RemoteDevice; onOpen: () => void }) {
  const tList = useTranslations("devices.list");
  const online = device.state === "online";
  const age = online ? "" : formatAge(device.lastSeen);
  return (
    <div className={`rv-row${online ? "" : " off"}`}>
      <span className="rv-lead">{online ? null : <i className={`rv-dot ${device.state === "offline" ? "bad" : "warn"}`} aria-hidden="true" />}</span>
      <span className="rv-main">
        <button type="button" className="rv-open" onClick={onOpen} disabled={!online}>
          {device.name}
        </button>
        {online ? null : (
          <span className={`rv-prob ${device.state === "offline" ? "bad" : "warn"}`}>
            {tList(device.state === "offline" ? "offline" : "notResponding", { age })}
          </span>
        )}
        <span className="rv-g">{[online ? device.os : "", device.address].filter(Boolean).join(" · ")}</span>
      </span>
    </div>
  );
}
