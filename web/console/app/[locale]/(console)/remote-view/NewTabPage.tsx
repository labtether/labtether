"use client";

import { useState, useEffect, useMemo, useCallback } from "react";
import { Search, Plus, Trash2, Monitor, X, Zap } from "lucide-react";
import { useFastStatus } from "../../../contexts/StatusContext";
import type { Asset } from "../../../console/models";
import { assetTypeIcon, isDeviceTier } from "../../../console/taxonomy";
import {
  PROTOCOL_DOT_COLOR,
  PROTOCOL_BADGE_STYLE,
  defaultPort,
  PROTOCOLS,
  type RemoteViewProtocol,
} from "./types";
import {
  listBookmarks,
  createBookmark,
  deleteBookmark,
  type RemoteBookmark,
} from "./remoteBookmarksClient";
import { PageHeader } from "../../../components/PageHeader";
import { Input, Select } from "../../../components/ui/Input";
import QuickConnectDialog from "./QuickConnectDialog";

// ── Props ──

interface NewTabPageProps {
  onConnectDevice: (assetId: string, name: string, protocol: RemoteViewProtocol) => void;
  onConnectBookmark: (bookmark: RemoteBookmark) => void;
  onConnectAdhoc: (params: {
    host: string;
    port: number;
    protocol: RemoteViewProtocol;
    username?: string;
    password?: string;
    saveBookmark?: { label: string };
  }) => void;
}

// ── Component ──

export default function NewTabPage({
  onConnectDevice,
  onConnectBookmark,
  onConnectAdhoc,
}: NewTabPageProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [bookmarks, setBookmarks] = useState<RemoteBookmark[]>([]);
  const [bookmarksLoading, setBookmarksLoading] = useState(true);
  const [showQuickConnect, setShowQuickConnect] = useState(false);

  // Bookmark form
  const [showBookmarkForm, setShowBookmarkForm] = useState(false);
  const [bookmarkLabel, setBookmarkLabel] = useState("");
  const [bookmarkHost, setBookmarkHost] = useState("");
  const [bookmarkPort, setBookmarkPort] = useState("");
  const [bookmarkProtocol, setBookmarkProtocol] = useState<RemoteViewProtocol>("vnc");
  const [bookmarkSaving, setBookmarkSaving] = useState(false);

  useEffect(() => {
    listBookmarks()
      .then(setBookmarks)
      .catch(() => setBookmarks([]))
      .finally(() => setBookmarksLoading(false));
  }, []);

  const status = useFastStatus();
  const assets = status?.assets;
  const remoteViewDevices = useMemo(() => {
    if (!assets) return [];
    return (assets as Asset[])
      .filter((a: Asset) => isDeviceTier(a))
      .map((a: Asset) => ({
        id: a.id,
        name: a.name,
        type: a.type,
        ip: a.metadata?.primary_ip,
        platform: a.platform,
        online: a.status === "online",
      }));
  }, [assets]);

  type DeviceItem = (typeof remoteViewDevices)[0];

  const filteredDevices = useMemo(() => {
    if (!searchQuery) return remoteViewDevices;
    const q = searchQuery.toLowerCase();
    return remoteViewDevices.filter(
      (d: DeviceItem) => d.name.toLowerCase().includes(q) || (d.ip && d.ip.includes(q)),
    );
  }, [remoteViewDevices, searchQuery]);

  const filteredBookmarks = useMemo(() => {
    if (!searchQuery) return bookmarks;
    const q = searchQuery.toLowerCase();
    return bookmarks.filter(
      (bm) => bm.label.toLowerCase().includes(q) || bm.host.toLowerCase().includes(q),
    );
  }, [bookmarks, searchQuery]);

  const handleDeviceClick = useCallback(
    (device: (typeof remoteViewDevices)[0]) => {
      if (!device.online) return;
      onConnectDevice(device.id, device.name, "vnc");
    },
    [onConnectDevice],
  );

  const handleDeleteBookmark = useCallback(async (id: string) => {
    try {
      await deleteBookmark(id);
      setBookmarks((prev) => prev.filter((b) => b.id !== id));
    } catch (err) {
      console.error("Failed to delete bookmark:", err);
    }
  }, []);

  const openBookmarkForm = useCallback(() => {
    setBookmarkLabel("");
    setBookmarkHost("");
    setBookmarkProtocol("vnc");
    setBookmarkPort(String(defaultPort("vnc")));
    setShowBookmarkForm(true);
  }, []);

  const cancelBookmarkForm = useCallback(() => {
    setShowBookmarkForm(false);
  }, []);

  const handleSaveBookmark = useCallback(async () => {
    const host = bookmarkHost.trim();
    if (!host) return;
    setBookmarkSaving(true);
    try {
      const created = await createBookmark({
        label: bookmarkLabel.trim() || host,
        protocol: bookmarkProtocol,
        host,
        port: parseInt(bookmarkPort, 10) || defaultPort(bookmarkProtocol),
      });
      setBookmarks((prev) => [...prev, created]);
      setShowBookmarkForm(false);
    } catch (err) {
      console.error("Failed to save bookmark:", err);
    } finally {
      setBookmarkSaving(false);
    }
  }, [bookmarkLabel, bookmarkHost, bookmarkPort, bookmarkProtocol]);

  return (
    <div className="flex-1 flex flex-col gap-6 p-4 md:p-6 overflow-y-auto animate-fade-in">
      {/* 1. PageHeader with Quick Connect action */}
      <PageHeader
        title="Remote View"
        subtitle="Connect to remote desktops across your homelab"
        action={
          <button
            onClick={() => setShowQuickConnect(true)}
            className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-lg bg-[var(--accent-subtle)] border border-[var(--accent)]/20 text-[var(--accent-text)] text-xs font-medium hover:border-[var(--accent)]/40 transition-[border-color] duration-[var(--dur-fast)]"
          >
            <Zap size={14} />
            Quick Connect
          </button>
        }
      />

      {/* 2. Search bar */}
      <div className="max-w-2xl mx-auto w-full">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--muted)] pointer-events-none" />
          <Input
            className="pl-9"
            placeholder="Search devices and bookmarks..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      {/* 3. Available Devices */}
      <div className="max-w-2xl mx-auto w-full">
        <h3 className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider mb-3">
          Available Devices
        </h3>

        {filteredDevices.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-4 px-4 py-8">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-[var(--line)] bg-[var(--surface)]">
              <Monitor size={20} className="text-[var(--muted)]" />
            </div>
            <p className="text-sm text-[var(--muted)]">
              {searchQuery ? "No devices or bookmarks match your search" : "No devices found"}
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-1.5">
            {filteredDevices.map((device) => {
              const Icon = assetTypeIcon(device.type);
              return (
                <button
                  key={device.id}
                  onClick={() => handleDeviceClick(device)}
                  disabled={!device.online}
                  className={`group flex items-center gap-2.5 w-full text-left px-3 py-2.5 rounded-lg border transition-[border-color,box-shadow] duration-[var(--dur-fast)] ${
                    device.online
                      ? "border-[var(--panel-border)] bg-[var(--panel-glass)] cursor-pointer hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
                      : "border-[var(--panel-border)]/50 bg-[var(--panel-glass)]/50 opacity-50 cursor-not-allowed"
                  }`}
                >
                  {/* Icon container */}
                  <span
                    className={`relative flex items-center justify-center w-[30px] h-[30px] rounded-lg shrink-0 ${
                      device.online ? "bg-green-500/10" : "bg-[var(--surface)]"
                    }`}
                  >
                    <Icon size={14} className={device.online ? "text-green-500/60" : "text-[var(--muted)]"} />
                    {device.online && (
                      <span
                        className="absolute -top-0.5 -right-0.5 w-[7px] h-[7px] rounded-full bg-[var(--ok)]"
                        style={{ boxShadow: "0 0 6px 1px var(--ok-glow)" }}
                      />
                    )}
                  </span>

                  {/* Text */}
                  <span className="flex-1 min-w-0">
                    <span className="block text-sm font-medium text-[var(--text)] truncate">
                      {device.name}
                    </span>
                    <span className="block text-[10px] text-[var(--muted)]">
                      {device.ip ?? "No IP"}
                      {device.platform && (
                        <>
                          {" "}
                          &middot; <span className="capitalize">{device.platform}</span>
                        </>
                      )}
                      {!device.online && " \u00b7 offline"}
                    </span>
                  </span>

                  {/* Protocol badge -- only online */}
                  {device.online && (
                    <span
                      className={`text-[9px] px-[7px] py-0.5 rounded-[5px] border font-medium shrink-0 ${PROTOCOL_BADGE_STYLE.vnc}`}
                    >
                      VNC
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* 4. Bookmarks */}
      <div className="max-w-2xl mx-auto w-full">
        <h3 className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider mb-3">
          Bookmarks
        </h3>

        <div className="flex flex-col gap-1.5">
          {bookmarksLoading ? (
            <p className="text-xs text-[var(--muted)] py-4 text-center">Loading...</p>
          ) : (
            <>
              {filteredBookmarks.map((bm) => (
                <div
                  key={bm.id}
                  role="button"
                  tabIndex={0}
                  onClick={() => onConnectBookmark(bm)}
                  onKeyDown={(e) => e.key === "Enter" && onConnectBookmark(bm)}
                  className="group flex items-center gap-2.5 px-3 py-2.5 rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] cursor-pointer transition-[border-color,box-shadow] duration-[var(--dur-fast)] hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)]"
                >
                  <span
                    className={`w-[7px] h-[7px] rounded-full shrink-0 ${PROTOCOL_DOT_COLOR[bm.protocol]}`}
                  />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-[var(--text)] truncate">
                      {bm.label}
                    </div>
                    <div className="text-[10px] text-[var(--muted)] font-mono truncate">
                      {bm.protocol}://{bm.host}:{bm.port}
                    </div>
                  </div>
                  <span
                    className={`text-[9px] px-[7px] py-0.5 rounded-[5px] border font-medium shrink-0 ${PROTOCOL_BADGE_STYLE[bm.protocol]}`}
                  >
                    {bm.protocol.toUpperCase()}
                  </span>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDeleteBookmark(bm.id);
                    }}
                    className="p-1 rounded opacity-0 group-hover:opacity-60 hover:!opacity-100 hover:bg-[var(--hover)] transition-opacity"
                  >
                    <Trash2 className="w-3 h-3" />
                  </button>
                </div>
              ))}

              {showBookmarkForm ? (
                <div className="p-3 rounded-lg border border-[var(--line)] bg-[var(--surface)]">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs font-semibold text-[var(--text-secondary)]">
                      New Bookmark
                    </span>
                    <button
                      onClick={cancelBookmarkForm}
                      className="p-0.5 rounded hover:bg-[var(--hover)] text-[var(--muted)]"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                  <div className="flex flex-col gap-2">
                    <Input
                      type="text"
                      value={bookmarkLabel}
                      onChange={(e) => setBookmarkLabel(e.target.value)}
                      placeholder="Label (optional)"
                    />
                    <Input
                      type="text"
                      value={bookmarkHost}
                      onChange={(e) => setBookmarkHost(e.target.value)}
                      placeholder="Host"
                    />
                    <div className="flex gap-1.5">
                      <Select
                        value={bookmarkProtocol}
                        onChange={(e) => {
                          const proto = e.target.value as RemoteViewProtocol;
                          setBookmarkProtocol(proto);
                          setBookmarkPort(String(defaultPort(proto)));
                        }}
                        className="flex-1"
                      >
                        {PROTOCOLS.map((p) => (
                          <option key={p.value} value={p.value}>
                            {p.label}
                          </option>
                        ))}
                      </Select>
                      <Input
                        type="number"
                        value={bookmarkPort}
                        onChange={(e) => setBookmarkPort(e.target.value)}
                        placeholder="Port"
                        className="w-20"
                      />
                    </div>
                    <div className="flex gap-1.5 pt-1">
                      <button
                        onClick={cancelBookmarkForm}
                        className="flex-1 px-2 py-1.5 rounded text-xs text-[var(--text-secondary)] border border-[var(--line)] hover:bg-[var(--hover)] transition-colors duration-[var(--dur-fast)]"
                      >
                        Cancel
                      </button>
                      <button
                        onClick={handleSaveBookmark}
                        disabled={!bookmarkHost.trim() || bookmarkSaving}
                        className="flex-1 px-2 py-1.5 rounded text-xs font-medium text-white bg-[var(--accent)] hover:opacity-90 transition-opacity duration-[var(--dur-fast)] disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {bookmarkSaving ? "Saving..." : "Save"}
                      </button>
                    </div>
                  </div>
                </div>
              ) : (
                <button
                  onClick={openBookmarkForm}
                  className="flex items-center justify-center gap-1.5 py-2.5 rounded-lg border border-dashed border-[var(--line)] text-[var(--muted)] hover:border-[var(--accent)]/50 hover:text-[var(--text-secondary)] transition-colors duration-[var(--dur-fast)]"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span className="text-xs">Add bookmark</span>
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {/* 5. QuickConnectDialog */}
      <QuickConnectDialog
        open={showQuickConnect}
        onClose={() => setShowQuickConnect(false)}
        onConnect={(params) => {
          onConnectAdhoc({
            host: params.host,
            port: params.port,
            protocol: params.protocol,
            username: params.username,
            password: params.password,
            saveBookmark: params.saveBookmark,
          });
        }}
      />
    </div>
  );
}
