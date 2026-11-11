"use client";

// Snapshots of one datastore, filtered by type and ID: when, size, verified, protected. A row's ⋯ verifies or
// forgets it; several can be forgotten at once. Protected snapshots cannot be picked or forgotten.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiNum, DiRow, DiSection, DiTime } from "../proxmox/DiList";
import { sizeParts } from "../proxmox/diFormat";
import { normalizePBSSnapshotsResponse, type PBSAssetDetailsResponse, type PBSSnapshotEntry } from "../pbsTabModel";
import { verificationWord } from "./PBSGroupSnapshots";
import { PBSSelect } from "./PBSSelect";
import { pbsStoreNames } from "./pbsViewModel";
import { pbsAction, pbsErrorDetail, pbsFetch, usePBSActions } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

type Props = {
  assetId: string;
  details: PBSAssetDetailsResponse;
};

type Filter = { store: string; type: string; id: string };

const COLS = "24px minmax(0, 1fr) 72px 120px 28px";
const COLS_S = "24px minmax(0, 1fr) 96px 28px";
const keyOf = (snap: PBSSnapshotEntry) => `${snap.ns ?? ""}/${snap.backup_type}/${snap.backup_id}/${snap.backup_time}`;

export function PBSSnapshotsView({ assetId, details }: Props) {
  const { t, tk, when } = usePBSWords();
  const stores = useMemo(() => pbsStoreNames(details), [details]);
  const [store, setStore] = useState("");
  const [type, setType] = useState("vm");
  const [idInput, setIdInput] = useState("");
  const [applied, setApplied] = useState<Filter | null>(null);
  const [snapshots, setSnapshots] = useState<PBSSnapshotEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();
  const seqRef = useRef(0);

  const chosenStore = store || stores[0] || "";

  const load = useCallback(async (filter: Filter) => {
    const id = ++seqRef.current;
    setLoading(true);
    setError(null);
    const params = new URLSearchParams({ store: filter.store });
    if (filter.type) params.set("type", filter.type);
    if (filter.id) params.set("id", filter.id);
    try {
      const raw = await pbsFetch<unknown>(`/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots?${params.toString()}`);
      if (seqRef.current !== id) return;
      setSnapshots(normalizePBSSnapshotsResponse(raw).snapshots);
      setApplied(filter);
      setSelected(new Set());
    } catch (err) {
      if (seqRef.current !== id) return;
      setError(err instanceof Error ? err.message : "");
      setSnapshots([]);
    } finally {
      if (seqRef.current === id) setLoading(false);
    }
  }, [assetId]);

  // A new datastore or type shows its snapshots right away; a typed ID applies on Show or Enter.
  const idRef = useRef("");
  useEffect(() => {
    idRef.current = idInput.trim();
  }, [idInput]);
  useEffect(() => {
    if (!chosenStore) return;
    void load({ store: chosenStore, type, id: idRef.current });
  }, [chosenStore, type, load]);

  const sorted = useMemo(() => [...snapshots].sort((a, b) => b.backup_time - a.backup_time), [snapshots]);
  const reload = () => { if (applied) void load(applied); };
  const target = applied?.store ?? chosenStore;

  const forgetOne = (snap: PBSSnapshotEntry) => pbsAction(
    `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots/forget?${new URLSearchParams({
      store: target,
      "backup-type": snap.backup_type,
      "backup-id": snap.backup_id,
      "backup-time": String(snap.backup_time),
      ...(snap.ns ? { ns: snap.ns } : {}),
    }).toString()}`,
    "DELETE",
  );

  const verify = (snap: PBSSnapshotEntry) => run(
    `verify-${keyOf(snap)}`,
    () => pbsAction(`/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots/verify?store=${encodeURIComponent(target)}`, "POST", {
      store: target,
      backup_type: snap.backup_type,
      backup_id: snap.backup_id,
      backup_time: snap.backup_time,
    }),
    t("snap.verifyStarted", { name: `${snap.backup_type}/${snap.backup_id}` }),
    t("snap.verifyFailedToStart", { name: `${snap.backup_type}/${snap.backup_id}` }),
    reload,
  );

  const forget = (snap: PBSSnapshotEntry) => run(
    `forget-${keyOf(snap)}`,
    () => forgetOne(snap),
    t("snap.forgotten", { name: `${snap.backup_type}/${snap.backup_id}`, time: when(snap.backup_time) }),
    t("snap.forgetFailed", { name: `${snap.backup_type}/${snap.backup_id}` }),
    reload,
  );

  const picked = sorted.filter((snap) => selected.has(keyOf(snap)) && !snap.protected);

  const forgetPicked = async () => {
    if (picked.length === 0) return;
    const failures: string[] = [];
    await run("bulk-forget", async () => {
      for (const snap of picked) {
        try {
          await forgetOne(snap);
        } catch (err) {
          failures.push(pbsErrorDetail(err instanceof Error ? err.message : "") || `${snap.backup_type}/${snap.backup_id}`);
        }
      }
      if (failures.length > 0) throw new Error(failures.join(", "));
    }, t("snap.forgottenMany", { count: picked.length }), t("snap.forgetManyFailed"), () => setSelected(new Set()));
    reload();
  };

  const toggle = (key: string) => setSelected((prev) => {
    const next = new Set(prev);
    if (next.has(key)) next.delete(key); else next.add(key);
    return next;
  });

  return (
    <DiSection id="pbs-snapshots" title={t("view.snapshots")}>
      <div className="di-tools">
        <PBSSelect label={t("snap.datastore")} value={chosenStore} onChange={setStore} disabled={stores.length === 0}>
          {stores.length === 0 ? <option value="">{t("noDatastores")}</option> : stores.map((name) => <option key={name} value={name}>{name}</option>)}
        </PBSSelect>
        <PBSSelect label={t("snap.type")} value={type} onChange={setType}>
          <option value="vm">{t("snap.typeVm")}</option>
          <option value="ct">{t("snap.typeCt")}</option>
          <option value="host">{t("snap.typeHost")}</option>
        </PBSSelect>
        <label className="di-filter di-pbs-id">
          <input
            value={idInput}
            placeholder={t("snap.idPlaceholder")}
            aria-label={t("snap.id")}
            onChange={(e) => setIdInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && chosenStore) void load({ store: chosenStore, type, id: idInput.trim() }); }}
          />
        </label>
        <Button size="md" disabled={!chosenStore || loading} onClick={() => void load({ store: chosenStore, type, id: idInput.trim() })}>
          {t("snap.show")}
        </Button>
        <span className="sp" />
        {picked.length > 0 ? (
          <>
            <span className="di-word">{t("snap.picked", { count: picked.length })}</span>
            <Button
              size="md"
              variant="danger"
              disabled={busy !== null}
              onClick={() => ask({
                title: t("snap.ask.manyTitle", { count: picked.length }),
                message: t("snap.ask.manyBody", { store: target }),
                confirmLabel: t("snap.ask.forget"),
                onConfirm: forgetPicked,
              })}
            >
              {t("snap.forgetPicked", { count: picked.length })}
            </Button>
            <Button size="md" variant="ghost" onClick={() => setSelected(new Set())}>{tk("clearFilter")}</Button>
          </>
        ) : null}
      </div>

      {error ? <DiNote tone="bad">{[t("snap.loadFailed"), pbsErrorDetail(error)].filter(Boolean).join(" ")}</DiNote> : null}
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}

      <DiList cols={COLS} colsS={COLS_S} label={t("view.snapshots")}>
        <DiCols>
          <span />
          <span>{t("cols.snapshot")}</span>
          <span className="r di-s">{t("cols.size")}</span>
          <span className="r">{t("cols.taken")}</span>
          <span />
        </DiCols>
        {sorted.length === 0 ? (
          <DiEmpty title={loading ? tk("loading") : t("snap.none")} />
        ) : null}
        {sorted.map((snap) => {
          const key = keyOf(snap);
          const name = `${snap.backup_type}/${snap.backup_id}`;
          const verifyWord = verificationWord(snap.verification?.state, (k) => t(k));
          const size = sizeParts(snap.size);
          const time = when(snap.backup_time);
          return (
            <DiRow key={key} tone={verifyWord.bad ? "bad" : ""} selected={selected.has(key)}>
              <input
                type="checkbox"
                className="di-pbs-check"
                data-key={key}
                aria-label={t("snap.select", { name, time })}
                checked={selected.has(key)}
                disabled={snap.protected}
                onChange={() => toggle(key)}
              />
              <DiMain
                name={name}
                mono
                problem={verifyWord.bad ? { tone: "bad", text: verifyWord.text } : null}
                grey={<DiDots parts={[verifyWord.text, snap.protected ? t("snap.protected") : null, snap.ns, snap.comment, snap.owner]} />}
              />
              <DiNum value={size?.value} unit={size?.unit} className="di-s" />
              <DiTime>{time}</DiTime>
              <DiRowMenu
                label={tk("actionsFor", { name: `${name} ${time}` })}
                items={[
                  { label: t("snap.verify"), disabled: busy !== null, onSelect: () => void verify(snap) },
                  { separator: true },
                  {
                    label: t("snap.forget"),
                    danger: true,
                    disabled: busy !== null || !!snap.protected,
                    hint: snap.protected ? t("snap.protected") : undefined,
                    onSelect: () => ask({
                      title: t("snap.ask.oneTitle", { name, time }),
                      message: t("snap.ask.oneBody", { store: target }),
                      confirmLabel: t("snap.ask.forget"),
                      onConfirm: () => forget(snap),
                    }),
                  },
                ]}
              />
            </DiRow>
          );
        })}
      </DiList>
      {dialog}
    </DiSection>
  );
}
