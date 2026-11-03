"use client";

import { Plus } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState, type FormEvent } from "react";
import { Button } from "../../../components/ui/Button";
import type { AlertSilence } from "../../../console/models";
import { useToast } from "../../../contexts/ToastContext";
import { Dot } from "./alertBits";
import { shortTime } from "./alertsModel";
import { blankLabel, durationPresets, labelsFromRows, type DurationPreset, type LabelRow, type SilencePayload } from "./alertsPageTypes";
import { LabelRows } from "./LabelRows";
import { Field } from "./ruleFields";
import { useCanWrite } from "../../../hooks/useCanWrite";

type Props = {
  silences: AlertSilence[];
  createSilence: (payload: SilencePayload) => Promise<void>;
  deleteSilence: (id: string) => Promise<void>;
};

/** Silences as a plain list (active first, then ended), with a form to silence alerts whose rule labels match. */
export function AlertSilencesTab({ silences, createSilence, deleteSilence }: Props) {
  const t = useTranslations("alerts.silences");
  const canWrite = useCanWrite();
  const locale = useLocale();
  const { addToast } = useToast();
  const [formOpen, setFormOpen] = useState(false);
  const [rows, setRows] = useState<LabelRow[]>([blankLabel()]);
  const [duration, setDuration] = useState<DurationPreset>("1h");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [saving, setSaving] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const now = Date.now();
  const active = silences.filter((s) => Date.parse(s.ends_at) > now).sort((a, b) => Date.parse(a.ends_at) - Date.parse(b.ends_at));
  const ended = silences.filter((s) => !(Date.parse(s.ends_at) > now)).sort((a, b) => Date.parse(b.ends_at) - Date.parse(a.ends_at));
  const matchers = labelsFromRows(rows);
  const filled = Object.keys(matchers);
  const matcherError = tried && filled.length === 0 ? t("errors.matchers") : undefined;

  const close = () => {
    setFormOpen(false);
    setRows([blankLabel()]);
    setDuration("1h");
    setReason("");
    setTried(false);
  };

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (filled.length === 0) {
      setTried(true);
      return;
    }
    const hours = durationPresets.find((p) => p.id === duration)?.hours ?? 1;
    const start = new Date();
    setSaving(true);
    try {
      await createSilence({
        matchers,
        starts_at: start.toISOString(),
        ends_at: new Date(start.getTime() + hours * 3_600_000).toISOString(),
        reason: reason.trim() || undefined,
      });
      addToast("success", t("created"));
      close();
    } catch (err) {
      addToast("error", err instanceof Error && err.message ? err.message : t("errors.save"));
    } finally {
      setSaving(false);
    }
  }

  async function remove(silence: AlertSilence) {
    setBusy(silence.id);
    try {
      await deleteSilence(silence.id);
    } catch (err) {
      addToast("error", err instanceof Error && err.message ? err.message : t("errors.delete"));
    } finally {
      setBusy(null);
    }
  }

  const row = (silence: AlertSilence, live: boolean) => {
    const matchers = Object.entries(silence.matchers ?? {}).map(([k, v]) => `${k}=${v}`).join(", ");
    return (
      <div key={silence.id} className={`al-row two${live ? "" : " done"}`}>
        <Dot tone={live ? "" : "off"} />
        <span className="al-it">
          <span className="al-t1">{silence.reason || t("unnamed")}</span>
          <span className="al-s al-mono">{matchers}</span>
        </span>
        <span className="al-row-end">
          <span className="al-time">{live ? t("until", { time: shortTime(silence.ends_at, locale, now) }) : t("endedAt", { time: shortTime(silence.ends_at, locale, now) })}</span>
          {canWrite ? (
            <Button size="sm" variant="ghost" disabled={busy === silence.id} onClick={() => void remove(silence)}>
              {live ? t("end") : t("remove")}
            </Button>
          ) : null}
        </span>
      </div>
    );
  };

  return (
    <>
      <div className="al-tools">
        <span className="al-sp" />
        {!formOpen && canWrite ? (
          <Button onClick={() => setFormOpen(true)}>
            <Plus aria-hidden="true" size={14} strokeWidth={2} />
            {t("add")}
          </Button>
        ) : null}
      </div>

      {formOpen ? (
        <form className="al-sheet al-sform" aria-label={t("add")} onSubmit={(e) => void submit(e)} noValidate>
          <LabelRows legend={t("matchers")} rows={rows} onRows={setRows} hint={t("matchHint")} error={matcherError} />
          <div className="al-grid">
            <Field id="sf-for" label={t("duration")}>
              <select id="sf-for" className="al-in" value={duration} onChange={(e) => setDuration(e.target.value as DurationPreset)}>
                {durationPresets.map((p) => <option key={p.id} value={p.id}>{t(`durations.${p.id}`)}</option>)}
              </select>
            </Field>
            <Field id="sf-reason" label={t("reason")}>
              <input id="sf-reason" className="al-in" value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("reasonPlaceholder")} autoComplete="off" />
            </Field>
          </div>
          <div className="al-ed-acts">
            <span className="al-sp" />
            <Button type="button" variant="ghost" onClick={close}>{t("cancel")}</Button>
            <Button type="submit" variant="primary" disabled={saving}>{saving ? t("saving") : t("save")}</Button>
          </div>
        </form>
      ) : null}

      {silences.length === 0 ? (
        !formOpen ? <div className="al-sheet al-empty"><b>{t("empty")}</b></div> : null
      ) : (
        <div className="al-sheet al-plain" role="group" aria-label={t("listLabel")}>
          {active.map((s) => row(s, true))}
          {active.length === 0 ? <p className="al-none">{t("noneActive")}</p> : null}
          {ended.length > 0 ? <div className="al-group">{t("ended")}</div> : null}
          {ended.map((s) => row(s, false))}
        </div>
      )}
    </>
  );
}
