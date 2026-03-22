"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Plus, Webhook, Pencil, Trash2 } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { EmptyState } from "../../../components/ui/EmptyState";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import { apiFetch, apiMutate } from "../../../lib/api";

// ── Types ──

interface WebhookRecord {
  id: string;
  name: string;
  url: string;
  secret?: string;
  events: string[];
  enabled: boolean;
  last_triggered_at?: string | null;
  created_at?: string;
}

// ── Constants ──

const EVENT_TYPES = [
  "asset.created",
  "asset.updated",
  "asset.deleted",
  "alert.fired",
  "alert.resolved",
  "incident.created",
  "incident.resolved",
  "action.completed",
  "update.completed",
] as const;

// ── Helpers ──

function truncateUrl(url: string, max = 40): string {
  return url.length > max ? `${url.slice(0, max)}…` : url;
}

function relativeTime(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const diff = Date.now() - new Date(iso).getTime();
  if (isNaN(diff)) return null;
  const secs = Math.floor(diff / 1000);
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

// ── Modal component ──

interface WebhookModalProps {
  mode: "create" | "edit";
  initial?: WebhookRecord;
  saving: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (fields: {
    name: string;
    url: string;
    secret: string;
    events: string[];
  }) => void;
}

function WebhookModal({
  mode,
  initial,
  saving,
  error,
  onClose,
  onSubmit,
}: WebhookModalProps) {
  const t = useTranslations("webhooks");
  const tc = useTranslations("common");

  const [name, setName] = useState(initial?.name ?? "");
  const [url, setUrl] = useState(initial?.url ?? "");
  const [secret, setSecret] = useState(initial?.secret ?? "");
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(
    new Set(initial?.events ?? []),
  );

  const toggleEvent = useCallback((event: string) => {
    setSelectedEvents((prev) => {
      const next = new Set(prev);
      if (next.has(event)) {
        next.delete(event);
      } else {
        next.add(event);
      }
      return next;
    });
  }, []);

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !saving) {
        e.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [saving, onClose]);

  const handleSubmit = useCallback(() => {
    onSubmit({ name, url, secret, events: Array.from(selectedEvents) });
  }, [name, url, secret, selectedEvents, onSubmit]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={() => { if (!saving) onClose(); }}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[36rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">
            {mode === "create" ? t("createWebhook") : t("editWebhook")}
          </h3>

          {/* Name */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("name")}</span>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("namePlaceholder")}
              disabled={saving}
            />
          </label>

          {/* URL */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("url")}</span>
            <Input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder={t("urlPlaceholder")}
              disabled={saving}
              type="url"
            />
          </label>

          {/* Secret */}
          <label className="block space-y-1">
            <span className="text-[10px] text-[var(--muted)]">{t("secret")}</span>
            <Input
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder={t("secretPlaceholder")}
              disabled={saving}
              type="password"
            />
          </label>

          {/* Event types */}
          <div className="space-y-2">
            <span className="text-[10px] text-[var(--muted)]">{t("eventTypes")}</span>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
              {EVENT_TYPES.map((evt) => (
                <label
                  key={evt}
                  className="flex items-center gap-2 cursor-pointer select-none"
                >
                  <input
                    type="checkbox"
                    checked={selectedEvents.has(evt)}
                    disabled={saving}
                    onChange={() => toggleEvent(evt)}
                    className="h-4 w-4 rounded border-[var(--line)] accent-[var(--accent)]"
                  />
                  <span className="text-xs text-[var(--text)] font-mono">{evt}</span>
                </label>
              ))}
            </div>
          </div>

          {error ? (
            <p className="text-xs text-[var(--bad)]">{error}</p>
          ) : null}

          <div className="flex items-center justify-end gap-2 pt-1">
            <Button
              variant="secondary"
              size="sm"
              onClick={onClose}
              disabled={saving}
            >
              {tc("cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={handleSubmit}
              loading={saving}
            >
              {tc("save")}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

// ── Page ──

export default function WebhooksPage() {
  const t = useTranslations("webhooks");

  const [webhooks, setWebhooks] = useState<WebhookRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // Modal state
  const [showCreate, setShowCreate] = useState(false);
  const [editTarget, setEditTarget] = useState<WebhookRecord | null>(null);
  const [modalSaving, setModalSaving] = useState(false);
  const [modalError, setModalError] = useState("");

  // Inline delete confirm
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const deleteRef = useRef<HTMLDivElement | null>(null);

  // Toggling enabled state
  const [togglingIds, setTogglingIds] = useState<Set<string>>(new Set());

  // ── Fetch ──

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const { response, data } = await apiFetch<{ webhooks: WebhookRecord[] }>(
        "/api/v2/webhooks",
      );
      if (!response.ok) {
        const msg =
          data && typeof data === "object" && "error" in data
            ? String((data as { error?: string }).error)
            : `Failed to load webhooks (${response.status})`;
        throw new Error(msg);
      }
      setWebhooks(data?.webhooks ?? []);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load webhooks.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // Close inline delete confirm when clicking outside
  useEffect(() => {
    if (!deleteTarget) return;
    const handler = (e: MouseEvent) => {
      if (deleteRef.current && !deleteRef.current.contains(e.target as Node)) {
        setDeleteTarget(null);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [deleteTarget]);

  // ── Create ──

  const openCreate = useCallback(() => {
    setModalError("");
    setShowCreate(true);
  }, []);

  const closeCreate = useCallback(() => {
    if (modalSaving) return;
    setShowCreate(false);
    setModalError("");
  }, [modalSaving]);

  const handleCreate = useCallback(
    async (fields: { name: string; url: string; secret: string; events: string[] }) => {
      const name = fields.name.trim();
      const url = fields.url.trim();
      if (!name || !url) {
        setModalError("Name and URL are required.");
        return;
      }
      setModalSaving(true);
      setModalError("");
      try {
        await apiMutate<WebhookRecord>("/api/v2/webhooks", "POST", {
          name,
          url,
          secret: fields.secret.trim() || undefined,
          events: fields.events,
        });
        setShowCreate(false);
        await load();
      } catch (err) {
        setModalError(err instanceof Error ? err.message : "Failed to create webhook.");
      } finally {
        setModalSaving(false);
      }
    },
    [load],
  );

  // ── Edit ──

  const openEdit = useCallback((webhook: WebhookRecord) => {
    setModalError("");
    setEditTarget(webhook);
  }, []);

  const closeEdit = useCallback(() => {
    if (modalSaving) return;
    setEditTarget(null);
    setModalError("");
  }, [modalSaving]);

  const handleEdit = useCallback(
    async (fields: { name: string; url: string; secret: string; events: string[] }) => {
      if (!editTarget) return;
      const name = fields.name.trim();
      const url = fields.url.trim();
      if (!name || !url) {
        setModalError("Name and URL are required.");
        return;
      }
      setModalSaving(true);
      setModalError("");
      try {
        await apiMutate<WebhookRecord>(
          `/api/v2/webhooks/${encodeURIComponent(editTarget.id)}`,
          "PATCH",
          {
            name,
            url,
            secret: fields.secret.trim() || undefined,
            events: fields.events,
            enabled: editTarget.enabled,
          },
        );
        setEditTarget(null);
        await load();
      } catch (err) {
        setModalError(err instanceof Error ? err.message : "Failed to update webhook.");
      } finally {
        setModalSaving(false);
      }
    },
    [editTarget, load],
  );

  // ── Delete ──

  const handleDelete = useCallback(
    async (id: string) => {
      setDeleting(true);
      try {
        await apiMutate(`/api/v2/webhooks/${encodeURIComponent(id)}`, "DELETE");
        setDeleteTarget(null);
        await load();
      } catch {
        // swallow — row will remain
      } finally {
        setDeleting(false);
      }
    },
    [load],
  );

  // ── Toggle enabled ──

  const handleToggleEnabled = useCallback(
    async (webhook: WebhookRecord) => {
      setTogglingIds((prev) => new Set(prev).add(webhook.id));
      try {
        await apiMutate<WebhookRecord>(
          `/api/v2/webhooks/${encodeURIComponent(webhook.id)}`,
          "PATCH",
          { enabled: !webhook.enabled },
        );
        await load();
      } catch {
        // swallow — toggle reverts visually on reload failure
      } finally {
        setTogglingIds((prev) => {
          const next = new Set(prev);
          next.delete(webhook.id);
          return next;
        });
      }
    },
    [load],
  );

  // ── Render ──

  return (
    <>
      <PageHeader
        title={t("title")}
        subtitle={t("subtitle")}
        action={
          <Button variant="primary" size="sm" onClick={openCreate}>
            <Plus size={14} />
            {t("createWebhook")}
          </Button>
        }
      />

      <Card variant="flush">
        {loading ? (
          <div className="p-4 space-y-1">
            <SkeletonRow />
            <SkeletonRow />
            <SkeletonRow />
          </div>
        ) : loadError ? (
          <div className="p-6 text-sm text-[var(--bad)]">{loadError}</div>
        ) : webhooks.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={Webhook}
              title={t("noWebhooks")}
              description={t("noWebhooksDesc")}
              action={
                <Button size="sm" variant="secondary" onClick={openCreate}>
                  <Plus size={14} />
                  {t("createWebhook")}
                </Button>
              }
            />
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--line)] text-left">
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)]">
                  {t("name")}
                </th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden sm:table-cell">
                  {t("url")}
                </th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden md:table-cell">
                  {t("events")}
                </th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)]">
                  {t("enabled")}
                </th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] hidden lg:table-cell">
                  {t("lastTriggered")}
                </th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--muted)] text-right">
                </th>
              </tr>
            </thead>
            <tbody>
              {webhooks.map((wh) => {
                const lastTriggered = relativeTime(wh.last_triggered_at);
                const isDeletePending = deleteTarget === wh.id;
                const isToggling = togglingIds.has(wh.id);

                return (
                  <tr
                    key={wh.id}
                    className="border-b border-[var(--line)] last:border-0 hover:bg-[var(--surface-hover)] transition-colors duration-[var(--dur-instant)]"
                  >
                    {/* Name */}
                    <td className="px-4 py-3 text-[var(--text)] font-medium">
                      {wh.name}
                    </td>

                    {/* URL */}
                    <td className="px-4 py-3 text-[var(--muted)] font-mono text-xs hidden sm:table-cell">
                      <span title={wh.url}>{truncateUrl(wh.url)}</span>
                    </td>

                    {/* Events */}
                    <td className="px-4 py-3 hidden md:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {wh.events.length === 0 ? (
                          <span className="text-xs text-[var(--muted)]">—</span>
                        ) : (
                          wh.events.map((evt) => (
                            <span
                              key={evt}
                              className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-[var(--surface)] text-[var(--muted)]"
                            >
                              {evt}
                            </span>
                          ))
                        )}
                      </div>
                    </td>

                    {/* Enabled */}
                    <td className="px-4 py-3">
                      <button
                        type="button"
                        title={wh.enabled ? "Enabled — click to disable" : "Disabled — click to enable"}
                        disabled={isToggling}
                        onClick={() => { void handleToggleEnabled(wh); }}
                        className="inline-flex items-center gap-1.5 cursor-pointer disabled:opacity-40 disabled:pointer-events-none"
                      >
                        <span
                          className={`inline-block h-2 w-2 rounded-full transition-colors duration-[var(--dur-fast)] ${
                            wh.enabled
                              ? "bg-[var(--ok)] shadow-[0_0_4px_var(--ok-glow)]"
                              : "bg-[var(--muted)]"
                          }`}
                        />
                      </button>
                    </td>

                    {/* Last triggered */}
                    <td className="px-4 py-3 text-xs text-[var(--muted)] hidden lg:table-cell">
                      {lastTriggered ?? t("never")}
                    </td>

                    {/* Actions */}
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        {isDeletePending ? (
                          <div
                            ref={deleteRef}
                            className="flex items-center gap-1"
                          >
                            <Button
                              variant="danger"
                              size="sm"
                              loading={deleting}
                              onClick={() => { void handleDelete(wh.id); }}
                            >
                              <Trash2 size={12} />
                              {t("delete")}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              disabled={deleting}
                              onClick={() => setDeleteTarget(null)}
                            >
                              {t("cancel")}
                            </Button>
                          </div>
                        ) : (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => openEdit(wh)}
                            >
                              <Pencil size={12} />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setDeleteTarget(wh.id)}
                            >
                              <Trash2 size={12} />
                            </Button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </Card>

      {/* Create modal */}
      {showCreate ? (
        <WebhookModal
          mode="create"
          saving={modalSaving}
          error={modalError}
          onClose={closeCreate}
          onSubmit={(fields) => { void handleCreate(fields); }}
        />
      ) : null}

      {/* Edit modal */}
      {editTarget ? (
        <WebhookModal
          mode="edit"
          initial={editTarget}
          saving={modalSaving}
          error={modalError}
          onClose={closeEdit}
          onSubmit={(fields) => { void handleEdit(fields); }}
        />
      ) : null}
    </>
  );
}
