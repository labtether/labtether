"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { type WebhookRecord } from "./webhookPayload";

// ── Constants ──

export const EVENT_TYPES = [
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

export const MAX_WEBHOOK_NAME_LENGTH = 120;

export const MAX_WEBHOOK_URL_LENGTH = 4096;

export const MAX_WEBHOOK_SECRET_LENGTH = 4096;

// ── Modal component ──

export interface WebhookModalProps {
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

export function WebhookModal({
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
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="webhook-dialog-title"
        onClick={(e) => e.stopPropagation()}
      >
        <Card className="w-[36rem] max-w-[92vw] space-y-4">
          <h3 id="webhook-dialog-title" className="text-sm font-medium text-[var(--text)]">
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
              maxLength={MAX_WEBHOOK_NAME_LENGTH}
              autoFocus
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
              maxLength={MAX_WEBHOOK_URL_LENGTH}
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
              maxLength={MAX_WEBHOOK_SECRET_LENGTH}
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
