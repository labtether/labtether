"use client";

import { useEffect, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Input, Select } from "../../../components/ui/Input";
import type { Group, MaintenanceWindow } from "../../../console/models";
import { Field, PeopleDialog, errorText } from "../users/PeopleDialog";
import { groupPath } from "../groups/groupListModel";
import { windowState } from "./healthModel";
import { span } from "./healthTime";
import type { NewDowntime } from "./useDowntimeWindows";

/** A Date as the value a datetime-local input wants, in local time. */
const toLocalInput = (value: Date) => new Date(value.getTime() - value.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);

type ScheduleProps = {
  open: boolean;
  groups: Group[];
  groupId: string;
  onClose: () => void;
  onSchedule: (groupId: string, body: NewDowntime) => Promise<void>;
};

type Effects = { suppress_alerts: boolean; block_actions: boolean; block_updates: boolean };

/** Schedule downtime for a group: a name, start and end, and what it mutes or pauses. Starts in 15 minutes, for an hour. */
export function ScheduleDowntimeDialog({ open, groups, groupId, onClose, onSchedule }: ScheduleProps) {
  const t = useTranslations("reliability.schedule");
  const [group, setGroup] = useState("");
  const [name, setName] = useState("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [effects, setEffects] = useState<Effects>({ suppress_alerts: true, block_actions: false, block_updates: false });
  const [error, setError] = useState<{ name?: string; end?: string; hub?: string }>({});
  const [saving, setSaving] = useState(false);
  const options = useMemo(
    () => groups.map((g) => ({ id: g.id, label: groupPath(groups, g.id) })).sort((a, b) => a.label.localeCompare(b.label)),
    [groups],
  );

  useEffect(() => {
    if (!open) return;
    const from = new Date(Date.now() + 15 * 60_000);
    setGroup(groupId);
    setName(t("defaultName"));
    setStart(toLocalInput(from));
    setEnd(toLocalInput(new Date(from.getTime() + 60 * 60_000)));
    setEffects({ suppress_alerts: true, block_actions: false, block_updates: false });
    setError({});
    setSaving(false);
  }, [open, groupId, t]);

  const submit = async () => {
    const from = new Date(start);
    const to = new Date(end);
    const found = {
      name: name.trim() ? undefined : t("errors.name"),
      end: Number.isNaN(from.getTime()) || Number.isNaN(to.getTime()) ? t("errors.times") : to <= from ? t("errors.order") : undefined,
    };
    setError(found);
    if (found.name || found.end || !group) return;
    setSaving(true);
    try {
      await onSchedule(group, { name: name.trim(), start_at: from.toISOString(), end_at: to.toISOString(), ...effects });
    } catch (err) {
      setError({ hub: errorText(err, t("errors.failed")) });
      setSaving(false);
    }
  };

  const toggle = (key: keyof Effects, label: string) => (
    <label className="pp-check">
      <input type="checkbox" checked={effects[key]} onChange={(e) => setEffects((cur) => ({ ...cur, [key]: e.target.checked }))} disabled={saving} />
      {label}
    </label>
  );

  return (
    <PeopleDialog open={open} title={t("title")} onClose={onClose} onSubmit={() => { void submit(); }} submitLabel={t("submit")} tone="main" busy={saving} error={error.hub}>
      <div className="pp-grid2">
        <Field id="pp-dt-group" label={t("group")}>
          <Select id="pp-dt-group" value={group} onChange={(e) => setGroup(e.target.value)} disabled={saving} className="w-full">
            {options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}
          </Select>
        </Field>
        <Field id="pp-dt-name" label={t("name")} error={error.name}>
          <Input id="pp-dt-name" value={name} onChange={(e) => setName(e.target.value)} disabled={saving} autoComplete="off" error={Boolean(error.name)} aria-invalid={error.name ? true : undefined} />
        </Field>
        <Field id="pp-dt-start" label={t("starts")}>
          <Input id="pp-dt-start" type="datetime-local" value={start} onChange={(e) => setStart(e.target.value)} disabled={saving} />
        </Field>
        <Field id="pp-dt-end" label={t("ends")} error={error.end}>
          <Input id="pp-dt-end" type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} disabled={saving} error={Boolean(error.end)} aria-invalid={error.end ? true : undefined} aria-describedby={error.end ? "pp-dt-end-err" : undefined} />
        </Field>
      </div>
      <fieldset className="pp-checks">
        <legend className="sr-only">{t("effects")}</legend>
        {toggle("suppress_alerts", t("mute"))}
        {toggle("block_actions", t("pauseActions"))}
        {toggle("block_updates", t("pauseUpdates"))}
      </fieldset>
    </PeopleDialog>
  );
}

type EndProps = { target: MaintenanceWindow | null; groupName: string; onClose: () => void; onEnd: (target: MaintenanceWindow) => Promise<void> };

/** End a running window now, or delete one that has not started or has ended. */
export function EndDowntimeDialog({ target, groupName, onClose, onEnd }: EndProps) {
  const t = useTranslations("reliability.end");
  const locale = useLocale();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setError("");
    setBusy(false);
  }, [target]);
  const active = target ? windowState(target, Date.now()) === "active" : false;

  const submit = async () => {
    if (!target) return;
    setBusy(true);
    try {
      await onEnd(target);
    } catch (err) {
      setError(errorText(err, t("failed")));
      setBusy(false);
    }
  };

  return (
    <PeopleDialog
      open={Boolean(target)}
      title={active ? t("titleActive", { name: groupName }) : t("title", { name: groupName })}
      onClose={onClose}
      onSubmit={() => { void submit(); }}
      submitLabel={active ? t("submitActive") : t("submit")}
      tone="danger"
      busy={busy}
      error={error}
    >
      <p className="pp-say">
        {target ? `${target.name} · ${span(target.start_at, target.end_at, Date.now(), locale)}.` : ""}
        {active ? ` ${t("bodyActive")}` : ""}
      </p>
    </PeopleDialog>
  );
}
