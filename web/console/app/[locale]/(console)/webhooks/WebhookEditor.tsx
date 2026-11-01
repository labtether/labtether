"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";
import { Input } from "../../../components/ui/Input";
import { errorText } from "../actions/_parts/autoApi";
import { EditorSheet, Field, fieldAria } from "../actions/_parts/Editor";
import { EVENT_GROUPS, EVENT_TYPES, eventKey } from "./webhookEvents";
import type { WebhookRecord } from "./webhookPayload";
import type { WebhookInput } from "./useWebhooks";

const MAX_WEBHOOK_NAME_LENGTH = 120;
const MAX_WEBHOOK_URL_LENGTH = 4096;
const MAX_WEBHOOK_SECRET_LENGTH = 4096;

/**
 * New or edit webhook: name, URL, an optional signing secret (never shown again; empty keeps the saved one) and the
 * events it sends, grouped as the hub names them. No event picked sends every event.
 */
export function WebhookEditor({
  webhook,
  onSave,
  onClose,
}: {
  webhook?: WebhookRecord;
  onSave: (input: WebhookInput) => Promise<void>;
  onClose: () => void;
}) {
  const t = useTranslations("webhooks.editor");
  const te = useTranslations("webhooks.events");
  const tc = useTranslations("actions.common");
  const [name, setName] = useState(webhook?.name ?? "");
  const [url, setUrl] = useState(webhook?.url ?? "");
  const [secret, setSecret] = useState("");
  const [events, setEvents] = useState<Set<string>>(new Set(webhook?.events ?? ["alert.fired", "alert.resolved"]));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const flip = (event: string) => setEvents((current) => {
    const next = new Set(current);
    if (next.has(event)) next.delete(event); else next.add(event);
    return next;
  });

  const save = async () => {
    const found: Record<string, string> = {};
    const address = url.trim();
    if (!name.trim()) found.name = t("nameRequired");
    if (!address) found.url = t("urlRequired");
    else if (!/^https?:\/\/\S+$/i.test(address)) found.url = t("urlInvalid");
    setErrors(found);
    if (Object.keys(found).length) return;
    setPending(true);
    setFormError(null);
    try {
      await onSave({ name: name.trim(), url: address, secret, events: [...events] });
    } catch (cause) {
      setFormError(errorText(cause, webhook ? t("saveFailed") : t("createFailed")));
    } finally {
      setPending(false);
    }
  };

  const insecure = url.trim().toLowerCase().startsWith("http://");

  return (
    <EditorSheet
      title={webhook ? t("editTitle") : t("newTitle")}
      submitLabel={webhook ? tc("save") : tc("create")}
      cancelLabel={tc("cancel")}
      onSubmit={() => { void save(); }}
      onCancel={onClose}
      pending={pending}
      error={formError}
    >
      <div className="au-grid au-wh">
        <Field id="wf-name" label={t("name")} error={errors.name}>
          <Input id="wf-name" value={name} maxLength={MAX_WEBHOOK_NAME_LENGTH} autoComplete="off" onChange={(e) => setName(e.target.value)} {...fieldAria("wf-name", errors.name)} />
        </Field>
        <Field id="wf-url" label={t("url")} error={errors.url} hint={insecure ? t("httpNotice") : undefined}>
          <Input
            id="wf-url"
            type="url"
            className="au-mono"
            value={url}
            maxLength={MAX_WEBHOOK_URL_LENGTH}
            autoComplete="off"
            spellCheck={false}
            placeholder="https://"
            onChange={(e) => setUrl(e.target.value)}
            {...fieldAria("wf-url", errors.url, insecure)}
          />
        </Field>
        <Field id="wf-secret" label={t("secret")}>
          <Input
            id="wf-secret"
            type="password"
            className="au-mono"
            value={secret}
            maxLength={MAX_WEBHOOK_SECRET_LENGTH}
            autoComplete="new-password"
            placeholder={webhook ? t("secretKeep") : t("optional")}
            onChange={(e) => setSecret(e.target.value)}
          />
        </Field>
        <fieldset className="au-evs au-wide" aria-describedby={events.size ? undefined : "wf-ev-hint"}>
          <legend>{t("events")}</legend>
          <div className="au-evg">
            {EVENT_GROUPS.map((group) => (
              <div key={group.id} className="au-evc" role="group" aria-label={te(`groups.${group.id}`)}>
                {EVENT_TYPES.filter((e) => e.startsWith(group.prefix)).map((event) => (
                  <label key={event} className="au-ck" title={event}>
                    <input type="checkbox" checked={events.has(event)} onChange={() => flip(event)} />
                    <span>{te(eventKey(event))}</span>
                  </label>
                ))}
              </div>
            ))}
          </div>
          {events.size ? null : <p className="au-hint" id="wf-ev-hint">{t("eventsAll")}</p>}
        </fieldset>
      </div>
    </EditorSheet>
  );
}
