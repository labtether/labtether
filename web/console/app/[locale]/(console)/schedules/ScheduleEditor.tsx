"use client";

import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import type { Asset, Group } from "../../../console/models";
import { Input, Select, Textarea } from "../../../components/ui/Input";
import { errorText } from "../actions/_parts/autoApi";
import { futureWhen } from "../actions/_parts/autoTime";
import { EditorSheet, Field, fieldAria } from "../actions/_parts/Editor";
import { Switch } from "../actions/_parts/RowMenu";
import { browserZone, CRON_PRESETS, type CronPresetID, nextRun, presetExpr, presetOf } from "./cronModel";
import { DevicePicker } from "./DevicePicker";
import type { Schedule, ScheduleInput } from "./scheduleModel";

const CUSTOM = "custom";

/**
 * New or edit schedule: name, when (a preset in this browser's time zone, or a custom cron line with a live "Next: Wed
 * 02:00"), the command, the devices or group it runs on, and On. The hub checks the line again when it saves.
 */
export function ScheduleEditor({
  schedule,
  devices,
  groups,
  onSave,
  onClose,
}: {
  schedule?: Schedule;
  devices: Asset[];
  groups: Group[];
  onSave: (input: ScheduleInput) => Promise<void>;
  onClose: () => void;
}) {
  const t = useTranslations("schedules.editor");
  const tc = useTranslations("actions.common");
  const locale = useLocale();
  const zone = useMemo(() => browserZone(), []);
  const startPreset = schedule ? presetOf(schedule.cron_expr, zone) : "daily";

  const [name, setName] = useState(schedule?.name ?? "");
  const [when, setWhen] = useState<CronPresetID | typeof CUSTOM>(startPreset || CUSTOM);
  const [cron, setCron] = useState(schedule && !startPreset ? schedule.cron_expr : "0 2 * * *");
  const [command, setCommand] = useState(schedule?.command ?? "");
  const [targets, setTargets] = useState<string[]>(schedule?.targets ?? []);
  const [group, setGroup] = useState(schedule?.group_id ?? "");
  const [enabled, setEnabled] = useState(schedule?.enabled ?? true);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const expr = when === CUSTOM ? cron.trim().replace(/\s+/g, " ") : presetExpr(when, zone);
  // Custom starts from the line the preset would save (with its CRON_TZ= zone), so switching does not move the time.
  const pickWhen = (value: CronPresetID | typeof CUSTOM) => {
    if (value === CUSTOM && when !== CUSTOM) setCron(presetExpr(when, zone));
    setWhen(value);
  };
  const next = nextRun(expr);
  const nextText = next ? t("next", { time: futureWhen(next.toISOString(), new Date(), locale) }) : t("cronHelp");

  const save = async () => {
    const found: Record<string, string> = {};
    if (!name.trim()) found.name = t("nameRequired");
    if (when === CUSTOM && !next) found.cron = t("cronHelp");
    if (!command.trim()) found.command = t("commandRequired");
    if (!targets.length && !group) found.targets = t("targetsRequired");
    setErrors(found);
    if (Object.keys(found).length) return;
    setPending(true);
    setFormError(null);
    try {
      await onSave({ name: name.trim(), cron_expr: expr, command: command.trim(), targets, group_id: group, enabled });
    } catch (cause) {
      setFormError(errorText(cause, schedule ? t("saveFailed") : t("createFailed")));
    } finally {
      setPending(false);
    }
  };

  return (
    <EditorSheet
      title={schedule ? t("editTitle") : t("newTitle")}
      submitLabel={schedule ? tc("save") : tc("create")}
      cancelLabel={tc("cancel")}
      onSubmit={() => { void save(); }}
      onCancel={onClose}
      pending={pending}
      error={formError}
      footStart={(
        <span className="au-pv">
          <Switch id="sf-on" on={enabled} labelledBy="sf-on-l" onChange={setEnabled} />
          <span id="sf-on-l">{tc("on")}</span>
        </span>
      )}
    >
      <div className="au-grid">
        <Field id="sf-name" label={t("name")} error={errors.name}>
          <Input id="sf-name" value={name} maxLength={200} autoComplete="off" onChange={(e) => setName(e.target.value)} {...fieldAria("sf-name", errors.name)} />
        </Field>
        <Field id="sf-when" label={t("when")} hint={when === CUSTOM ? undefined : nextText}>
          <Select className="w-full" id="sf-when" value={when} onChange={(e) => pickWhen(e.target.value as CronPresetID | typeof CUSTOM)} {...fieldAria("sf-when", undefined, when !== CUSTOM)}>
            {CRON_PRESETS.map((p) => <option key={p.id} value={p.id}>{t(`presets.${p.id}`)}</option>)}
            <option value={CUSTOM}>{t("custom")}</option>
          </Select>
        </Field>
        {when === CUSTOM ? (
          <Field id="sf-cron" label={t("cron")} error={errors.cron} hint={<span className="num">{nextText}</span>} wide>
            <Input
              id="sf-cron"
              className="au-mono"
              value={cron}
              maxLength={256}
              autoComplete="off"
              spellCheck={false}
              placeholder="0 2 * * *"
              onChange={(e) => setCron(e.target.value)}
              {...fieldAria("sf-cron", errors.cron, true)}
            />
          </Field>
        ) : null}
        <Field id="sf-cmd" label={t("command")} error={errors.command} wide>
          <Textarea
            id="sf-cmd"
            className="au-mono au-two"
            rows={2}
            value={command}
            maxLength={4096}
            spellCheck={false}
            placeholder="df -h"
            onChange={(e) => setCommand(e.target.value)}
            {...fieldAria("sf-cmd", errors.command)}
          />
        </Field>
        <Field id="sf-dev" label={t("devices")} error={errors.targets}>
          <DevicePicker
            id="sf-dev"
            value={targets}
            devices={devices}
            addLabel={t("addDevice")}
            removeLabel={(device) => tc("remove", { name: device })}
            onChange={setTargets}
            describedBy={errors.targets ? "sf-dev-err" : undefined}
          />
        </Field>
        <Field id="sf-group" label={t("group")}>
          <Select className="w-full" id="sf-group" value={group} onChange={(e) => setGroup(e.target.value)}>
            <option value="">{t("noGroup")}</option>
            {group && !groups.some((g) => g.id === group) ? <option value={group}>{group}</option> : null}
            {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
          </Select>
        </Field>
      </div>
    </EditorSheet>
  );
}
