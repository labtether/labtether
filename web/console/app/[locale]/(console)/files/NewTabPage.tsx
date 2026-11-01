"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, ChevronRight, Plus, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useMemo, useState } from "react";
import type { Asset } from "../../../console/models";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import { ConnectionForm } from "./ConnectionForm";
import { RowMenu, ToolMenu } from "./FileMenus";
import { deleteFileConnection, testFileConnection, type FileConnection } from "./fileConnectionsClient";
import { connectionAddress, parseQuickConnect } from "./filesListModel";
import { TransportNotice } from "./TransportNotice";
import type { useFileSources, Unavailable } from "./useFileSources";

// The page a Files tab opens on: "Connect to…" (type a name, Enter opens the first match; sftp://host starts a new
// connection), the devices whose agent is connected, saved connections (SFTP, SMB, FTP, WebDAV) with New connection,
// Edit…, Test and Delete…, and the devices it can't open folded under Not available with the reason.

export const PROTOCOLS: Array<[string, string]> = [["sftp", "SFTP"], ["smb", "SMB"], ["ftp", "FTP"], ["webdav", "WebDAV"]];
export const protocolName = (p: string) => PROTOCOLS.find(([k]) => k === p)?.[1] ?? p.toUpperCase();

interface NewTabPageProps {
  sources: ReturnType<typeof useFileSources>;
  onOpenAgent: (assetId: string, name: string) => void;
  onOpenConnection: (connectionId: string, name: string, protocol: string) => void;
}

type TestState = { state: "testing" | "ok" | "bad"; text: string };

function useDeviceLine() {
  const t = useTranslations("files.picker");
  return (a: Asset) => {
    const m = a.metadata ?? {};
    const os = m.os_pretty_name || m.os_name || a.platform || "";
    const guest = m.vmid && m.node ? t(a.type === "vm" ? "guestVm" : "guestLxc", { id: m.vmid, host: m.node }) : "";
    return [os, guest].filter(Boolean).join(" · ");
  };
}

const matches = (q: string, ...fields: Array<string | undefined>) => !q || fields.some((f) => f?.toLowerCase().includes(q));

export function NewTabPage({ sources, onOpenAgent, onOpenConnection }: NewTabPageProps) {
  const t = useTranslations("files.picker");
  const tm = useTranslations("files.menu");
  const line = useDeviceLine();
  const { ready, unavailable, connections, connectionsLoading, refetchConnections } = sources;
  const [query, setQuery] = useState("");
  const [form, setForm] = useState<{ protocol: string; existing?: FileConnection } | null>(null);
  const [showNot, setShowNot] = useState(false);
  const [removing, setRemoving] = useState<FileConnection | null>(null);
  const [tests, setTests] = useState<Record<string, TestState>>({});

  const q = query.trim().toLowerCase();
  const devices = useMemo(() => ready.filter((a) => matches(q, a.name, a.platform, a.metadata?.os_pretty_name)), [ready, q]);
  const saved = useMemo(() => connections.filter((c) => matches(q, c.name, c.host, c.protocol)), [connections, q]);
  const notReady = useMemo(() => unavailable.filter((u) => matches(q, u.asset.name)), [unavailable, q]);

  const onEnter = () => {
    const quick = parseQuickConnect(query);
    if (quick) { setForm({ protocol: quick.protocol }); return; }
    if (devices[0]) onOpenAgent(devices[0].id, devices[0].name);
    else if (saved[0]) onOpenConnection(saved[0].id, saved[0].name, saved[0].protocol);
  };

  const test = useCallback(async (c: FileConnection) => {
    setTests((s) => ({ ...s, [c.id]: { state: "testing", text: t("testing") } }));
    try {
      const r = await testFileConnection(c.id);
      const text = r.success ? t("answers", { ms: r.latency_ms ?? 0 }) : r.error || t("noAnswer");
      setTests((s) => ({ ...s, [c.id]: { state: r.success ? "ok" : "bad", text } }));
    } catch (err) {
      setTests((s) => ({ ...s, [c.id]: { state: "bad", text: err instanceof Error ? err.message : t("noAnswer") } }));
    }
  }, [t]);

  const closeForm = () => setForm(null);

  if (form) {
    return (
      <div className="fx-pick">
        <div className="fx-form-sheet">
          <ConnectionForm
            key={form.existing?.id ?? form.protocol}
            protocol={form.protocol}
            existingConnection={form.existing}
            onConnect={(c) => { closeForm(); void refetchConnections(); onOpenConnection(c.id, c.name, c.protocol); }}
            onCancel={closeForm}
            onDeleted={() => { closeForm(); void refetchConnections(); }}
          />
        </div>
      </div>
    );
  }

  const deviceRow = (a: Asset) => (
    <button key={a.id} type="button" className="fx-prow" onClick={() => onOpenAgent(a.id, a.name)}>
      <span className="fx-prow-nm">{a.name}</span>
      <span className="fx-prow-sub">{line(a)}</span>
      <ChevronRight className="fx-chev" aria-hidden="true" width={14} height={14} strokeWidth={2} />
    </button>
  );

  const connRow = (c: FileConnection) => {
    const tst = tests[c.id];
    return (
      <div key={c.id} className="fx-prow-w">
        <button type="button" className="fx-prow" onClick={() => onOpenConnection(c.id, c.name, c.protocol)}>
          {tst ? <span className={`fx-dot ${tst.state === "testing" ? "busy" : tst.state}`} aria-hidden="true" /> : null}
          <span className="fx-prow-nm">{c.name}</span>
          <span className={`fx-prow-sub${tst?.state === "bad" ? " bad" : ""}`}>
            {tst ? tst.text : `${protocolName(c.protocol)} · ${connectionAddress(c)}`}
          </span>
        </button>
        <RowMenu
          label={tm("actionsFor", { name: c.name })}
          items={() => [
            { key: "edit", label: t("edit"), onSelect: () => setForm({ protocol: c.protocol, existing: c }) },
            { key: "test", label: t("test"), onSelect: () => void test(c) },
            { key: "sep", sep: true },
            { key: "delete", label: tm("delete"), danger: true, onSelect: () => setRemoving(c) },
          ]}
        />
        <TransportNotice connection={c} className="fx-prow-note" />
      </div>
    );
  };

  const notRow = ({ asset, reason }: Unavailable) => (
    <div key={asset.id} className="fx-prow dim">
      <span className="fx-prow-nm">{asset.name}</span>
      <span className="fx-prow-sub">{line(asset)}</span>
      <span className="fx-prow-why">{t(reason)}</span>
    </div>
  );

  return (
    <div className="fx-pick">
      <label className="fx-pick-q">
        <Search aria-hidden="true" width={15} height={15} strokeWidth={2} />
        <input
          type="search"
          value={query}
          autoFocus
          spellCheck={false}
          autoComplete="off"
          placeholder={t("connectTo")}
          aria-label={t("connectTo")}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); onEnter(); } }}
        />
      </label>

      <section className="fx-sec" aria-labelledby="fx-devices-h">
        <h2 className="fx-h" id="fx-devices-h">{t("devices")}</h2>
        {devices.length ? <div className="fx-plist">{devices.map(deviceRow)}</div>
          : <p className="fx-none">{q ? t("noDeviceMatch", { query: query.trim() }) : t("noDevices")}</p>}
      </section>

      <section className="fx-sec" aria-labelledby="fx-conns-h">
        <div className="fx-sec-h">
          <h2 className="fx-h" id="fx-conns-h">{t("connections")}</h2>
          <ToolMenu
            label={t("newConnection")}
            trigger={
              <button type="button" className="fx-btn sm">
                <Plus aria-hidden="true" width={14} height={14} strokeWidth={2} />
                {t("newConnection")}
                <ChevronDown aria-hidden="true" width={13} height={13} strokeWidth={2} />
              </button>
            }
          >
            {PROTOCOLS.map(([k, name]) => (
              <Menu.Item key={k} className="lt-menu-item" onSelect={() => setForm({ protocol: k })}>{name}</Menu.Item>
            ))}
          </ToolMenu>
        </div>
        {saved.length ? <div className="fx-plist">{saved.map(connRow)}</div>
          : <p className="fx-none">{connectionsLoading ? t("loading") : q ? t("noConnectionMatch", { query: query.trim() }) : t("noConnections")}</p>}
      </section>

      {notReady.length ? (
        <section className="fx-sec">
          <button type="button" className="fx-h fx-fold" aria-expanded={showNot || Boolean(q)} onClick={() => setShowNot((v) => !v)}>
            {showNot || q ? <ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.4} /> : <ChevronRight aria-hidden="true" width={12} height={12} strokeWidth={2.4} />}
            {t("notAvailable")}
          </button>
          {showNot || q ? <div className="fx-plist">{notReady.map(notRow)}</div> : null}
        </section>
      ) : null}

      <ConfirmDialog
        open={removing !== null}
        onClose={() => setRemoving(null)}
        onConfirm={() => {
          const c = removing;
          setRemoving(null);
          if (c) void deleteFileConnection(c.id).then(refetchConnections, refetchConnections);
        }}
        title={t("deleteTitle", { name: removing?.name ?? "" })}
        message={t("deleteBody", { host: removing?.host ?? "" })}
        confirmLabel={t("delete")}
        danger
      />
    </div>
  );
}
