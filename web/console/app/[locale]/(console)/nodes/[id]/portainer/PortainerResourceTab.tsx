"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Bar, ListEmpty, ListError, ListSkeleton } from "../../../../../components/containers/PanelParts";
import { RowMenu, useConfirm } from "../../../../../components/containers/RowMenu";
import { useAuth } from "../../../../../contexts/AuthContext";
import { useToast } from "../../../../../contexts/ToastContext";
import { hasWriteRole } from "../../../../../lib/roles";
import { portainerAction, portainerFetch } from "./usePortainerData";

export type ResourceColumn<T> = { label: string; cell: (item: T) => ReactNode; cls?: string };
export type CreateField = { key: string; label: string; placeholder: string; initial: string; required?: boolean };

/**
 * One Portainer resource list (images, volumes or networks): rows with Remove… in ⋯, and a create form (Pull image,
 * Create volume, Create network) that opens under the switch.
 */
export type ResourceSpec<T> = {
  kind: "images" | "volumes" | "networks";
  parse: (data: unknown) => T[];
  id: (item: T) => string;
  name: (item: T) => string;
  columns: ResourceColumn<T>[];
  /** CSS grid columns after the 16 px lead and before the ⋯. */
  grid: string;
  create: { button: string; title: string; submit: string; path: string; fields: CreateField[]; body: (v: Record<string, string>) => unknown };
};

export function PortainerResourceTab<T>({ assetId, bar, spec }: { assetId: string; bar?: ReactNode; spec: ResourceSpec<T> }) {
  const t = useTranslations("devices-docker.portainer");
  const { user } = useAuth();
  const { addToast } = useToast();
  const [ask, dialog] = useConfirm();
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, string> | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const seqRef = useRef(0);
  const base = `/api/portainer/assets/${encodeURIComponent(assetId)}/${spec.kind}`;
  const canWrite = hasWriteRole(user?.role);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    setLoading(true);
    try {
      const data = await portainerFetch<unknown>(base);
      if (seqRef.current !== id) return;
      setItems(spec.parse(data));
      setError(null);
    } catch (err) {
      if (seqRef.current !== id) return;
      setError(err instanceof Error ? err.message : "error");
    } finally {
      if (seqRef.current === id) setLoading(false);
    }
  }, [base, spec]);

  useEffect(() => { void load(); }, [load]);

  const remove = async (item: T) => {
    try {
      await portainerAction(`${base}/${encodeURIComponent(spec.id(item))}`, "DELETE");
      addToast("success", t("removed", { name: spec.name(item) }));
      await load();
    } catch (err) {
      const why = err instanceof Error && err.message ? `: ${err.message}` : "";
      addToast("error", `${t("removeFailed", { name: spec.name(item) })}${why}`);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!form || spec.create.fields.some((f) => f.required && !form[f.key]?.trim())) return;
    setSaving(true);
    setFormError(null);
    try {
      await portainerAction(spec.create.path ? `${base}/${spec.create.path}` : base, "POST", spec.create.body(form));
      setForm(null);
      await load();
    } catch (err) {
      setFormError(err instanceof Error && err.message ? err.message : t("createFailed"));
    } finally {
      setSaving(false);
    }
  };
  const open = () => { setFormError(null); setForm(Object.fromEntries(spec.create.fields.map((f) => [f.key, f.initial]))); };

  let body: ReactNode;
  if (loading && !items.length) body = <ListSkeleton />;
  else if (error && !items.length) body = <ListError text={t(`${spec.kind}Failed`)} onRetry={() => void load()} />;
  else if (!items.length) body = <ListEmpty text={t(`no.${spec.kind}`)} />;
  else body = (
    <div className="ct-list" role="table" aria-label={t(spec.kind)} style={{ ["--ct-res" as string]: spec.grid }}>
      <div className="ct-row res ct-cols" role="row">
        <span className="ct-lead" />
        {spec.columns.map((c) => <span key={c.label} className={c.cls}>{c.label}</span>)}
        <span />
      </div>
      {items.map((item) => (
        <div key={spec.id(item)} className="ct-row res" role="row">
          <span className="ct-lead" />
          {spec.columns.map((c) => <span key={c.label} role="cell" className={c.cls}>{c.cell(item)}</span>)}
          <span className="ct-act">
            <RowMenu
              label={t("rowMenu", { name: spec.name(item) })}
              items={canWrite ? [{ label: t("remove"), danger: true, onSelect: () => ask({ title: t("removeItemTitle", { name: spec.name(item) }), body: t(`removeBody.${spec.kind}`), confirm: t("removeConfirm"), danger: true, run: () => remove(item) }) }] : []}
            />
          </span>
        </div>
      ))}
    </div>
  );

  const createButton = canWrite && !form ? <Button onClick={open}><Plus size={14} aria-hidden="true" />{spec.create.button}</Button> : null;
  return (
    <>
      <Bar left={bar} right={<>{createButton}<Button variant="ghost" onClick={() => void load()} disabled={loading}>{t("refresh")}</Button></>} />
      {form ? (
        <form className="ct-sheet ct-form ct-inline" onSubmit={(e) => void submit(e)} aria-label={spec.create.title}>
          <div className="ct-grid two">
            {spec.create.fields.map((f, i) => (
              <div key={f.key} className="ct-field">
                <label htmlFor={`pt-${spec.kind}-${f.key}`}>{f.label}</label>
                <input className="ct-input mono" id={`pt-${spec.kind}-${f.key}`} value={form[f.key] ?? ""} placeholder={f.placeholder} autoComplete="off" spellCheck={false} autoFocus={i === 0}
                  onChange={(e) => setForm((v) => ({ ...(v ?? {}), [f.key]: e.target.value }))} />
              </div>
            ))}
          </div>
          {formError ? <p className="ct-err" role="alert">{formError}</p> : null}
          <div className="ct-form-foot">
            <Button variant="ghost" onClick={() => setForm(null)} disabled={saving}>{t("cancel")}</Button>
            <Button type="submit" loading={saving} disabled={spec.create.fields.some((f) => f.required && !form[f.key]?.trim())}>{spec.create.submit}</Button>
          </div>
        </form>
      ) : null}
      {body}
      {dialog}
    </>
  );
}
