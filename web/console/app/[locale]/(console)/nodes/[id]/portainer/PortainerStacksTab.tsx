"use client";

import { useTranslations } from "next-intl";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Bar, ListEmpty, ListError, ListSkeleton } from "../../../../../components/containers/PanelParts";
import { RowMenu, useConfirm, type MenuEntry } from "../../../../../components/containers/RowMenu";
import { useAuth } from "../../../../../contexts/AuthContext";
import { useToast } from "../../../../../contexts/ToastContext";
import { hasWriteRole } from "../../../../../lib/roles";
import { portainerAction, portainerFetch } from "./usePortainerData";

type PortainerStack = {
  Id: number;
  Name: string;
  // 1 = active, 2 = inactive
  Status: number;
  Env?: Array<{ name: string; value: string }>;
};
type StackAction = "start" | "stop" | "redeploy" | "remove";

type Props = {
  assetId: string;
  bar?: ReactNode;
};

/** The endpoint's Portainer stacks: Start, Stop…, Redeploy… and Remove… in each row's ⋯. */
export function PortainerStacksTab({ assetId, bar }: Props) {
  const t = useTranslations("devices-docker.portainer");
  const { user } = useAuth();
  const { addToast } = useToast();
  const [ask, dialog] = useConfirm();
  const [stacks, setStacks] = useState<PortainerStack[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<number | null>(null);
  const seqRef = useRef(0);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    setLoading(true);
    try {
      const data = await portainerFetch<PortainerStack[]>(`/api/portainer/assets/${encodeURIComponent(assetId)}/stacks`);
      if (seqRef.current !== id) return;
      setStacks(data ?? []);
      setError(null);
    } catch (err) {
      if (seqRef.current !== id) return;
      setError(err instanceof Error ? err.message : "error");
    } finally {
      if (seqRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => { void load(); }, [load]);

  // Every stack action is a POST; the hub turns remove into Portainer's delete.
  const doAction = useCallback(async (stack: PortainerStack, action: StackAction) => {
    setBusy(stack.Id);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/stacks/${encodeURIComponent(stack.Id)}/${action}`,
        "POST",
      );
      addToast("success", t(`stackToast.${action}`, { name: stack.Name }));
      await load();
    } catch (err) {
      const why = err instanceof Error && err.message ? `: ${err.message}` : "";
      addToast("error", `${t(`stackFailed.${action}`, { name: stack.Name })}${why}`);
    } finally {
      setBusy(null);
    }
  }, [addToast, assetId, load, t]);

  const menu = (s: PortainerStack): MenuEntry[] => {
    if (!hasWriteRole(user?.role) || busy === s.Id) return [];
    const active = s.Status === 1;
    return [
      active
        ? { label: t("stop"), onSelect: () => ask({ title: t("stopTitle", { name: s.Name }), body: t("stopBody"), confirm: t("stopConfirm"), danger: true, run: () => doAction(s, "stop") }) }
        : { label: t("start"), onSelect: () => void doAction(s, "start") },
      { label: t("redeploy"), onSelect: () => ask({ title: t("redeployTitle", { name: s.Name }), body: t("redeployBody"), confirm: t("redeployConfirm"), run: () => doAction(s, "redeploy") }) },
      "sep",
      { label: t("remove"), danger: true, onSelect: () => ask({ title: t("removeTitle", { name: s.Name }), body: t("stackRemoveBody"), confirm: t("removeConfirm"), danger: true, run: () => doAction(s, "remove") }) },
    ];
  };

  let body: ReactNode;
  if (loading && !stacks.length) body = <ListSkeleton />;
  else if (error && !stacks.length) body = <ListError text={t("stacksFailed")} onRetry={() => void load()} />;
  else if (!stacks.length) body = <ListEmpty text={t("noStacks")} />;
  else body = (
    <div className="ct-list" role="table" aria-label={t("stacks")}>
      <div className="ct-row sm ct-cols" role="row"><span className="ct-lead" /><span>{t("stack")}</span><span /></div>
      {[...stacks].sort((a, b) => a.Name.localeCompare(b.Name)).map((s) => (
        <div key={s.Id} className="ct-row sm" role="row">
          <span className="ct-lead" aria-hidden="true">{busy === s.Id ? <i className="ct-dot busy" /> : s.Status === 1 ? null : <i className="ct-dot off" />}</span>
          <span className="ct-main" role="cell">
            <span className="ct-nm plain">{s.Name}</span>
            {s.Status !== 1 ? <span className="ct-grey w-off">{t("inactive")}</span> : null}
          </span>
          <span className="ct-act"><RowMenu label={t("stackMenu", { name: s.Name })} items={menu(s)} /></span>
        </div>
      ))}
    </div>
  );

  return (
    <>
      <Bar left={bar} right={<Button variant="ghost" onClick={() => void load()} disabled={loading}>{t("refresh")}</Button>} />
      {body}
      {dialog}
    </>
  );
}
